// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func createTestUser(t *testing.T, db *gorm.DB) *User {
	t.Helper()
	user, err := CreateUser(t.Context(), db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	return user
}

func TestCreateAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	apiKey, cleartext, err := CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if apiKey.ID == "" {
		t.Error("expected non-empty ID")
	}
	if apiKey.Name != "test-key" {
		t.Errorf("expected name 'test-key', got %q", apiKey.Name)
	}
	if apiKey.UserID != user.ID {
		t.Errorf("expected UserID %q, got %q", user.ID, apiKey.UserID)
	}
	if !apiKey.Active {
		t.Error("expected key to be active")
	}
	if !strings.HasPrefix(cleartext, "fdo_") {
		t.Errorf("expected cleartext to start with 'fdo_', got %q", cleartext)
	}
	if len(apiKey.Prefix) != 6 {
		t.Errorf("expected prefix length 6, got %d", len(apiKey.Prefix))
	}
	if cleartext[4:10] != apiKey.Prefix {
		t.Errorf("expected prefix %q, got %q", cleartext[4:10], apiKey.Prefix)
	}
}

func TestFindAPIKeysByPrefix(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	_, cleartext, err := CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	prefix := cleartext[4:10]
	keys, err := FindAPIKeysByPrefix(ctx, db, prefix)
	if err != nil {
		t.Fatalf("FindAPIKeysByPrefix failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

func TestCreateAPIKeyWithExpiration(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	expiry := time.Now().Add(24 * time.Hour)
	apiKey, _, err := CreateAPIKey(ctx, db, "expiring-key", user.ID, nil, &expiry)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if apiKey.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestCreateAPIKeyWithScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	scopes := []string{"vouchers:read"}
	apiKey, _, err := CreateAPIKey(ctx, db, "limited-key", user.ID, scopes, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	got, err := apiKey.ScopesList()
	if err != nil {
		t.Fatalf("ScopesList failed: %v", err)
	}
	if len(got) != 1 || got[0] != "vouchers:read" {
		t.Errorf("expected scopes [vouchers:read], got %v", got)
	}
}

func TestScopesList_Corrupted(t *testing.T) {
	key := &APIKey{Scopes: "not valid json"}
	_, err := key.ScopesList()
	if err == nil {
		t.Error("expected error for corrupted scopes JSON")
	}
}

func TestScopesList_Empty(t *testing.T) {
	key := &APIKey{Scopes: ""}
	got, err := key.ScopesList()
	if err != nil {
		t.Fatalf("ScopesList failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCreateAPIKey_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	tests := []struct {
		name   string
		kname  string
		scopes []string
	}{
		{"empty name", "", nil},
		{"invalid scope chars", "key", []string{"INVALID:SCOPE"}},
		{"scope with spaces", "key", []string{"has space"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CreateAPIKey(ctx, db, tt.kname, user.ID, tt.scopes, nil)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestCreateAPIKeyWithScopes_ScopeRestricted(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	restricted, _, err := CreateAPIKey(ctx, db, "restricted", user.ID, []string{"vouchers:read"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if !restricted.ScopeRestricted {
		t.Error("expected ScopeRestricted to be true for key with scopes")
	}

	unrestricted, _, err := CreateAPIKey(ctx, db, "unrestricted", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if unrestricted.ScopeRestricted {
		t.Error("expected ScopeRestricted to be false for key without scopes")
	}
}
