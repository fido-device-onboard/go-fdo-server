// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package apikey

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

var errInternal = errors.New("internal authentication error")

const (
	headerName     = "X-API-Key"
	keyPrefix      = "fdo_"
	minKeyLen      = 10
	prefixStartIdx = 4
	prefixEndIdx   = 10
)

var _ auth.Authenticator = (*APIKeyAuthenticator)(nil)

type APIKeyAuthenticator struct {
	db *gorm.DB
	wg sync.WaitGroup
}

func New(db *gorm.DB) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{db: db}
}

func (a *APIKeyAuthenticator) Wait() {
	a.wg.Wait()
}

func (a *APIKeyAuthenticator) Name() string { return "api-key" }

func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*auth.Identity, error) {
	key := r.Header.Get(headerName)
	if key == "" {
		return nil, nil
	}

	if !strings.HasPrefix(key, keyPrefix) || len(key) < minKeyLen {
		slog.Debug("API key format invalid")
		return nil, auth.ErrInvalidCredentials
	}

	prefix := key[prefixStartIdx:prefixEndIdx]

	candidates, err := state.FindAPIKeysByPrefix(ctx, a.db, prefix)
	if err != nil {
		slog.Error("Failed to look up API keys by prefix", "prefix", prefix, "error", err)
		return nil, errInternal
	}

	hash := sha256.Sum256([]byte(key))

	for _, candidate := range candidates {
		if len(candidate.HashedKey) != len(hash) || subtle.ConstantTimeCompare(candidate.HashedKey, hash[:]) != 1 {
			continue
		}

		if candidate.ExpiresAt != nil && candidate.ExpiresAt.Before(time.Now()) {
			slog.Debug("API key expired", "prefix", prefix)
			return nil, auth.ErrInvalidCredentials
		}

		user, err := state.GetUserByID(ctx, a.db, candidate.UserID)
		if err != nil {
			slog.Error("Failed to look up API key owner", "user_id", candidate.UserID, "error", err)
			return nil, errInternal
		}
		if !user.Active {
			slog.Warn("API key authentication rejected: user inactive", "user_id", candidate.UserID, "prefix", prefix)
			return nil, auth.ErrInvalidCredentials
		}

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			state.UpdateAPIKeyLastUsed(bgCtx, a.db, candidate.ID)
		}()

		scopes, err := a.resolveScopes(ctx, &candidate)
		if err != nil {
			slog.Error("Failed to resolve scopes for API key", "api_key_id", candidate.ID, "error", err)
			return nil, errInternal
		}

		roles, err := state.GetUserRoles(ctx, a.db, candidate.UserID)
		if err != nil {
			slog.Error("Failed to look up user roles", "user_id", candidate.UserID, "error", err)
			return nil, errInternal
		}
		roleNames := make([]string, 0, len(roles))
		for _, r := range roles {
			roleNames = append(roleNames, r.Name)
		}

		return auth.NewIdentity(
			candidate.UserID,
			user.Name,
			"api-key",
			roleNames,
			scopes,
			map[string]string{"api_key_prefix": prefix, "api_key_name": candidate.Name},
		), nil
	}

	slog.Debug("API key authentication failed: no matching key", "prefix", prefix)
	return nil, auth.ErrInvalidCredentials
}

func (a *APIKeyAuthenticator) resolveScopes(ctx context.Context, apiKey *state.APIKey) ([]string, error) {
	userScopes, err := state.GetUserScopes(ctx, a.db, apiKey.UserID)
	if err != nil {
		return nil, err
	}

	if !apiKey.ScopeRestricted {
		return userScopes, nil
	}

	keyScopes, err := apiKey.ScopesList()
	if err != nil {
		return nil, err
	}
	if len(keyScopes) == 0 {
		return nil, nil
	}

	userScopeSet := make(map[string]struct{}, len(userScopes))
	for _, s := range userScopes {
		userScopeSet[s] = struct{}{}
	}

	effective := make([]string, 0, len(keyScopes))
	for _, s := range keyScopes {
		if _, ok := userScopeSet[s]; ok {
			effective = append(effective, s)
		}
	}
	return effective, nil
}
