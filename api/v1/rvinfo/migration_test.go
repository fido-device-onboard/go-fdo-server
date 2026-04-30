// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"testing"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateJSONToCBOR_NoData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvInfoState, err := state.InitRvInfoDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo database: %v", err)
	}

	server := NewServer(rvInfoState)
	if err = server.MigrateJSONToCBOR(context.Background()); err != nil {
		t.Errorf("Expected no error for empty database, got: %v", err)
	}
}

func TestMigrateJSONToCBOR_AlreadyCBOR(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvInfoState, err := state.InitRvInfoDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo database: %v", err)
	}

	// Insert data that's already in CBOR format
	rvInstructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: must(cbor.Marshal("rv.example.com"))},
		{Variable: protocol.RVProtocol, Value: must(cbor.Marshal(uint8(protocol.RVProtHTTP)))},
	}}

	err = rvInfoState.CreateRvInfo(context.Background(), rvInstructions)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Migration should detect it's already CBOR and do nothing
	server := NewServer(rvInfoState)
	if err = server.MigrateJSONToCBOR(context.Background()); err != nil {
		t.Errorf("Expected no error for CBOR data, got: %v", err)
	}
}

func TestMigrateJSONToCBOR_V1JSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvInfoState, err := state.InitRvInfoDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo database: %v", err)
	}

	// Insert V1 JSON data directly into database
	v1JSON := []byte(`[{"dns":"rv.example.com","protocol":"http","owner_port":"8080"}]`)
	result := db.Exec("INSERT INTO rvinfo (id, value) VALUES (?, ?)", 1, v1JSON)
	if result.Error != nil {
		t.Fatalf("Failed to insert V1 JSON: %v", result.Error)
	}

	server := NewServer(rvInfoState)
	if err = server.MigrateJSONToCBOR(context.Background()); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify it's now CBOR by fetching it
	rvInstructions, err := rvInfoState.GetRvInfo(context.Background())
	if err != nil {
		t.Fatalf("Failed to fetch after migration: %v", err)
	}

	// Verify structure is correct
	if len(rvInstructions) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(rvInstructions))
	}

	if len(rvInstructions[0]) != 3 { // dns, protocol, owner_port
		t.Errorf("Expected 3 instructions, got %d", len(rvInstructions[0]))
	}
}

// must is a helper for tests - panics on error
func must(data []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return data
}
