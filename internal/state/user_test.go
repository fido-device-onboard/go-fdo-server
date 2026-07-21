// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"fmt"
	"strings"
	"testing"

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
	if err := InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("Failed to initialize auth database: %v", err)
	}
	return db
}

func TestCreateUser(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.ID == "" {
		t.Error("expected non-empty ID")
	}
	if user.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", user.Name)
	}
	if user.Email != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got %q", user.Email)
	}
	if !user.Active {
		t.Error("expected user to be active")
	}
}

func TestGetUserByID(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	created, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	found, err := GetUserByID(ctx, db, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
	if found.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", found.Name)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	db := setupAuthTestDB(t)
	_, err := GetUserByID(t.Context(), db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestUserCount(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	if _, err := CreateUser(ctx, db, "admin", "admin@example.com"); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	count, err = UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestCreateUser_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name  string
		uname string
		email string
	}{
		{"empty name", "", "test@example.com"},
		{"empty email", "admin", ""},
		{"invalid email", "admin", "not-an-email"},
		{"display name form", "admin", "Admin <admin@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateUser(ctx, db, tt.uname, tt.email)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
