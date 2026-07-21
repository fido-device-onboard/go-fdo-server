// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"slices"
	"testing"
)

func TestCreateRole(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if role.ID == "" {
		t.Error("expected non-empty ID")
	}
	if role.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", role.Name)
	}
	if !role.BuiltIn {
		t.Error("expected BuiltIn to be true")
	}
}

func TestAssignRoleAndGetScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	expectedScopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete"}
	role, err := CreateRole(ctx, db, "admin", "Full access", true, expectedScopes)
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	user, err := CreateUser(ctx, db, "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	roles, err := GetUserRoles(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("GetUserRoles failed: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
	if roles[0].Name != "admin" {
		t.Errorf("expected role name 'admin', got %q", roles[0].Name)
	}

	scopes, err := GetUserScopes(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("GetUserScopes failed: %v", err)
	}
	slices.Sort(scopes)
	slices.Sort(expectedScopes)
	if !slices.Equal(scopes, expectedScopes) {
		t.Errorf("expected scopes %v, got %v", expectedScopes, scopes)
	}
}

func TestFindOrCreateRole(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	role1, err := FindOrCreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("FindOrCreateRole (create) failed: %v", err)
	}

	role2, err := FindOrCreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("FindOrCreateRole (find) failed: %v", err)
	}

	if role1.ID != role2.ID {
		t.Errorf("expected same role ID, got %q and %q", role1.ID, role2.ID)
	}
}

func TestCreateRole_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	_, err := CreateRole(ctx, db, "", "description", false, nil)
	if err == nil {
		t.Error("expected validation error for empty role name")
	}

	_, err = CreateRole(ctx, db, "admin", "description", false, []string{"INVALID:SCOPE"})
	if err == nil {
		t.Error("expected validation error for invalid scope format")
	}
}
