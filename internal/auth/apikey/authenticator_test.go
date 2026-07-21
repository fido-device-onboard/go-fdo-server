// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package apikey

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if err := state.InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("Failed to initialize auth database: %v", err)
	}
	return db
}

func TestAuthenticate_NoHeader(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := authn.Authenticate(r.Context(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_EmptyHeader(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "")
	id, err := authn.Authenticate(r.Context(), r)
	if id != nil {
		t.Errorf("expected nil identity for empty header, got %+v", id)
	}
	if err != nil {
		t.Errorf("expected nil error for empty header (skip to next authenticator), got %v", err)
	}
}

func TestAuthenticate_InvalidFormat(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "bad_key")
	id, err := authn.Authenticate(r.Context(), r)
	if err == nil {
		t.Fatal("expected error for invalid key format")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_ValidKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if id.Subject() != user.ID {
		t.Errorf("expected subject %q, got %q", user.ID, id.Subject())
	}
	if id.AuthMethod() != "api-key" {
		t.Errorf("expected auth method 'api-key', got %q", id.AuthMethod())
	}
	expectedScopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete"}
	gotScopes := id.Scopes()
	slices.Sort(gotScopes)
	slices.Sort(expectedScopes)
	if !slices.Equal(gotScopes, expectedScopes) {
		t.Errorf("expected scopes %v, got %v", expectedScopes, gotScopes)
	}

	authn.Wait()
}

func TestAuthenticate_ExpiredKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	expired := time.Now().Add(-1 * time.Hour)
	_, cleartext, err := state.CreateAPIKey(ctx, db, "expired-key", user.ID, nil, &expired)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr == nil {
		t.Fatal("expected error for expired key")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_InactiveUser(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if err := db.WithContext(ctx).Model(&state.User{}).Where("id = ?", user.ID).Update("active", false).Error; err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr == nil {
		t.Fatal("expected error for inactive user")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_ScopedKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete", "device-ca:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "limited-key", user.ID, []string{"vouchers:read"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	gotScopes := id.Scopes()
	if len(gotScopes) != 1 || gotScopes[0] != "vouchers:read" {
		t.Errorf("expected scopes [vouchers:read], got %v", gotScopes)
	}

	authn.Wait()
}

func TestAuthenticate_InactiveKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if err := db.WithContext(ctx).Model(&state.APIKey{}).Where("prefix = ?", cleartext[4:10]).Update("active", false).Error; err != nil {
		t.Fatalf("Failed to deactivate key: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if id != nil {
		t.Errorf("expected nil identity for inactive key, got %+v", id)
	}
	if authErr == nil {
		t.Error("expected error for inactive key")
	}
}

func TestAuthenticate_ScopedKeyNoIntersection(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "limited", "Limited", false, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "no-overlap-key", user.ID, []string{"admin:special"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if len(id.Scopes()) != 0 {
		t.Errorf("expected empty scopes (no intersection), got %v", id.Scopes())
	}

	authn.Wait()
}
