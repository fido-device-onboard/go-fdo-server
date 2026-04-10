// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// setupTestDB creates a temporary SQLite database for testing
func setupCBORTestDB(t *testing.T) (*RvInfoState, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "rvinfo_cbor_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	gormDB, err := gorm.Open(sqlite.Open(tmpFile.Name()), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}

	rvInfoState, err := InitRvInfoDB(gormDB)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo state: %v", err)
	}

	cleanup := func() {
		sqlDB.Close()
		os.Remove(tmpFile.Name())
	}

	return rvInfoState, cleanup
}

// mustCBORMarshal marshals a value to CBOR or fails the test
func mustCBORMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := cbor.Marshal(v)
	if err != nil {
		t.Fatalf("cbor.Marshal failed: %v", err)
	}
	return data
}

// TestCreateRVInfo_StoresCBOR verifies state layer stores CBOR, not JSON
func TestCreateRVInfo_StoresCBOR(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Manually construct RV instructions (state layer tests don't depend on JSON parsing)
	rvInstructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
		{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
	}}

	// Insert via state layer
	if err := state.CreateRVInfo(ctx, rvInstructions); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Read raw bytes from database
	var rvInfoRow RvInfo
	if err := state.DB.WithContext(ctx).Where("id = ?", 1).First(&rvInfoRow).Error; err != nil {
		t.Fatalf("failed to read from database: %v", err)
	}

	// Verify it's valid CBOR (can be unmarshaled)
	var rvInfo [][]protocol.RvInstruction
	if err := cbor.Unmarshal(rvInfoRow.Value, &rvInfo); err != nil {
		t.Errorf("Expected CBOR-encoded data, got error: %v", err)
	}

	// Verify it's NOT valid JSON
	var jsonTest interface{}
	if err := json.Unmarshal(rvInfoRow.Value, &jsonTest); err == nil {
		t.Error("Data should be CBOR, not JSON - JSON unmarshal should fail")
	}

	// Verify decoded structure is correct
	if len(rvInfo) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(rvInfo))
	}
	if len(rvInfo[0]) != 3 {
		t.Errorf("Expected 3 instructions, got %d", len(rvInfo[0]))
	}
}

// TestGetRVInfo_RetrievesProtocolStructs tests fetching RV info as protocol.RvInstruction
func TestGetRVInfo_RetrievesProtocolStructs(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	rvInstructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
		{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
	}}

	if err := state.CreateRVInfo(ctx, rvInstructions); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Fetch it back
	retrieved, err := state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo failed: %v", err)
	}

	// Verify structure
	if len(retrieved) != 1 {
		t.Fatalf("Expected 1 directive, got %d", len(retrieved))
	}
	if len(retrieved[0]) != 3 {
		t.Fatalf("Expected 3 instructions, got %d", len(retrieved[0]))
	}

	// Verify instruction types
	if retrieved[0][0].Variable != protocol.RVDns {
		t.Errorf("Expected RVDns, got %v", retrieved[0][0].Variable)
	}
	if retrieved[0][1].Variable != protocol.RVProtocol {
		t.Errorf("Expected RVProtocol, got %v", retrieved[0][1].Variable)
	}
	if retrieved[0][2].Variable != protocol.RVOwnerPort {
		t.Errorf("Expected RVOwnerPort, got %v", retrieved[0][2].Variable)
	}

	// Verify values can be unmarshaled correctly
	var dns string
	if err := cbor.Unmarshal(retrieved[0][0].Value, &dns); err != nil {
		t.Fatalf("Failed to unmarshal DNS: %v", err)
	}
	if dns != "rv.example.com" {
		t.Errorf("Expected DNS 'rv.example.com', got %q", dns)
	}

	var port uint16
	if err := cbor.Unmarshal(retrieved[0][2].Value, &port); err != nil {
		t.Fatalf("Failed to unmarshal port: %v", err)
	}
	if port != 8443 {
		t.Errorf("Expected port 8443, got %d", port)
	}
}

// TestGetRVInfo_NotFound tests fetching when no data exists
func TestGetRVInfo_NotFound(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	_, err := state.GetRVInfo(ctx)
	if err != ErrRvInfoNotFound {
		t.Errorf("Expected ErrRvInfoNotFound, got %v", err)
	}
}

// TestUpdateRVInfo_UpdatesExistingData tests updating RV info
func TestUpdateRVInfo_UpdatesExistingData(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert initial data
	initial := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv1.example.com")},
	}}
	if err := state.CreateRVInfo(ctx, initial); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Update with new data
	updated := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv2.example.com")},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTP))},
	}}
	if err := state.UpdateRVInfo(ctx, updated); err != nil {
		t.Fatalf("UpdateRVInfo failed: %v", err)
	}

	// Fetch and verify
	retrieved, err := state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo failed: %v", err)
	}

	if len(retrieved) != 1 || len(retrieved[0]) != 2 {
		t.Fatalf("Expected 1 directive with 2 instructions, got %d directives", len(retrieved))
	}

	// Verify updated DNS value
	var dns string
	if err := cbor.Unmarshal(retrieved[0][0].Value, &dns); err != nil {
		t.Fatalf("Failed to unmarshal DNS: %v", err)
	}
	if dns != "rv2.example.com" {
		t.Errorf("Expected DNS 'rv2.example.com', got %q", dns)
	}
}

// TestUpdateRVInfo_NotFound tests updating when no data exists
func TestUpdateRVInfo_NotFound(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	updated := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
	}}
	err := state.UpdateRVInfo(ctx, updated)
	if err != ErrRvInfoNotFound {
		t.Errorf("Expected ErrRvInfoNotFound, got %v", err)
	}
}

// TestDeleteRvInfo_RemovesData tests deleting RV info
func TestDeleteRvInfo_RemovesData(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert data
	rvInstructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
	}}
	if err := state.CreateRVInfo(ctx, rvInstructions); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Delete it
	if err := state.DeleteRvInfo(ctx); err != nil {
		t.Fatalf("DeleteRvInfo failed: %v", err)
	}

	// Verify it's gone
	_, err := state.GetRVInfo(ctx)
	if err != ErrRvInfoNotFound {
		t.Errorf("Expected ErrRvInfoNotFound, got %v", err)
	}
}

// TestDeleteRvInfo_NotFound tests deleting when no data exists
func TestDeleteRvInfo_NotFound(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := state.DeleteRvInfo(ctx)
	if err != ErrRvInfoNotFound {
		t.Errorf("Expected ErrRvInfoNotFound, got %v", err)
	}
}

// TestCreateOrUpdateRVInfo_AtomicInsertOrUpdate tests atomic upsert operation
func TestCreateOrUpdateRVInfo_AtomicInsertOrUpdate(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First upsert - should create new record
	initial := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
		{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
	}}

	if err := state.CreateOrUpdateRVInfo(ctx, initial); err != nil {
		t.Fatalf("First CreateOrUpdateRVInfo failed: %v", err)
	}

	// Verify it was created
	retrieved, err := state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo failed: %v", err)
	}
	if len(retrieved) != 1 || len(retrieved[0]) != 3 {
		t.Fatalf("Expected 1 directive with 3 instructions, got %d directives", len(retrieved))
	}

	// Second upsert - should update existing record
	updated := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv2.example.com")},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTP))},
	}}

	if err := state.CreateOrUpdateRVInfo(ctx, updated); err != nil {
		t.Fatalf("Second CreateOrUpdateRVInfo failed: %v", err)
	}

	// Verify it was updated
	retrieved, err = state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo after update failed: %v", err)
	}
	if len(retrieved) != 1 || len(retrieved[0]) != 2 {
		t.Fatalf("Expected 1 directive with 2 instructions, got %d directives", len(retrieved))
	}

	// Verify updated DNS value
	var dns string
	if err := cbor.Unmarshal(retrieved[0][0].Value, &dns); err != nil {
		t.Fatalf("Failed to unmarshal DNS: %v", err)
	}
	if dns != "rv2.example.com" {
		t.Errorf("Expected DNS 'rv2.example.com', got %q", dns)
	}
}

// TestMultipleDirectives verifies handling of multiple RV directives
func TestMultipleDirectives(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert multiple directives
	rvInstructions := [][]protocol.RvInstruction{
		{
			{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv1.example.com")},
			{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
			{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
		},
		{
			{Variable: protocol.RVIPAddress, Value: mustCBORMarshal(t, []byte{192, 168, 1, 100})},
			{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTP))},
			{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8080))},
		},
	}

	if err := state.CreateRVInfo(ctx, rvInstructions); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Fetch and verify
	retrieved, err := state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 directives, got %d", len(retrieved))
	}

	// Verify first directive
	if len(retrieved[0]) != 3 {
		t.Errorf("Expected 3 instructions in first directive, got %d", len(retrieved[0]))
	}
	var dns string
	if err := cbor.Unmarshal(retrieved[0][0].Value, &dns); err != nil {
		t.Fatalf("Failed to unmarshal DNS: %v", err)
	}
	if dns != "rv1.example.com" {
		t.Errorf("Expected DNS 'rv1.example.com', got %q", dns)
	}

	// Verify second directive
	if len(retrieved[1]) != 3 {
		t.Errorf("Expected 3 instructions in second directive, got %d", len(retrieved[1]))
	}
}

// TestAllInstructionTypes verifies all RV instruction types can be stored/retrieved
func TestAllInstructionTypes(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create instructions with all field types
	rvInstructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
		{Variable: protocol.RVIPAddress, Value: mustCBORMarshal(t, []byte{192, 168, 1, 100})},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
		{Variable: protocol.RVDevPort, Value: mustCBORMarshal(t, uint16(8041))},
		{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
		{Variable: protocol.RVBypass, Value: nil}, // Flag only
		{Variable: protocol.RVDelaysec, Value: mustCBORMarshal(t, uint32(120))},
		{Variable: protocol.RVMedium, Value: mustCBORMarshal(t, uint8(protocol.RVMedWifiAll))},
		{Variable: protocol.RVWifiSsid, Value: mustCBORMarshal(t, "TestNetwork")},
		{Variable: protocol.RVWifiPw, Value: mustCBORMarshal(t, "password123")},
		{Variable: protocol.RVDevOnly, Value: nil},
		{Variable: protocol.RVOwnerOnly, Value: nil},
		{Variable: protocol.RVUserInput, Value: nil},
		{Variable: protocol.RVExtRV, Value: mustCBORMarshal(t, []string{"ext1", "ext2"})},
		{Variable: protocol.RVSvCertHash, Value: mustCBORMarshal(t, []byte{0xAA, 0xBB, 0xCC})},
		{Variable: protocol.RVClCertHash, Value: mustCBORMarshal(t, []byte{0xDD, 0xEE, 0xFF})},
	}}

	if err := state.CreateRVInfo(ctx, rvInstructions); err != nil {
		t.Fatalf("CreateRVInfo failed: %v", err)
	}

	// Fetch and verify
	retrieved, err := state.GetRVInfo(ctx)
	if err != nil {
		t.Fatalf("GetRVInfo failed: %v", err)
	}

	if len(retrieved) != 1 {
		t.Fatalf("Expected 1 directive, got %d", len(retrieved))
	}
	if len(retrieved[0]) != 16 {
		t.Fatalf("Expected 16 instructions, got %d", len(retrieved[0]))
	}

	// Verify all instruction types are present
	expectedVars := []protocol.RvVar{
		protocol.RVDns,
		protocol.RVIPAddress,
		protocol.RVProtocol,
		protocol.RVDevPort,
		protocol.RVOwnerPort,
		protocol.RVBypass,
		protocol.RVDelaysec,
		protocol.RVMedium,
		protocol.RVWifiSsid,
		protocol.RVWifiPw,
		protocol.RVDevOnly,
		protocol.RVOwnerOnly,
		protocol.RVUserInput,
		protocol.RVExtRV,
		protocol.RVSvCertHash,
		protocol.RVClCertHash,
	}

	for i, expectedVar := range expectedVars {
		if retrieved[0][i].Variable != expectedVar {
			t.Errorf("Instruction[%d]: expected variable %v, got %v", i, expectedVar, retrieved[0][i].Variable)
		}
	}
}
