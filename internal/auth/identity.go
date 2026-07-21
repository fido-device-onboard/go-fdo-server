// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"errors"
)

type contextKey struct{}

var ErrInvalidCredentials = errors.New("invalid credentials")

type Identity struct {
	subject    string
	name       string
	authMethod string
	roles      []string
	scopes     []string
	metadata   map[string]string
}

func NewIdentity(subject, name, authMethod string, roles, scopes []string, metadata map[string]string) *Identity {
	id := &Identity{
		subject:    subject,
		name:       name,
		authMethod: authMethod,
	}
	if roles != nil {
		id.roles = make([]string, len(roles))
		copy(id.roles, roles)
	}
	if scopes != nil {
		id.scopes = make([]string, len(scopes))
		copy(id.scopes, scopes)
	}
	if metadata != nil {
		id.metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			id.metadata[k] = v
		}
	}
	return id
}

func (i *Identity) Subject() string    { return i.subject }
func (i *Identity) Name() string       { return i.name }
func (i *Identity) AuthMethod() string { return i.authMethod }

func (i *Identity) Roles() []string {
	out := make([]string, len(i.roles))
	copy(out, i.roles)
	return out
}

func (i *Identity) Scopes() []string {
	out := make([]string, len(i.scopes))
	copy(out, i.scopes)
	return out
}

func (i *Identity) Metadata() map[string]string {
	out := make(map[string]string, len(i.metadata))
	for k, v := range i.metadata {
		out[k] = v
	}
	return out
}

func (i *Identity) HasAllScopes(required []string) bool {
	if len(required) == 0 {
		return true
	}
	if i == nil || len(i.scopes) < len(required) {
		return false
	}
	have := make(map[string]struct{}, len(i.scopes))
	for _, s := range i.scopes {
		have[s] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			return false
		}
	}
	return true
}

func ContextWithIdentity(ctx context.Context, id *Identity) context.Context {
	if id == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, id)
}

// IdentityFromContext retrieves the authenticated identity from the context.
// Returns (nil, false) when no identity is present.
func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(contextKey{}).(*Identity)
	return id, ok
}
