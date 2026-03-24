// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package ownerinfo

import (
	"context"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *state.RVTO2AddrState {
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
		t.Errorf("Expected GetRVTO2Addr404TextResponse, got: %T", resp)
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

	err := rvto2addrState.Update(context.Background(), testAddrs)
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
		t.Errorf("Expected GetRVTO2Addr200JSONResponse, got: %T", resp)
	}

	// Verify response has correct data
	if len(resp200) != 1 {
		t.Errorf("Expected 1 entry, got: %d", len(resp200))
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
		t.Errorf("Expected CreateRVTO2Addr201JSONResponse, got: %T", resp)
	}

	// Verify response has correct data
	if len(resp201) != 1 {
		t.Errorf("Expected 1 entry, got: %d", len(resp201))
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

	err := rvto2addrState.Update(context.Background(), testAddrs)
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
		t.Errorf("Expected CreateRVTO2Addr409TextResponse, got: %T", resp)
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
		t.Errorf("Expected CreateRVTO2Addr400TextResponse, got: %T", resp)
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

	err := rvto2addrState.Update(context.Background(), testAddrs)
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
		t.Errorf("Expected UpdateRVTO2Addr200JSONResponse, got: %T", resp)
	}

	// Verify response has updated data
	if len(resp200) != 1 {
		t.Errorf("Expected 1 entry, got: %d", len(resp200))
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
		t.Errorf("Expected UpdateRVTO2Addr404TextResponse, got: %T", resp)
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

	err := rvto2addrState.Update(context.Background(), testAddrs)
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
		t.Errorf("Expected UpdateRVTO2Addr400TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp400, ok := resp.(UpdateRVTO2Addr400TextResponse); ok {
		if string(resp400) != "Invalid ownerInfo" {
			t.Errorf("Expected 'Invalid ownerInfo', got: %s", string(resp400))
		}
	}
}
