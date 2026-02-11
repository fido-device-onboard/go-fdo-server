// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCascadeTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Enable foreign key constraints in SQLite (disabled by default!)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	return db
}

func TestCascadeDeleteTO0Sessions(t *testing.T) {
	db := setupCascadeTestDB(t)
	ctx := context.Background()

	// Initialize token service (creates sessions table)
	tokenService, err := InitTokenServiceDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize token service: %v", err)
	}

	// Initialize TO0 session state (creates to0_sessions table with FK)
	to0State, err := InitTO0SessionDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize TO0 session DB: %v", err)
	}

	// Create a session with TO0 protocol
	token, err := tokenService.NewToken(ctx, protocol.TO0Protocol)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Add token to context
	ctx = tokenService.TokenContext(ctx, token)

	// Create a TO0 session entry
	nonce := protocol.Nonce{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	if err := to0State.SetTO0SignNonce(ctx, nonce); err != nil {
		t.Fatalf("Failed to set TO0 sign nonce: %v", err)
	}

	// Verify TO0 session exists
	var to0Count int64
	db.Model(&TO0Session{}).Count(&to0Count)
	if to0Count != 1 {
		t.Fatalf("Expected 1 TO0 session, got %d", to0Count)
	}

	// Delete the parent session
	sessionID, err := tokenService.getSessionID(ctx)
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	if err := db.Delete(&Session{}, "id = ?", sessionID).Error; err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Verify TO0 session was automatically deleted via CASCADE
	db.Model(&TO0Session{}).Count(&to0Count)
	if to0Count != 0 {
		t.Errorf("Expected 0 TO0 sessions after cascade delete, got %d", to0Count)
		t.Error("CASCADE DELETE is not working - TO0 sessions are orphaned")
	}
}

func TestCascadeDeleteTO1Sessions(t *testing.T) {
	db := setupCascadeTestDB(t)
	ctx := context.Background()

	// Initialize token service
	tokenService, err := InitTokenServiceDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize token service: %v", err)
	}

	// Initialize TO1 session state
	to1State, err := InitTO1SessionDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize TO1 session DB: %v", err)
	}

	// Create a session with TO1 protocol
	token, err := tokenService.NewToken(ctx, protocol.TO1Protocol)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	ctx = tokenService.TokenContext(ctx, token)

	// Create a TO1 session entry
	nonce := protocol.Nonce{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if err := to1State.SetTO1ProofNonce(ctx, nonce); err != nil {
		t.Fatalf("Failed to set TO1 proof nonce: %v", err)
	}

	// Verify TO1 session exists
	var to1Count int64
	db.Model(&TO1Session{}).Count(&to1Count)
	if to1Count != 1 {
		t.Fatalf("Expected 1 TO1 session, got %d", to1Count)
	}

	// Delete the parent session
	sessionID, err := tokenService.getSessionID(ctx)
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	if err := db.Delete(&Session{}, "id = ?", sessionID).Error; err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Verify TO1 session was automatically deleted via CASCADE
	db.Model(&TO1Session{}).Count(&to1Count)
	if to1Count != 0 {
		t.Errorf("Expected 0 TO1 sessions after cascade delete, got %d", to1Count)
		t.Error("CASCADE DELETE is not working - TO1 sessions are orphaned")
	}
}

func TestSessionCleanupCascadesToProtocolSessions(t *testing.T) {
	db := setupCascadeTestDB(t)
	ctx := context.Background()

	// Initialize all session states
	tokenService, err := InitTokenServiceDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize token service: %v", err)
	}

	to0State, err := InitTO0SessionDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize TO0 session DB: %v", err)
	}

	to1State, err := InitTO1SessionDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize TO1 session DB: %v", err)
	}

	// Create an old TO0 session
	token0, err := tokenService.NewToken(ctx, protocol.TO0Protocol)
	if err != nil {
		t.Fatalf("Failed to create TO0 token: %v", err)
	}
	ctx0 := tokenService.TokenContext(ctx, token0)
	if err := to0State.SetTO0SignNonce(ctx0, protocol.Nonce{1, 2, 3}); err != nil {
		t.Fatalf("Failed to set TO0 nonce: %v", err)
	}

	// Create an old TO1 session
	token1, err := tokenService.NewToken(ctx, protocol.TO1Protocol)
	if err != nil {
		t.Fatalf("Failed to create TO1 token: %v", err)
	}
	ctx1 := tokenService.TokenContext(ctx, token1)
	if err := to1State.SetTO1ProofNonce(ctx1, protocol.Nonce{4, 5, 6}); err != nil {
		t.Fatalf("Failed to set TO1 nonce: %v", err)
	}

	// Make both sessions old by updating their created_at timestamp
	db.Model(&Session{}).Where("protocol = ?", protocol.TO0Protocol).
		Update("created_at", db.NowFunc().Add(-2*time.Hour))
	db.Model(&Session{}).Where("protocol = ?", protocol.TO1Protocol).
		Update("created_at", db.NowFunc().Add(-2*time.Hour))

	// Verify both protocol sessions exist
	var to0Count, to1Count int64
	db.Model(&TO0Session{}).Count(&to0Count)
	db.Model(&TO1Session{}).Count(&to1Count)
	if to0Count != 1 || to1Count != 1 {
		t.Fatalf("Expected 1 TO0 and 1 TO1 session, got %d and %d", to0Count, to1Count)
	}

	// Run session cleanup (should delete both old sessions)
	deletedCount, err := tokenService.CleanupExpiredSessions(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("Expected 2 sessions deleted, got %d", deletedCount)
	}

	// Verify protocol sessions were CASCADE deleted
	db.Model(&TO0Session{}).Count(&to0Count)
	db.Model(&TO1Session{}).Count(&to1Count)
	if to0Count != 0 {
		t.Errorf("Expected 0 TO0 sessions after cleanup, got %d - CASCADE DELETE not working", to0Count)
	}
	if to1Count != 0 {
		t.Errorf("Expected 0 TO1 sessions after cleanup, got %d - CASCADE DELETE not working", to1Count)
	}
}
