// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SeedAdminConfig mirrors config.SeedAdminConfig to avoid an import cycle
// (state → config → serviceinfo → state). Keep fields in sync.
type SeedAdminConfig struct {
	Name   string
	Email  string
	APIKey string
}

// SeedAuth seeds the authentication database with initial admin user and roles.
// If admin is nil, returns immediately. If force is false and users already exist, skips seeding.
// Returns the admin API key in cleartext.
func SeedAuth(ctx context.Context, db *gorm.DB, admin *SeedAdminConfig, serverScopes []string, force bool) (string, error) {
	if admin == nil {
		return "", nil
	}

	if !force {
		count, err := UserCount(ctx, db)
		if err != nil {
			return "", err
		}
		if count > 0 {
			slog.Info("Users already exist, skipping auth seed")
			return "", nil
		}
	}

	slog.Info("Seeding auth database with initial admin user")

	// All seed operations run in a single transaction for atomicity.
	// FindOrCreateRole → CreateRole uses a nested transaction (GORM savepoint),
	// which SQLite supports. The outer transaction is the commit boundary.
	var cleartext string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		adminScopes := append(serverScopes[:len(serverScopes):len(serverScopes)], "auth:manage")
		adminRole, err := FindOrCreateRole(ctx, tx, "admin", "Full administrative access", true, adminScopes)
		if err != nil {
			return fmt.Errorf("failed to create admin role: %w", err)
		}

		var operatorScopes []string
		for _, s := range serverScopes {
			if strings.HasSuffix(s, ":read") || strings.HasSuffix(s, ":write") || s == "vouchers:extend" {
				operatorScopes = append(operatorScopes, s)
			}
		}
		if _, err := FindOrCreateRole(ctx, tx, "operator", "Operational access (create/modify, no delete or auth management)", true, operatorScopes); err != nil {
			return fmt.Errorf("failed to create operator role: %w", err)
		}

		user, err := CreateUser(ctx, tx, admin.Name, admin.Email)
		if err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}

		if err := AssignRoleToUser(ctx, tx, user.ID, adminRole.ID); err != nil {
			return fmt.Errorf("failed to assign admin role: %w", err)
		}

		if admin.APIKey != "" {
			if err := createPresetAPIKey(ctx, tx, user.ID, admin.APIKey); err != nil {
				return err
			}
			cleartext = admin.APIKey
			return nil
		}

		_, key, err := CreateAPIKey(ctx, tx, "admin-seed-key", user.ID, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to create admin API key: %w", err)
		}
		cleartext = key
		return nil
	})
	if err != nil {
		return "", err
	}

	return cleartext, nil
}

// createPresetAPIKey must be called within the SeedAuth transaction after CreateUser,
// so the userID foreign key is guaranteed to exist.
func createPresetAPIKey(ctx context.Context, db *gorm.DB, userID, cleartext string) error {
	if !strings.HasPrefix(cleartext, "fdo_") || len(cleartext) < 10 {
		return fmt.Errorf("invalid preset API key: must start with 'fdo_' and be at least 10 characters")
	}

	hash := sha256.Sum256([]byte(cleartext))
	hashed := hash[:]

	prefix := cleartext[4:10]
	apiKey := &APIKey{
		ID:        uuid.New().String(),
		Prefix:    prefix,
		HashedKey: hashed,
		Name:      "admin-seed-key",
		UserID:    userID,
		Active:    true,
	}

	if err := db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return fmt.Errorf("failed to create preset API key: %w", err)
	}
	return nil
}
