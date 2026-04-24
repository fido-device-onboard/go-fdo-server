// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
)

func TestCleanupModules_RemovesDirsOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "guid1")
	dir2 := filepath.Join(tmpDir, "guid2")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}

	dbState := &db.State{}
	token := "test-token"
	ctx := dbState.TokenContext(context.Background(), token)

	sm := moduleStateMachines{
		DB: dbState,
		states: map[string]*moduleStateMachineState{
			token: {
				Completed:  false,
				UploadDirs: []string{dir1, dir2},
				Stop:       func() {},
			},
		},
	}

	sm.CleanupModules(ctx)

	for _, dir := range []string{dir1, dir2} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected directory %s to be removed, but it still exists", dir)
		}
	}
}

func TestCleanupModules_KeepsDirsOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "guid1")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}

	dbState := &db.State{}
	token := "test-token"
	ctx := dbState.TokenContext(context.Background(), token)

	sm := moduleStateMachines{
		DB: dbState,
		states: map[string]*moduleStateMachineState{
			token: {
				Completed:  true,
				UploadDirs: []string{dir1},
				Stop:       func() {},
			},
		},
	}

	sm.CleanupModules(ctx)

	if _, err := os.Stat(dir1); os.IsNotExist(err) {
		t.Error("expected directory to be kept on successful completion, but it was removed")
	}
}

func TestCleanupModules_NoStateNoPanic(t *testing.T) {
	dbState := &db.State{}
	token := "unknown-token"
	ctx := dbState.TokenContext(context.Background(), token)

	sm := moduleStateMachines{
		DB:     dbState,
		states: map[string]*moduleStateMachineState{},
	}

	// Should return silently without panicking
	sm.CleanupModules(ctx)
}

func TestCleanupModules_SweepsOrphanedStates(t *testing.T) {
	// Set up a real in-memory DB so SessionExists works
	dbState, err := db.InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}

	// Create a session for the "active" token (the one being cleaned up normally)
	activeToken, err := dbState.NewToken(context.Background(), protocol.TO2Protocol)
	if err != nil {
		t.Fatal(err)
	}
	activeCtx := dbState.TokenContext(context.Background(), activeToken)

	// Create a session for the "orphan" token, then invalidate it
	// (simulates transport layer invalidating on client disconnect)
	orphanToken, err := dbState.NewToken(context.Background(), protocol.TO2Protocol)
	if err != nil {
		t.Fatal(err)
	}
	orphanCtx := dbState.TokenContext(context.Background(), orphanToken)
	if err := dbState.InvalidateToken(orphanCtx); err != nil {
		t.Fatal(err)
	}

	// Create upload dirs for the orphaned session
	tmpDir := t.TempDir()
	orphanDir := filepath.Join(tmpDir, "orphan-guid")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sm := moduleStateMachines{
		DB: dbState,
		states: map[string]*moduleStateMachineState{
			activeToken: {
				Completed: true,
				Stop:      func() {},
			},
			orphanToken: {
				Completed:  false,
				UploadDirs: []string{orphanDir},
				Stop:       func() {},
			},
		},
	}

	// CleanupModules for the active session should also sweep the orphan
	sm.CleanupModules(activeCtx)

	// Orphan's upload dir should be removed
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Error("expected orphaned upload directory to be removed by sweep")
	}

	// Both entries should be gone from the states map
	if len(sm.states) != 0 {
		t.Errorf("expected states map to be empty, got %d entries", len(sm.states))
	}
}
