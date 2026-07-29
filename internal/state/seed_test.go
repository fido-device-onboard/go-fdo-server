// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"strings"
	"testing"
)

func TestSeedAuth_NilAdmin(t *testing.T) {
	db := setupAuthTestDB(t)
	apiKey, err := SeedAuth(t.Context(), db, nil, []string{"vouchers:read"}, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if apiKey != "" {
		t.Errorf("expected empty apiKey for nil admin, got %q", apiKey)
	}
}

func TestSeedAuth_CreatesAdminUserAndKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	scopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete", "vouchers:extend"}
	admin := &SeedAdminConfig{
		Name:  "admin",
		Email: "admin@example.com",
	}

	apiKey, err := SeedAuth(ctx, db, admin, scopes, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if !strings.HasPrefix(apiKey, "fdo_") {
		t.Errorf("apiKey = %q, want fdo_ prefix", apiKey)
	}

	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}
}

func TestSeedAuth_SkipsIfUsersExist(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	if _, err := CreateUser(ctx, db, "existing", "existing@example.com"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	admin := &SeedAdminConfig{
		Name:  "admin",
		Email: "admin@example.com",
	}

	apiKey, err := SeedAuth(ctx, db, admin, nil, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if apiKey != "" {
		t.Errorf("expected empty apiKey, got %q", apiKey)
	}
}

func TestSeedAuth_ForceCreatesWhenUsersExist(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	scopes := []string{"vouchers:read"}

	admin1 := &SeedAdminConfig{Name: "admin", Email: "admin@example.com"}
	if _, err := SeedAuth(ctx, db, admin1, scopes, false); err != nil {
		t.Fatalf("SeedAuth (first): %v", err)
	}

	admin2 := &SeedAdminConfig{Name: "admin2", Email: "admin2@example.com"}
	apiKey, err := SeedAuth(ctx, db, admin2, scopes, true)
	if err != nil {
		t.Fatalf("SeedAuth (force): %v", err)
	}
	if apiKey == "" {
		t.Error("expected non-empty apiKey with --force")
	}

	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 2 {
		t.Errorf("user count = %d, want 2", count)
	}
}

func TestSeedAuth_PresetAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	admin := &SeedAdminConfig{
		Name:   "admin",
		Email:  "admin@example.com",
		APIKey: "fdo_a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5P6",
	}

	apiKey, err := SeedAuth(ctx, db, admin, []string{"vouchers:read"}, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if apiKey != admin.APIKey {
		t.Errorf("apiKey = %q, want %q", apiKey, admin.APIKey)
	}
}

func TestSeedAuth_InvalidPresetAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name string
		key  string
	}{
		{"missing prefix", "bad_a1B2c3D4e5F6"},
		{"too short", "fdo_abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admin := &SeedAdminConfig{
				Name:   "admin",
				Email:  "admin@example.com",
				APIKey: tt.key,
			}
			_, err := SeedAuth(ctx, db, admin, []string{"vouchers:read"}, true)
			if err == nil {
				t.Error("expected error for invalid preset API key")
			}
		})
	}
}
