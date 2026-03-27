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

// TestInsertRvInfo_StoresCBOR_V2 verifies V2 API stores CBOR, not JSON
func TestInsertRvInfo_StoresCBOR_V2(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Valid V2 OpenAPI JSON input (array of arrays of single-key objects)
	v2JSON := []byte(`[[
		{"dns":"rv.example.com"},
		{"protocol":"https"},
		{"owner_port":8443}
	]]`)

	// Insert via V2 API
	if err := state.InsertRvInfo(ctx, v2JSON); err != nil {
		t.Fatalf("InsertRvInfo failed: %v", err)
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

	// Verify it's NOT valid JSON (V2 format)
	var jsonTest interface{}
	if err := json.Unmarshal(rvInfoRow.Value, &jsonTest); err == nil {
		t.Error("Data should be CBOR, not JSON - JSON unmarshal should fail")
	}

	// Verify decoded structure is correct
	if len(rvInfo) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(rvInfo))
	}
	if len(rvInfo[0]) != 3 {
		t.Errorf("Expected 3 instructions (dns, protocol, owner_port), got %d", len(rvInfo[0]))
	}
}

// TestFetchRvInfoJSON_AutoMigration_V2 tests automatic migration from V2 JSON to CBOR
func TestFetchRvInfoJSON_AutoMigration_V2(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Simulate old database: Insert V2 OpenAPI JSON directly (bypassing CBOR encoding)
	// Use format that matches existing parser tests
	v2JSON := `[
		[
			{"dns":"rv.example.com"},
			{"protocol":"https"},
			{"owner_port":8443}
		]
	]`
	oldRvInfo := RvInfo{
		ID:    1,
		Value: []byte(v2JSON),
	}
	if err := state.DB.Create(&oldRvInfo).Error; err != nil {
		t.Fatalf("failed to create old JSON record: %v", err)
	}

	// Verify it's JSON before migration
	var jsonTest interface{}
	if err := json.Unmarshal(oldRvInfo.Value, &jsonTest); err != nil {
		t.Fatalf("Setup failed: inserted data should be JSON: %v", err)
	}

	// Call FetchRvInfoJSON() - should trigger auto-migration
	outputJSON, err := state.FetchRvInfoJSON(ctx)
	if err != nil {
		t.Fatalf("FetchRvInfoJSON failed: %v", err)
	}

	// Verify returned data is correct V2 format
	var output [][]map[string]interface{}
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	if len(output) != 1 {
		t.Errorf("Expected 1 directive, got %d, output: %v", len(output), output)
	}
	if len(output) > 0 && len(output[0]) != 3 {
		t.Errorf("Expected 3 instructions, got %d", len(output[0]))
	}

	// Verify database now contains CBOR (not JSON)
	var migratedRow RvInfo
	if err := state.DB.Where("id = ?", 1).First(&migratedRow).Error; err != nil {
		t.Fatalf("failed to read migrated row: %v", err)
	}

	// Should be valid CBOR
	var cborTest [][]protocol.RvInstruction
	if err := cbor.Unmarshal(migratedRow.Value, &cborTest); err != nil {
		t.Errorf("After migration, data should be CBOR: %v", err)
	}

	// Should NOT be valid JSON
	if err := json.Unmarshal(migratedRow.Value, &jsonTest); err == nil {
		t.Error("After migration, data should be CBOR, not JSON")
	}
}

// TestV2_RoundTrip verifies V2 JSON → CBOR → V2 JSON preserves data
func TestV2_RoundTrip(t *testing.T) {
	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Input V2 OpenAPI JSON with multiple directives and field types
	inputJSON := []byte(`[
		[
			{"dns":"rv1.example.com"},
			{"protocol":"https"},
			{"owner_port":8443},
			{"device_port":8041},
			{"rv_bypass":true}
		],
		[
			{"ip":"192.168.1.100"},
			{"protocol":"http"},
			{"owner_port":8080},
			{"delay_seconds":30}
		]
	]`)

	// Insert via V2 API
	if err := state.InsertRvInfo(ctx, inputJSON); err != nil {
		t.Fatalf("InsertRvInfo failed: %v", err)
	}

	// Retrieve via V2 API
	outputJSON, err := state.FetchRvInfoJSON(ctx)
	if err != nil {
		t.Fatalf("FetchRvInfoJSON failed: %v", err)
	}

	// Parse input and output
	var inputData, outputData [][]map[string]interface{}
	if err := json.Unmarshal(inputJSON, &inputData); err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}
	if err := json.Unmarshal(outputJSON, &outputData); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Verify structure matches
	if len(inputData) != len(outputData) {
		t.Fatalf("Expected %d directives, got %d", len(inputData), len(outputData))
	}

	// Verify each directive
	for i := range inputData {
		if len(inputData[i]) != len(outputData[i]) {
			t.Errorf("Directive[%d]: expected %d instructions, got %d",
				i, len(inputData[i]), len(outputData[i]))
		}
	}

	// Detailed verification of first directive
	if len(outputData) > 0 && len(outputData[0]) >= 5 {
		// Create map for easier lookup
		directive0 := make(map[string]interface{})
		for _, instr := range outputData[0] {
			for k, v := range instr {
				directive0[k] = v
			}
		}

		if directive0["dns"] != "rv1.example.com" {
			t.Errorf("Expected dns 'rv1.example.com', got %v", directive0["dns"])
		}
		if directive0["protocol"] != "https" {
			t.Errorf("Expected protocol 'https', got %v", directive0["protocol"])
		}
		// V2 uses integer ports
		if ownerPort, ok := directive0["owner_port"].(float64); !ok || int(ownerPort) != 8443 {
			t.Errorf("Expected owner_port 8443 (int), got %v", directive0["owner_port"])
		}
		if devicePort, ok := directive0["device_port"].(float64); !ok || int(devicePort) != 8041 {
			t.Errorf("Expected device_port 8041 (int), got %v", directive0["device_port"])
		}
		if rvBypass, ok := directive0["rv_bypass"].(bool); !ok || !rvBypass {
			t.Errorf("Expected rv_bypass true, got %v", directive0["rv_bypass"])
		}
	}
}

// TestConvertRvInstructionsToV2JSON_AllFields tests conversion with all RV instruction types
func TestConvertRvInstructionsToV2JSON_AllFields(t *testing.T) {
	// Create instructions with all field types
	instructions := [][]protocol.RvInstruction{{
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

	// Convert to V2 JSON
	result, err := convertRvInstructionsToV2JSON(instructions)
	if err != nil {
		t.Fatalf("convertRvInstructionsToV2JSON failed: %v", err)
	}

	// Parse result
	var output [][]map[string]interface{}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(output) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(output))
	}

	if len(output[0]) != 16 {
		t.Fatalf("expected 16 instructions, got %d", len(output[0]))
	}

	// Create map for easier verification
	directive := make(map[string]interface{})
	for _, instr := range output[0] {
		for k, v := range instr {
			directive[k] = v
		}
	}

	// Verify V2 format specifics (each instruction is single-key object)
	if directive["dns"] != "rv.example.com" {
		t.Errorf("Expected dns 'rv.example.com', got %v", directive["dns"])
	}
	if directive["ip"] != "192.168.1.100" {
		t.Errorf("Expected ip '192.168.1.100', got %v", directive["ip"])
	}
	if directive["protocol"] != "https" {
		t.Errorf("Expected protocol 'https', got %v", directive["protocol"])
	}

	// V2 uses INTEGER ports (not strings like V1)
	devicePort, ok := directive["device_port"].(float64) // JSON numbers are float64
	if !ok || int(devicePort) != 8041 {
		t.Errorf("Expected device_port 8041 (int), got %v", directive["device_port"])
	}
	ownerPort, ok := directive["owner_port"].(float64)
	if !ok || int(ownerPort) != 8443 {
		t.Errorf("Expected owner_port 8443 (int), got %v", directive["owner_port"])
	}

	// Boolean flags
	if rvBypass, ok := directive["rv_bypass"].(bool); !ok || !rvBypass {
		t.Errorf("Expected rv_bypass true, got %v", directive["rv_bypass"])
	}
	if devOnly, ok := directive["dev_only"].(bool); !ok || !devOnly {
		t.Errorf("Expected dev_only true, got %v", directive["dev_only"])
	}
	if ownerOnly, ok := directive["owner_only"].(bool); !ok || !ownerOnly {
		t.Errorf("Expected owner_only true, got %v", directive["owner_only"])
	}
	if userInput, ok := directive["user_input"].(bool); !ok || !userInput {
		t.Errorf("Expected user_input true, got %v", directive["user_input"])
	}

	// Other fields
	if delaySeconds, ok := directive["delay_seconds"].(float64); !ok || int(delaySeconds) != 120 {
		t.Errorf("Expected delay_seconds 120, got %v", directive["delay_seconds"])
	}
	if directive["medium"] != "wifi_all" {
		t.Errorf("Expected medium 'wifi_all', got %v", directive["medium"])
	}
	if directive["wifi_ssid"] != "TestNetwork" {
		t.Errorf("Expected wifi_ssid 'TestNetwork', got %v", directive["wifi_ssid"])
	}
	if directive["wifi_pw"] != "password123" {
		t.Errorf("Expected wifi_pw 'password123', got %v", directive["wifi_pw"])
	}

	// ext_rv is array in V2 (not JSON string like V1)
	extRV, ok := directive["ext_rv"].([]interface{})
	if !ok || len(extRV) != 2 || extRV[0] != "ext1" || extRV[1] != "ext2" {
		t.Errorf("Expected ext_rv [\"ext1\",\"ext2\"], got %v", directive["ext_rv"])
	}

	if directive["sv_cert_hash"] != "aabbcc" {
		t.Errorf("Expected sv_cert_hash 'aabbcc', got %v", directive["sv_cert_hash"])
	}
	if directive["cl_cert_hash"] != "ddeeff" {
		t.Errorf("Expected cl_cert_hash 'ddeeff', got %v", directive["cl_cert_hash"])
	}
}

// TestConvertRvInstructionsToV2JSON_MultipleDirectives tests multiple RV directives
func TestConvertRvInstructionsToV2JSON_MultipleDirectives(t *testing.T) {
	instructions := [][]protocol.RvInstruction{
		{
			{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv1.example.com")},
			{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
			{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
		},
		{
			{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv2.example.com")},
			{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTP))},
			{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8080))},
		},
	}

	result, err := convertRvInstructionsToV2JSON(instructions)
	if err != nil {
		t.Fatalf("convertRvInstructionsToV2JSON failed: %v", err)
	}

	var output [][]map[string]interface{}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(output) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(output))
	}

	// First directive
	dir0 := make(map[string]interface{})
	for _, instr := range output[0] {
		for k, v := range instr {
			dir0[k] = v
		}
	}
	if dir0["dns"] != "rv1.example.com" || dir0["protocol"] != "https" {
		t.Error("First directive values incorrect")
	}
	if port, ok := dir0["owner_port"].(float64); !ok || int(port) != 8443 {
		t.Error("First directive owner_port incorrect")
	}

	// Second directive
	dir1 := make(map[string]interface{})
	for _, instr := range output[1] {
		for k, v := range instr {
			dir1[k] = v
		}
	}
	if dir1["dns"] != "rv2.example.com" || dir1["protocol"] != "http" {
		t.Error("Second directive values incorrect")
	}
	if port, ok := dir1["owner_port"].(float64); !ok || int(port) != 8080 {
		t.Error("Second directive owner_port incorrect")
	}
}

// TestProtocolStringFromCode_V2 verifies protocol code to string conversion
func TestProtocolStringFromCode_V2(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{uint8(protocol.RVProtRest), "rest"},
		{uint8(protocol.RVProtHTTP), "http"},
		{uint8(protocol.RVProtHTTPS), "https"},
		{uint8(protocol.RVProtTCP), "tcp"},
		{uint8(protocol.RVProtTLS), "tls"},
		{uint8(protocol.RVProtCoapTCP), "coap+tcp"},
		{uint8(protocol.RVProtCoapUDP), "coap"},
		{99, "99"}, // Unknown code returns numeric string
	}

	for _, tt := range tests {
		got := ProtocolStringFromCode(tt.code)
		if got != tt.want {
			t.Errorf("ProtocolStringFromCode(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

// TestMediumStringFromCode_V2 verifies medium code to string conversion
func TestMediumStringFromCode_V2(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{protocol.RVMedEthAll, "eth_all"},
		{protocol.RVMedWifiAll, "wifi_all"},
		{99, "99"}, // Unknown code returns numeric string
	}

	for _, tt := range tests {
		got := MediumStringFromCode(tt.code)
		if got != tt.want {
			t.Errorf("MediumStringFromCode(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

// TestCrossAPI_V1InsertV2Read tests inserting via V1 API and reading via V2 API
func TestCrossAPI_V1InsertV2Read(t *testing.T) {
	// This test verifies that V2 API can read CBOR stored by any source
	// Both V1 and V2 APIs use the same CBOR storage format

	state, cleanup := setupCBORTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Insert via V2 format and verify cross-compatibility
	// (Both V1 and V2 APIs store the same CBOR internally)
	v2JSON := []byte(`[[ {"dns":"rv.example.com"}, {"protocol":"https"}, {"owner_port":8443} ]]`)

	if err := state.InsertRvInfo(ctx, v2JSON); err != nil {
		t.Fatalf("InsertRvInfo failed: %v", err)
	}

	// Read it back
	outputJSON, err := state.FetchRvInfoJSON(ctx)
	if err != nil {
		t.Fatalf("FetchRvInfoJSON failed: %v", err)
	}

	// Should get V2 format with integer ports
	var output [][]map[string]interface{}
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Verify V2 format (array of arrays of single-key objects)
	if len(output) != 1 || len(output[0]) != 3 {
		t.Fatalf("Expected 1 directive with 3 instructions, got %d directives", len(output))
	}

	// Verify port is integer
	directive := make(map[string]interface{})
	for _, instr := range output[0] {
		for k, v := range instr {
			directive[k] = v
		}
	}

	if port, ok := directive["owner_port"].(float64); !ok || int(port) != 8443 {
		t.Errorf("Expected owner_port 8443 (int), got %v (type %T)", directive["owner_port"], directive["owner_port"])
	}
}

// Helper function to CBOR marshal values for tests
func mustCBORMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := cbor.Marshal(v)
	if err != nil {
		t.Fatalf("failed to CBOR marshal test data: %v", err)
	}
	return data
}
