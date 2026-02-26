// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestTokenDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Enable foreign key constraints in SQLite (required for CASCADE DELETE)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	return db
}

func TestCleanupExpiredSessions(t *testing.T) {
	db := setupTestTokenDB(t)
	tokenService, err := InitTokenServiceDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize token service DB: %v", err)
	}

	ctx := context.Background()

	// Create a new session (this will be the stale one)
	staleToken, err := tokenService.NewToken(ctx, protocol.TO0Protocol)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Verify session exists
	var count int64
	db.Model(&Session{}).Count(&count)
	if count != 1 {
		t.Fatalf("Expected 1 session, got %d", count)
	}

	// Manually update the session to be old
	db.Model(&Session{}).Where("protocol = ?", protocol.TO0Protocol).
		Update("created_at", time.Now().Add(-2*time.Hour))

	// Create a fresh session
	freshToken, err := tokenService.NewToken(ctx, protocol.TO1Protocol)
	if err != nil {
		t.Fatalf("Failed to create second token: %v", err)
	}

	// Verify both sessions exist
	db.Model(&Session{}).Count(&count)
	if count != 2 {
		t.Fatalf("Expected 2 sessions, got %d", count)
	}

	// Cleanup sessions older than 1 hour
	deletedCount, err := tokenService.CleanupExpiredSessions(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("Expected 1 session to be deleted, got %d", deletedCount)
	}

	// Verify only the fresh session remains
	db.Model(&Session{}).Count(&count)
	if count != 1 {
		t.Fatalf("Expected 1 session after cleanup, got %d", count)
	}

	// Verify the remaining session is the fresh one
	var session Session
	db.First(&session)
	if session.Protocol != int(protocol.TO1Protocol) {
		t.Fatalf("Remaining session has wrong protocol: %d", session.Protocol)
	}

	// Verify the stale token is invalid (session was deleted)
	staleCtx := tokenService.TokenContext(ctx, staleToken)
	_, err = tokenService.getSessionID(staleCtx)
	if err == nil {
		t.Fatal("Expected stale token to be invalid after cleanup")
	}
	if !errors.Is(err, fdo.ErrInvalidSession) {
		t.Fatalf("Expected ErrInvalidSession for stale token, got: %v", err)
	}

	// Verify the fresh token is still valid
	freshCtx := tokenService.TokenContext(ctx, freshToken)
	sessionID, err := tokenService.getSessionID(freshCtx)
	if err != nil {
		t.Fatalf("Fresh token should be valid, got error: %v", err)
	}
	if sessionID == nil {
		t.Fatal("Expected valid session ID for fresh token")
	}
}

func TestSessionCreatedAtTimestamp(t *testing.T) {
	db := setupTestTokenDB(t)
	tokenService, err := InitTokenServiceDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize token service DB: %v", err)
	}

	ctx := context.Background()

	before := time.Now()
	_, err = tokenService.NewToken(ctx, protocol.TO0Protocol)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}
	after := time.Now()

	// Verify the session has a created_at timestamp
	var session Session
	db.First(&session)

	if session.CreatedAt.IsZero() {
		t.Fatal("Session created_at is zero")
	}

	if session.CreatedAt.Before(before) || session.CreatedAt.After(after) {
		t.Fatalf("Session created_at %v is outside expected range [%v, %v]",
			session.CreatedAt, before, after)
	}
}
