// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package ownerinfo

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *state.RVTO2AddrState {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvto2addrState, err := state.InitRVTO2AddrDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RVTO2Addr database: %v", err)
	}

	return rvto2addrState
}

// TestGetRVTO2Addr_NotFound verifies that GET returns 404 when no config exists
func TestGetRVTO2Addr_NotFound(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	resp, err := server.GetRVTO2Addr(context.Background(), GetRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 404 when no config exists
	if _, ok := resp.(GetRVTO2Addr404TextResponse); !ok {
		t.Fatalf("Expected GetRVTO2Addr404TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp404, ok := resp.(GetRVTO2Addr404TextResponse); ok {
		if string(resp404) != "No ownerInfo found" {
			t.Errorf("Expected 'No ownerInfo found', got: %s", string(resp404))
		}
	}
}

// TestGetRVTO2Addr_Success verifies that GET returns 200 with data when config exists
func TestGetRVTO2Addr_Success(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create a test configuration
	dns := "example.com"
	testAddrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8080,
			TransportProtocol: protocol.HTTPTransport,
		},
	}

	err := rvto2addrState.Create(context.Background(), testAddrs)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	resp, err := server.GetRVTO2Addr(context.Background(), GetRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 200 when config exists
	resp200, ok := resp.(GetRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetRVTO2Addr200JSONResponse, got: %T", resp)
	}

	// Verify response has correct data
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 entry, got: %d", len(resp200))
	}
}

// TestCreateRVTO2Addr_Success verifies that POST creates new config
func TestCreateRVTO2Addr_Success(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	dns := "example.com"
	requestBody := RVTO2Addr{
		{
			Dns:      &dns,
			Port:     "8080",
			Protocol: "http",
		},
	}

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 201 on successful create
	resp201, ok := resp.(CreateRVTO2Addr201JSONResponse)
	if !ok {
		t.Fatalf("Expected CreateRVTO2Addr201JSONResponse, got: %T", resp)
	}

	// Verify response has correct data
	if len(resp201) != 1 {
		t.Fatalf("Expected 1 entry, got: %d", len(resp201))
	}
}

// TestCreateRVTO2Addr_Conflict verifies that POST returns 409 when config already exists
func TestCreateRVTO2Addr_Conflict(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create initial configuration
	dns := "example.com"
	testAddrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8080,
			TransportProtocol: protocol.HTTPTransport,
		},
	}

	err := rvto2addrState.Create(context.Background(), testAddrs)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Try to create again - should fail with 409
	requestBody := RVTO2Addr{
		{
			Dns:      &dns,
			Port:     "8080",
			Protocol: "http",
		},
	}

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 409 when config already exists
	if _, ok := resp.(CreateRVTO2Addr409TextResponse); !ok {
		t.Fatalf("Expected CreateRVTO2Addr409TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp409, ok := resp.(CreateRVTO2Addr409TextResponse); ok {
		if string(resp409) != "ownerInfo already exists" {
			t.Errorf("Expected 'ownerInfo already exists', got: %s", string(resp409))
		}
	}
}

// TestCreateRVTO2Addr_InvalidData verifies that POST returns 400 for invalid data
func TestCreateRVTO2Addr_InvalidData(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create request with neither dns nor ip (invalid)
	requestBody := RVTO2Addr{
		{
			Port:     "8080",
			Protocol: "http",
		},
	}

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 400 for invalid data
	if _, ok := resp.(CreateRVTO2Addr400TextResponse); !ok {
		t.Fatalf("Expected CreateRVTO2Addr400TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp400, ok := resp.(CreateRVTO2Addr400TextResponse); ok {
		if string(resp400) != "Invalid ownerInfo" {
			t.Errorf("Expected 'Invalid ownerInfo', got: %s", string(resp400))
		}
	}
}

// TestUpdateRVTO2Addr_Success verifies that PUT updates existing config
func TestUpdateRVTO2Addr_Success(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create initial configuration
	dns := "example.com"
	testAddrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8080,
			TransportProtocol: protocol.HTTPTransport,
		},
	}

	err := rvto2addrState.Create(context.Background(), testAddrs)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Update with new data
	newDns := "new-example.com"
	requestBody := RVTO2Addr{
		{
			Dns:      &newDns,
			Port:     "9090",
			Protocol: "https",
		},
	}

	resp, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 200 on successful update
	resp200, ok := resp.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got: %T", resp)
	}

	// Verify response has updated data
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 entry, got: %d", len(resp200))
	}
	if resp200[0].Dns == nil || *resp200[0].Dns != "new-example.com" {
		t.Errorf("Expected dns 'new-example.com', got: %v", resp200[0].Dns)
	}
}

// TestUpdateRVTO2Addr_NotFound verifies that PUT returns 404 when config doesn't exist
func TestUpdateRVTO2Addr_NotFound(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Try to update without creating first - should fail with 404
	dns := "example.com"
	requestBody := RVTO2Addr{
		{
			Dns:      &dns,
			Port:     "8080",
			Protocol: "http",
		},
	}

	resp, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 404 when config doesn't exist
	if _, ok := resp.(UpdateRVTO2Addr404TextResponse); !ok {
		t.Fatalf("Expected UpdateRVTO2Addr404TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp404, ok := resp.(UpdateRVTO2Addr404TextResponse); ok {
		if string(resp404) != "ownerInfo does not exist" {
			t.Errorf("Expected 'ownerInfo does not exist', got: %s", string(resp404))
		}
	}
}

// TestUpdateRVTO2Addr_InvalidData verifies that PUT returns 400 for invalid data
func TestUpdateRVTO2Addr_InvalidData(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create initial configuration
	dns := "example.com"
	testAddrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8080,
			TransportProtocol: protocol.HTTPTransport,
		},
	}

	err := rvto2addrState.Create(context.Background(), testAddrs)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Try to update with invalid data (neither dns nor ip)
	requestBody := RVTO2Addr{
		{
			Port:     "8080",
			Protocol: "http",
		},
	}

	resp, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 400 for invalid data
	if _, ok := resp.(UpdateRVTO2Addr400TextResponse); !ok {
		t.Fatalf("Expected UpdateRVTO2Addr400TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp400, ok := resp.(UpdateRVTO2Addr400TextResponse); ok {
		if string(resp400) != "Invalid ownerInfo" {
			t.Errorf("Expected 'Invalid ownerInfo', got: %s", string(resp400))
		}
	}
}

// setupTestDBWithLegacyOwnerInfo creates a test database with the legacy owner_info
// table populated with JSON data, simulating the pre-migration state.
func setupTestDBWithLegacyOwnerInfo(t *testing.T, jsonData []byte) *state.RVTO2AddrState {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create the legacy owner_info table manually
	err = db.Exec("CREATE TABLE owner_info (id INTEGER PRIMARY KEY, value BLOB NOT NULL)").Error
	if err != nil {
		t.Fatalf("Failed to create legacy owner_info table: %v", err)
	}

	// Insert legacy JSON data
	err = db.Exec("INSERT INTO owner_info (id, value) VALUES (?, ?)", 1, jsonData).Error
	if err != nil {
		t.Fatalf("Failed to insert legacy owner_info data: %v", err)
	}

	// Initialize the rvto2addr state (creates rvto2addr table but does NOT migrate)
	rvto2addrState, err := state.InitRVTO2AddrDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RVTO2Addr database: %v", err)
	}

	return rvto2addrState
}

// ---------- MigrateOwnerInfo tests ----------

// TestMigrateOwnerInfo_Success verifies that migration from legacy JSON format works
func TestMigrateOwnerInfo_Success(t *testing.T) {
	dns := "owner.example.com"
	ip := "192.168.1.1"
	legacyEntries := []RVTO2AddrEntry{
		{
			Dns:      &dns,
			Ip:       &ip,
			Port:     "8443",
			Protocol: "https",
		},
	}
	legacyJSON, err := json.Marshal(legacyEntries)
	if err != nil {
		t.Fatalf("Failed to marshal legacy JSON: %v", err)
	}

	rvto2addrState := setupTestDBWithLegacyOwnerInfo(t, legacyJSON)
	server := NewServer(rvto2addrState)

	err = server.MigrateOwnerInfo(context.Background())
	if err != nil {
		t.Fatalf("MigrateOwnerInfo failed: %v", err)
	}

	// Verify migrated data can be read back
	addrs, err := rvto2addrState.Get(context.Background())
	if err != nil {
		t.Fatalf("Failed to read migrated data: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("Expected 1 migrated entry, got: %d", len(addrs))
	}
	if addrs[0].DNSAddress == nil || *addrs[0].DNSAddress != dns {
		t.Errorf("Expected DNS %q, got: %v", dns, addrs[0].DNSAddress)
	}
	if addrs[0].IPAddress == nil || addrs[0].IPAddress.String() != ip {
		t.Errorf("Expected IP %q, got: %v", ip, addrs[0].IPAddress)
	}
	if addrs[0].Port != 8443 {
		t.Errorf("Expected port 8443, got: %d", addrs[0].Port)
	}
	if addrs[0].TransportProtocol != protocol.HTTPSTransport {
		t.Errorf("Expected HTTPS transport, got: %v", addrs[0].TransportProtocol)
	}

	// Verify legacy table was dropped
	if rvto2addrState.DB.Migrator().HasTable("owner_info") {
		t.Error("Expected legacy owner_info table to be dropped after migration")
	}
}

// TestMigrateOwnerInfo_MultipleEntries verifies migration of multiple addresses
func TestMigrateOwnerInfo_MultipleEntries(t *testing.T) {
	dns1 := "primary.example.com"
	dns2 := "fallback.example.com"
	ip2 := "10.0.0.1"
	legacyEntries := []RVTO2AddrEntry{
		{Dns: &dns1, Port: "443", Protocol: "https"},
		{Dns: &dns2, Ip: &ip2, Port: "8080", Protocol: "http"},
	}
	legacyJSON, err := json.Marshal(legacyEntries)
	if err != nil {
		t.Fatalf("Failed to marshal legacy JSON: %v", err)
	}

	rvto2addrState := setupTestDBWithLegacyOwnerInfo(t, legacyJSON)
	server := NewServer(rvto2addrState)

	err = server.MigrateOwnerInfo(context.Background())
	if err != nil {
		t.Fatalf("MigrateOwnerInfo failed: %v", err)
	}

	addrs, err := rvto2addrState.Get(context.Background())
	if err != nil {
		t.Fatalf("Failed to read migrated data: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("Expected 2 migrated entries, got: %d", len(addrs))
	}
	if addrs[0].TransportProtocol != protocol.HTTPSTransport {
		t.Errorf("Entry 0: expected HTTPS, got %v", addrs[0].TransportProtocol)
	}
	if addrs[1].TransportProtocol != protocol.HTTPTransport {
		t.Errorf("Entry 1: expected HTTP, got %v", addrs[1].TransportProtocol)
	}
}

// TestMigrateOwnerInfo_NoLegacyTable verifies that migration is a no-op when the legacy table doesn't exist
func TestMigrateOwnerInfo_NoLegacyTable(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	err := server.MigrateOwnerInfo(context.Background())
	if err != nil {
		t.Fatalf("Expected no error when no legacy table exists, got: %v", err)
	}
}

// TestMigrateOwnerInfo_RVTO2AddrAlreadyHasData verifies migration is skipped when rvto2addr already has data
func TestMigrateOwnerInfo_RVTO2AddrAlreadyHasData(t *testing.T) {
	dns := "legacy.example.com"
	legacyEntries := []RVTO2AddrEntry{
		{Dns: &dns, Port: "8080", Protocol: "http"},
	}
	legacyJSON, err := json.Marshal(legacyEntries)
	if err != nil {
		t.Fatalf("Failed to marshal legacy JSON: %v", err)
	}

	rvto2addrState := setupTestDBWithLegacyOwnerInfo(t, legacyJSON)
	server := NewServer(rvto2addrState)

	// Pre-populate rvto2addr so migration should be skipped
	existingDNS := "existing.example.com"
	err = rvto2addrState.Create(context.Background(), []protocol.RvTO2Addr{
		{DNSAddress: &existingDNS, Port: 9090, TransportProtocol: protocol.HTTPSTransport},
	})
	if err != nil {
		t.Fatalf("Failed to insert existing data: %v", err)
	}

	err = server.MigrateOwnerInfo(context.Background())
	if err != nil {
		t.Fatalf("MigrateOwnerInfo failed: %v", err)
	}

	// Verify the existing data was NOT overwritten
	addrs, err := rvto2addrState.Get(context.Background())
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("Expected 1 entry, got: %d", len(addrs))
	}
	if addrs[0].DNSAddress == nil || *addrs[0].DNSAddress != existingDNS {
		t.Errorf("Expected DNS %q (existing), got: %v", existingDNS, addrs[0].DNSAddress)
	}
}

// TestMigrateOwnerInfo_InvalidJSON verifies that migration fails gracefully on corrupt JSON
func TestMigrateOwnerInfo_InvalidJSON(t *testing.T) {
	rvto2addrState := setupTestDBWithLegacyOwnerInfo(t, []byte(`not valid json`))
	server := NewServer(rvto2addrState)

	err := server.MigrateOwnerInfo(context.Background())
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

// TestMigrateOwnerInfo_InvalidEntry verifies that migration fails when an entry has no dns/ip
func TestMigrateOwnerInfo_InvalidEntry(t *testing.T) {
	// Entry with neither dns nor ip
	legacyJSON := []byte(`[{"port":"8080","protocol":"http"}]`)
	rvto2addrState := setupTestDBWithLegacyOwnerInfo(t, legacyJSON)
	server := NewServer(rvto2addrState)

	err := server.MigrateOwnerInfo(context.Background())
	if err == nil {
		t.Fatal("Expected error for entry with no dns/ip, got nil")
	}
}

// ---------- Nil body tests ----------

// TestCreateRVTO2Addr_NilBody verifies that POST returns 400 when body is nil
func TestCreateRVTO2Addr_NilBody(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: nil,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if _, ok := resp.(CreateRVTO2Addr400TextResponse); !ok {
		t.Fatalf("Expected CreateRVTO2Addr400TextResponse, got: %T", resp)
	}
}

// TestUpdateRVTO2Addr_NilBody verifies that PUT returns 400 when body is nil
func TestUpdateRVTO2Addr_NilBody(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	resp, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: nil,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if _, ok := resp.(UpdateRVTO2Addr400TextResponse); !ok {
		t.Fatalf("Expected UpdateRVTO2Addr400TextResponse, got: %T", resp)
	}
}

// ---------- Multiple addresses in create/update ----------

// TestCreateRVTO2Addr_MultipleAddresses verifies that POST creates multiple address entries
func TestCreateRVTO2Addr_MultipleAddresses(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	dns1 := "primary.example.com"
	ip2 := "10.0.0.1"
	requestBody := RVTO2Addr{
		{Dns: &dns1, Port: "443", Protocol: "https"},
		{Ip: &ip2, Port: "8080", Protocol: "http"},
	}

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	resp201, ok := resp.(CreateRVTO2Addr201JSONResponse)
	if !ok {
		t.Fatalf("Expected CreateRVTO2Addr201JSONResponse, got: %T", resp)
	}
	if len(resp201) != 2 {
		t.Fatalf("Expected 2 entries, got: %d", len(resp201))
	}
	if resp201[0].Dns == nil || *resp201[0].Dns != dns1 {
		t.Errorf("Entry 0: expected DNS %q, got %v", dns1, resp201[0].Dns)
	}
	if resp201[1].Ip == nil || *resp201[1].Ip != ip2 {
		t.Errorf("Entry 1: expected IP %q, got %v", ip2, resp201[1].Ip)
	}
}

// TestUpdateRVTO2Addr_MultipleAddresses verifies that PUT updates to multiple addresses
func TestUpdateRVTO2Addr_MultipleAddresses(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Create initial single entry
	dns := "old.example.com"
	err := rvto2addrState.Create(context.Background(), []protocol.RvTO2Addr{
		{DNSAddress: &dns, Port: 80, TransportProtocol: protocol.HTTPTransport},
	})
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Update with multiple entries
	dns1 := "new1.example.com"
	dns2 := "new2.example.com"
	requestBody := RVTO2Addr{
		{Dns: &dns1, Port: "443", Protocol: "https"},
		{Dns: &dns2, Port: "8443", Protocol: "tls"},
	}

	resp, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	resp200, ok := resp.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got: %T", resp)
	}
	if len(resp200) != 2 {
		t.Fatalf("Expected 2 entries, got: %d", len(resp200))
	}
}

// ---------- Helper function edge case tests ----------

// TestApiToProtocolAddr_InvalidIP verifies that invalid IP addresses are rejected
func TestApiToProtocolAddr_InvalidIP(t *testing.T) {
	badIP := "999.999.999.999"
	entry := RVTO2AddrEntry{
		Ip:       &badIP,
		Port:     "8080",
		Protocol: "http",
	}
	_, err := apiToProtocolAddr(entry)
	if err == nil {
		t.Fatal("Expected error for invalid IP, got nil")
	}
}

// TestApiToProtocolAddr_InvalidPort verifies that invalid ports are rejected
func TestApiToProtocolAddr_InvalidPort(t *testing.T) {
	dns := "example.com"
	tests := []struct {
		name string
		port string
	}{
		{"non-numeric", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"too large", "70000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := RVTO2AddrEntry{
				Dns:      &dns,
				Port:     tc.port,
				Protocol: "http",
			}
			_, err := apiToProtocolAddr(entry)
			if err == nil {
				t.Errorf("Expected error for port %q, got nil", tc.port)
			}
		})
	}
}

// TestApiToTransportProtocol_Unsupported verifies that unknown protocol types are rejected
func TestApiToTransportProtocol_Unsupported(t *testing.T) {
	unsupported := []TransportProtocol{
		TransportProtocol("coap+tcp"), // CoAP over TCP is not defined in the FDO spec
		TransportProtocol("unknown"),
	}
	for _, pt := range unsupported {
		t.Run(string(pt), func(t *testing.T) {
			_, err := apiToTransportProtocol(pt)
			if err == nil {
				t.Fatalf("Expected error for unsupported protocol type %q, got nil", pt)
			}
		})
	}
}

// TestApiToTransportProtocol_AllValid verifies all known protocol types map correctly
func TestApiToTransportProtocol_AllValid(t *testing.T) {
	tests := []struct {
		input    TransportProtocol
		expected protocol.TransportProtocol
	}{
		{Tcp, protocol.TCPTransport},
		{Tls, protocol.TLSTransport},
		{Http, protocol.HTTPTransport},
		{Https, protocol.HTTPSTransport},
		{Coap, protocol.CoAPTransport},
		{Rest, protocol.HTTPSTransport},
	}

	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			got, err := apiToTransportProtocol(tc.input)
			if err != nil {
				t.Fatalf("Unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, got)
			}
		})
	}
}

// TestTransportToAPIProtocol_AllValues verifies all protocol-to-API mappings including edge cases
func TestTransportToAPIProtocol_AllValues(t *testing.T) {
	tests := []struct {
		name     string
		input    protocol.TransportProtocol
		expected TransportProtocol
	}{
		{"TCP", protocol.TCPTransport, Tcp},
		{"TLS", protocol.TLSTransport, Tls},
		{"HTTP", protocol.HTTPTransport, Http},
		{"HTTPS", protocol.HTTPSTransport, Https},
		{"CoAP", protocol.CoAPTransport, Coap},
		{"CoAPS maps to CoAP", protocol.CoAPSTransport, Coap},
		{"unknown defaults to HTTPS", protocol.TransportProtocol(255), Https},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := transportToAPIProtocol(tc.input)
			if got != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// TestProtocolToAPIAddr_WithIPOnly verifies round-trip for IP-only address
func TestProtocolToAPIAddr_WithIPOnly(t *testing.T) {
	ip := net.ParseIP("192.168.1.100")
	addr := protocol.RvTO2Addr{
		IPAddress:         &ip,
		Port:              443,
		TransportProtocol: protocol.HTTPSTransport,
	}
	apiAddr := protocolToAPIAddr(addr)

	if apiAddr.Ip == nil || *apiAddr.Ip != "192.168.1.100" {
		t.Errorf("Expected IP '192.168.1.100', got: %v", apiAddr.Ip)
	}
	if apiAddr.Dns != nil {
		t.Errorf("Expected nil DNS, got: %v", apiAddr.Dns)
	}
	if apiAddr.Port != "443" {
		t.Errorf("Expected port '443', got: %s", apiAddr.Port)
	}
	if apiAddr.Protocol != Https {
		t.Errorf("Expected protocol 'https', got: %s", apiAddr.Protocol)
	}
}

// TestCreateRVTO2Addr_InvalidProtocol verifies that POST returns 400 for unsupported protocol
func TestCreateRVTO2Addr_InvalidProtocol(t *testing.T) {
	rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	dns := "example.com"
	requestBody := RVTO2Addr{
		{Dns: &dns, Port: "8080", Protocol: "ftp"},
	}

	resp, err := server.CreateRVTO2Addr(context.Background(), CreateRVTO2AddrRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if _, ok := resp.(CreateRVTO2Addr400TextResponse); !ok {
		t.Fatalf("Expected CreateRVTO2Addr400TextResponse, got: %T", resp)
	}
}

// TestApiToProtocolAddr_BothDNSAndIP verifies entries with both dns and ip
func TestApiToProtocolAddr_BothDNSAndIP(t *testing.T) {
	dns := "example.com"
	ip := "10.0.0.1"
	entry := RVTO2AddrEntry{
		Dns:      &dns,
		Ip:       &ip,
		Port:     "443",
		Protocol: "https",
	}
	addr, err := apiToProtocolAddr(entry)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if addr.DNSAddress == nil || *addr.DNSAddress != dns {
		t.Errorf("Expected DNS %q, got %v", dns, addr.DNSAddress)
	}
	if addr.IPAddress == nil || addr.IPAddress.String() != ip {
		t.Errorf("Expected IP %q, got %v", ip, addr.IPAddress)
	}
}

// TestApiToProtocolAddr_EmptyDNSAndEmptyIP verifies that empty strings for both fields are rejected
func TestApiToProtocolAddr_EmptyDNSAndEmptyIP(t *testing.T) {
	emptyDNS := ""
	emptyIP := ""
	entry := RVTO2AddrEntry{
		Dns:      &emptyDNS,
		Ip:       &emptyIP,
		Port:     "8080",
		Protocol: "http",
	}
	_, err := apiToProtocolAddr(entry)
	if err == nil {
		t.Fatal("Expected error for empty dns and ip, got nil")
	}
}
