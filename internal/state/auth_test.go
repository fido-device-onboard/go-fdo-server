// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import "testing"

func TestInitAuthDB(t *testing.T) {
	db := setupAuthTestDB(t)

	for _, table := range []string{"users", "roles", "role_scopes", "user_roles", "api_keys"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestInitAuthDB_Idempotent(t *testing.T) {
	db := setupAuthTestDB(t)

	if err := InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("InitAuthDB (second call) failed: %v", err)
	}
}

func TestCascadeDeleteUser_RemovesAPIKeys(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, _, err := CreateAPIKey(ctx, db, "key1", user.ID, nil, nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, _, err := CreateAPIKey(ctx, db, "key2", user.ID, nil, nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	var count int64
	db.Model(&APIKey{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 API keys, got %d", count)
	}

	if err := db.Delete(&User{}, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

	db.Model(&APIKey{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 API keys after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteUser_RemovesUserRoles(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser: %v", err)
	}

	var count int64
	db.Model(&UserRole{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 user_role, got %d", count)
	}

	if err := db.Delete(&User{}, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

	db.Model(&UserRole{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 user_roles after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteRole_RemovesRoleScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read", "vouchers:write"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	var count int64
	db.Model(&RoleScope{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 role_scopes, got %d", count)
	}

	if err := db.Delete(&Role{}, "id = ?", role.ID).Error; err != nil {
		t.Fatalf("delete role: %v", err)
	}

	db.Model(&RoleScope{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 role_scopes after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteRole_RemovesUserRoles(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	role, err := CreateRole(ctx, db, "operator", "Ops access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser: %v", err)
	}

	var count int64
	db.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 user_role, got %d", count)
	}

	if err := db.Delete(&Role{}, "id = ?", role.ID).Error; err != nil {
		t.Fatalf("delete role: %v", err)
	}

	db.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 user_roles after cascade delete of role, got %d", count)
	}
}
