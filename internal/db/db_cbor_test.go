// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/json"
	"testing"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// TestInsertRvInfo_StoresCBOR verifies that RvInfo is stored as CBOR, not JSON
func TestInsertRvInfo_StoresCBOR(t *testing.T) {
	state, err := InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		_ = state.Close()
	})

	// Valid V1 JSON input
	v1JSON := []byte(`[{"dns":"rv.example.com","protocol":"http","owner_port":"8080"}]`)

	// Insert via V1 API
	if err := InsertRvInfo(v1JSON); err != nil {
		t.Fatalf("InsertRvInfo failed: %v", err)
	}

	// Read raw bytes from database
	var rvInfoRow RvInfo
	if err := db.Where("id = ?", 1).First(&rvInfoRow).Error; err != nil {
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
		t.Errorf("Expected 3 instructions (dns, protocol, owner_port), got %d", len(rvInfo[0]))
	}
}

// TestFetchRvInfo_AutoMigration tests automatic migration from V1 JSON to CBOR
func TestFetchRvInfo_AutoMigration(t *testing.T) {
	state, err := InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		_ = state.Close()
	})

	// Simulate old database: Insert V1 JSON directly (bypassing CBOR encoding)
	v1JSON := `[{"dns":"rv.example.com","protocol":"https","owner_port":"8443","device_port":"8041"}]`
	oldRvInfo := RvInfo{
		ID:    1,
		Value: []byte(v1JSON),
	}
	if err := db.Create(&oldRvInfo).Error; err != nil {
		t.Fatalf("failed to create old JSON record: %v", err)
	}

	// Verify it's JSON before migration
	var jsonTest interface{}
	if err := json.Unmarshal(oldRvInfo.Value, &jsonTest); err != nil {
		t.Fatalf("Setup failed: inserted data should be JSON: %v", err)
	}

	// Call FetchRvInfo() - should trigger auto-migration
	result, err := FetchRvInfo()
	if err != nil {
		t.Fatalf("FetchRvInfo failed: %v", err)
	}

	// Verify returned data is correct
	if len(result) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(result))
	}
	if len(result[0]) != 4 {
		t.Errorf("Expected 4 instructions, got %d", len(result[0]))
	}

	// Verify database now contains CBOR (not JSON)
	var migratedRow RvInfo
	if err := db.Where("id = ?", 1).First(&migratedRow).Error; err != nil {
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

// TestFetchRvInfoJSON_AutoMigration tests auto-migration and V1 JSON output format
func TestFetchRvInfoJSON_AutoMigration(t *testing.T) {
	// Define V1 format structure for parsing
	type rvHuman struct {
		DNS          string  `json:"dns,omitempty"`
		IP           string  `json:"ip,omitempty"`
		Protocol     string  `json:"protocol,omitempty"`
		Medium       string  `json:"medium,omitempty"`
		DevicePort   string  `json:"device_port,omitempty"`
		OwnerPort    string  `json:"owner_port,omitempty"`
		WifiSSID     string  `json:"wifi_ssid,omitempty"`
		WifiPW       string  `json:"wifi_pw,omitempty"`
		DevOnly      bool    `json:"dev_only,omitempty"`
		OwnerOnly    bool    `json:"owner_only,omitempty"`
		RvBypass     bool    `json:"rv_bypass,omitempty"`
		DelaySeconds *uint32 `json:"delay_seconds,omitempty"`
		SvCertHash   string  `json:"sv_cert_hash,omitempty"`
		ClCertHash   string  `json:"cl_cert_hash,omitempty"`
		UserInput    string  `json:"user_input,omitempty"`
		ExtRV        string  `json:"ext_rv,omitempty"`
	}

	state, err := InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		_ = state.Close()
	})

	// Simulate old database: Insert V1 JSON directly
	v1JSON := `[{"dns":"rv.example.com","protocol":"http","owner_port":"8080","rv_bypass":true}]`
	oldRvInfo := RvInfo{
		ID:    1,
		Value: []byte(v1JSON),
	}
	if err := db.Create(&oldRvInfo).Error; err != nil {
		t.Fatalf("failed to create old JSON record: %v", err)
	}

	// Call FetchRvInfo() - should trigger migration
	rvInfo, err := FetchRvInfo()
	if err != nil {
		t.Fatalf("FetchRvInfo failed: %v", err)
	}

	// Convert to V1 JSON
	outputJSON, err := ConvertRvInstructionsToV1JSON(rvInfo)
	if err != nil {
		t.Fatalf("ConvertRvInstructionsToV1JSON failed: %v", err)
	}

	// Parse output to verify V1 format
	var output []rvHuman
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	// Verify V1 format specifics
	if len(output) != 1 {
		t.Errorf("Expected 1 item, got %d", len(output))
	}
	if output[0].DNS != "rv.example.com" {
		t.Errorf("Expected DNS 'rv.example.com', got '%s'", output[0].DNS)
	}
	if output[0].Protocol != "http" {
		t.Errorf("Expected protocol 'http', got '%s'", output[0].Protocol)
	}
	// V1 uses STRING ports
	if output[0].OwnerPort != "8080" {
		t.Errorf("Expected owner_port '8080' (string), got '%s'", output[0].OwnerPort)
	}
	if !output[0].RvBypass {
		t.Error("Expected rv_bypass to be true")
	}

	// Verify database was migrated to CBOR
	var migratedRow RvInfo
	if err := db.Where("id = ?", 1).First(&migratedRow).Error; err != nil {
		t.Fatalf("failed to read migrated row: %v", err)
	}
	var cborTest [][]protocol.RvInstruction
	if err := cbor.Unmarshal(migratedRow.Value, &cborTest); err != nil {
		t.Errorf("After migration, data should be CBOR: %v", err)
	}
}

// TestV1_RoundTrip verifies V1 JSON → CBOR → V1 JSON preserves data
func TestV1_RoundTrip(t *testing.T) {
	// Define V1 format structure for parsing
	type rvHuman struct {
		DNS          string  `json:"dns,omitempty"`
		IP           string  `json:"ip,omitempty"`
		Protocol     string  `json:"protocol,omitempty"`
		Medium       string  `json:"medium,omitempty"`
		DevicePort   string  `json:"device_port,omitempty"`
		OwnerPort    string  `json:"owner_port,omitempty"`
		WifiSSID     string  `json:"wifi_ssid,omitempty"`
		WifiPW       string  `json:"wifi_pw,omitempty"`
		DevOnly      bool    `json:"dev_only,omitempty"`
		OwnerOnly    bool    `json:"owner_only,omitempty"`
		RvBypass     bool    `json:"rv_bypass,omitempty"`
		DelaySeconds *uint32 `json:"delay_seconds,omitempty"`
		SvCertHash   string  `json:"sv_cert_hash,omitempty"`
		ClCertHash   string  `json:"cl_cert_hash,omitempty"`
		UserInput    string  `json:"user_input,omitempty"`
		ExtRV        string  `json:"ext_rv,omitempty"`
	}

	state, err := InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		_ = state.Close()
	})

	// Input V1 JSON with all field types
	inputJSON := []byte(`[
		{"dns":"rv1.example.com","protocol":"https","owner_port":"8443","device_port":"8041","rv_bypass":true},
		{"ip":"192.168.1.100","protocol":"http","owner_port":"8080","delay_seconds":30}
	]`)

	// Insert via V1 API
	if err := InsertRvInfo(inputJSON); err != nil {
		t.Fatalf("InsertRvInfo failed: %v", err)
	}

	// Retrieve via V1 API
	rvInfo, err := FetchRvInfo()
	if err != nil {
		t.Fatalf("FetchRvInfo failed: %v", err)
	}

	outputJSON, err := ConvertRvInstructionsToV1JSON(rvInfo)
	if err != nil {
		t.Fatalf("ConvertRvInstructionsToV1JSON failed: %v", err)
	}

	// Parse input and output
	var inputData, outputData []rvHuman
	if err := json.Unmarshal(inputJSON, &inputData); err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}
	if err := json.Unmarshal(outputJSON, &outputData); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Verify all values match
	if len(inputData) != len(outputData) {
		t.Fatalf("Expected %d items, got %d", len(inputData), len(outputData))
	}

	for i := range inputData {
		in := inputData[i]
		out := outputData[i]

		if in.DNS != out.DNS {
			t.Errorf("[%d] DNS mismatch: %s != %s", i, in.DNS, out.DNS)
		}
		if in.IP != out.IP {
			t.Errorf("[%d] IP mismatch: %s != %s", i, in.IP, out.IP)
		}
		if in.Protocol != out.Protocol {
			t.Errorf("[%d] Protocol mismatch: %s != %s", i, in.Protocol, out.Protocol)
		}
		if in.OwnerPort != out.OwnerPort {
			t.Errorf("[%d] OwnerPort mismatch: %s != %s", i, in.OwnerPort, out.OwnerPort)
		}
		if in.DevicePort != out.DevicePort {
			t.Errorf("[%d] DevicePort mismatch: %s != %s", i, in.DevicePort, out.DevicePort)
		}
		if in.RvBypass != out.RvBypass {
			t.Errorf("[%d] RvBypass mismatch: %v != %v", i, in.RvBypass, out.RvBypass)
		}
		if (in.DelaySeconds == nil) != (out.DelaySeconds == nil) {
			t.Errorf("[%d] DelaySeconds nil mismatch", i)
		} else if in.DelaySeconds != nil && *in.DelaySeconds != *out.DelaySeconds {
			t.Errorf("[%d] DelaySeconds mismatch: %d != %d", i, *in.DelaySeconds, *out.DelaySeconds)
		}
	}
}

// TestConvertRvInstructionsToV1JSON_AllFields tests conversion with all RV instruction types
func TestConvertRvInstructionsToV1JSON_AllFields(t *testing.T) {
	// Define V1 format structure for parsing
	type rvHuman struct {
		DNS          string  `json:"dns,omitempty"`
		IP           string  `json:"ip,omitempty"`
		Protocol     string  `json:"protocol,omitempty"`
		Medium       string  `json:"medium,omitempty"`
		DevicePort   string  `json:"device_port,omitempty"`
		OwnerPort    string  `json:"owner_port,omitempty"`
		WifiSSID     string  `json:"wifi_ssid,omitempty"`
		WifiPW       string  `json:"wifi_pw,omitempty"`
		DevOnly      bool    `json:"dev_only,omitempty"`
		OwnerOnly    bool    `json:"owner_only,omitempty"`
		RvBypass     bool    `json:"rv_bypass,omitempty"`
		DelaySeconds *uint32 `json:"delay_seconds,omitempty"`
		SvCertHash   string  `json:"sv_cert_hash,omitempty"`
		ClCertHash   string  `json:"cl_cert_hash,omitempty"`
		UserInput    string  `json:"user_input,omitempty"`
		ExtRV        string  `json:"ext_rv,omitempty"`
	}

	// Create instructions with all field types
	delay := uint32(120)
	instructions := [][]protocol.RvInstruction{{
		{Variable: protocol.RVDns, Value: mustCBORMarshal(t, "rv.example.com")},
		{Variable: protocol.RVIPAddress, Value: mustCBORMarshal(t, []byte{192, 168, 1, 100})},
		{Variable: protocol.RVProtocol, Value: mustCBORMarshal(t, uint8(protocol.RVProtHTTPS))},
		{Variable: protocol.RVDevPort, Value: mustCBORMarshal(t, uint16(8041))},
		{Variable: protocol.RVOwnerPort, Value: mustCBORMarshal(t, uint16(8443))},
		{Variable: protocol.RVBypass, Value: nil}, // Flag only
		{Variable: protocol.RVDelaysec, Value: mustCBORMarshal(t, delay)},
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

	// Convert to V1 JSON
	result, err := ConvertRvInstructionsToV1JSON(instructions)
	if err != nil {
		t.Fatalf("ConvertRvInstructionsToV1JSON failed: %v", err)
	}

	// Parse result
	var output []rvHuman
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(output) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output))
	}

	item := output[0]

	// Verify V1 format specifics
	if item.DNS != "rv.example.com" {
		t.Errorf("Expected DNS 'rv.example.com', got '%s'", item.DNS)
	}
	if item.IP != "192.168.1.100" {
		t.Errorf("Expected IP '192.168.1.100', got '%s'", item.IP)
	}
	if item.Protocol != "https" {
		t.Errorf("Expected protocol 'https', got '%s'", item.Protocol)
	}
	// V1 uses STRING ports
	if item.DevicePort != "8041" {
		t.Errorf("Expected device_port '8041' (string), got '%s'", item.DevicePort)
	}
	if item.OwnerPort != "8443" {
		t.Errorf("Expected owner_port '8443' (string), got '%s'", item.OwnerPort)
	}
	if !item.RvBypass {
		t.Error("Expected rv_bypass to be true")
	}
	if item.DelaySeconds == nil || *item.DelaySeconds != 120 {
		t.Errorf("Expected delay_seconds 120, got %v", item.DelaySeconds)
	}
	if item.Medium != "wifi_all" {
		t.Errorf("Expected medium 'wifi_all', got '%s'", item.Medium)
	}
	if item.WifiSSID != "TestNetwork" {
		t.Errorf("Expected wifi_ssid 'TestNetwork', got '%s'", item.WifiSSID)
	}
	if item.WifiPW != "password123" {
		t.Errorf("Expected wifi_pw 'password123', got '%s'", item.WifiPW)
	}
	if !item.DevOnly {
		t.Error("Expected dev_only to be true")
	}
	if !item.OwnerOnly {
		t.Error("Expected owner_only to be true")
	}
	if item.UserInput != "true" {
		t.Errorf("Expected user_input 'true', got '%s'", item.UserInput)
	}
	// ext_rv is stored as JSON array string in V1
	if item.ExtRV != `["ext1","ext2"]` {
		t.Errorf("Expected ext_rv '[\"ext1\",\"ext2\"]', got '%s'", item.ExtRV)
	}
	if item.SvCertHash != "aabbcc" {
		t.Errorf("Expected sv_cert_hash 'aabbcc', got '%s'", item.SvCertHash)
	}
	if item.ClCertHash != "ddeeff" {
		t.Errorf("Expected cl_cert_hash 'ddeeff', got '%s'", item.ClCertHash)
	}
}

// TestConvertRvInstructionsToV1JSON_MultipleDirectives tests multiple RV directives
func TestConvertRvInstructionsToV1JSON_MultipleDirectives(t *testing.T) {
	// Define V1 format structure for parsing
	type rvHuman struct {
		DNS          string  `json:"dns,omitempty"`
		IP           string  `json:"ip,omitempty"`
		Protocol     string  `json:"protocol,omitempty"`
		Medium       string  `json:"medium,omitempty"`
		DevicePort   string  `json:"device_port,omitempty"`
		OwnerPort    string  `json:"owner_port,omitempty"`
		WifiSSID     string  `json:"wifi_ssid,omitempty"`
		WifiPW       string  `json:"wifi_pw,omitempty"`
		DevOnly      bool    `json:"dev_only,omitempty"`
		OwnerOnly    bool    `json:"owner_only,omitempty"`
		RvBypass     bool    `json:"rv_bypass,omitempty"`
		DelaySeconds *uint32 `json:"delay_seconds,omitempty"`
		SvCertHash   string  `json:"sv_cert_hash,omitempty"`
		ClCertHash   string  `json:"cl_cert_hash,omitempty"`
		UserInput    string  `json:"user_input,omitempty"`
		ExtRV        string  `json:"ext_rv,omitempty"`
	}

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

	result, err := ConvertRvInstructionsToV1JSON(instructions)
	if err != nil {
		t.Fatalf("ConvertRvInstructionsToV1JSON failed: %v", err)
	}

	var output []rvHuman
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(output) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(output))
	}

	if output[0].DNS != "rv1.example.com" || output[0].Protocol != "https" || output[0].OwnerPort != "8443" {
		t.Error("First directive values incorrect")
	}
	if output[1].DNS != "rv2.example.com" || output[1].Protocol != "http" || output[1].OwnerPort != "8080" {
		t.Error("Second directive values incorrect")
	}
}

// TestProtocolStringFromCode verifies protocol code to string conversion
func TestProtocolStringFromCode(t *testing.T) {
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
		got := utils.ProtocolStringFromCode(tt.code)
		if got != tt.want {
			t.Errorf("utils.ProtocolStringFromCode(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

// TestMediumStringFromCode verifies medium code to string conversion
func TestMediumStringFromCode(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{protocol.RVMedEthAll, "eth_all"},
		{protocol.RVMedWifiAll, "wifi_all"},
		{99, "99"}, // Unknown code returns numeric string
	}

	for _, tt := range tests {
		got := utils.MediumStringFromCode(tt.code)
		if got != tt.want {
			t.Errorf("utils.MediumStringFromCode(%d) = %s, want %s", tt.code, got, tt.want)
		}
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
