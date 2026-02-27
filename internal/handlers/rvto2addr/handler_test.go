// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvto2addr

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, *state.RVTO2AddrState) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvto2addrState, err := state.InitRVTO2AddrDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RVTO2Addr state: %v", err)
	}

	return db, rvto2addrState
}

func TestGetRVTO2Addr_EmptyConfiguration(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	response, err := server.GetRVTO2Addr(context.Background(), GetRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("GetRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(GetRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetRVTO2Addr200JSONResponse, got %T", response)
	}

	if len(resp) != 0 {
		t.Errorf("Expected empty configuration, got %d entries", len(resp))
	}
}

func TestUpdateRVTO2Addr_ValidConfiguration(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test single address with DNS
	dnsAddr := "owner.example.com"
	port := 8080
	protocol := components.Https
	config := []RVTO2AddrEntry{
		{
			Dns:      &dnsAddr,
			Port:     port,
			Protocol: protocol,
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got %T", response)
	}

	if len(resp) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(resp))
	}
	if resp[0].Dns == nil || *resp[0].Dns != dnsAddr {
		t.Errorf("Expected DNS %s, got %v", dnsAddr, resp[0].Dns)
	}
	if resp[0].Port != port {
		t.Errorf("Expected port %d, got %d", port, resp[0].Port)
	}
}

func TestUpdateRVTO2Addr_WithIPAddress(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test single address with IP
	ipAddr := "192.168.1.100"
	port := 8443
	protocol := components.Https
	config := []RVTO2AddrEntry{
		{
			Ip:       &ipAddr,
			Port:     port,
			Protocol: protocol,
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got %T", response)
	}

	if len(resp) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(resp))
	}
	if resp[0].Ip == nil || *resp[0].Ip != ipAddr {
		t.Errorf("Expected IP %s, got %v", ipAddr, resp[0].Ip)
	}
}

func TestUpdateRVTO2Addr_WithBothDNSAndIP(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test address with both DNS and IP
	dnsAddr := "owner.example.com"
	ipAddr := "192.168.1.100"
	port := 8443
	protocol := components.Https
	config := []RVTO2AddrEntry{
		{
			Dns:      &dnsAddr,
			Ip:       &ipAddr,
			Port:     port,
			Protocol: protocol,
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got %T", response)
	}

	if len(resp) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(resp))
	}
	if resp[0].Dns == nil || *resp[0].Dns != dnsAddr {
		t.Errorf("Expected DNS %s, got %v", dnsAddr, resp[0].Dns)
	}
	if resp[0].Ip == nil || *resp[0].Ip != ipAddr {
		t.Errorf("Expected IP %s, got %v", ipAddr, resp[0].Ip)
	}
}

func TestUpdateRVTO2Addr_MultipleAddresses(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test multiple addresses (primary and backup)
	dns1 := "owner-primary.example.com"
	dns2 := "owner-backup.example.com"
	ip3 := "192.168.1.100"
	config := []RVTO2AddrEntry{
		{
			Dns:      &dns1,
			Port:     8443,
			Protocol: "https",
		},
		{
			Dns:      &dns2,
			Port:     8443,
			Protocol: "https",
		},
		{
			Ip:       &ip3,
			Port:     8443,
			Protocol: "https",
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got %T", response)
	}

	if len(resp) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(resp))
	}
}

func TestUpdateRVTO2Addr_InvalidMissingBothDNSAndIP(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test invalid configuration (missing both DNS and IP)
	port := 8443
	protocol := components.Https
	config := []RVTO2AddrEntry{
		{
			Port:     port,
			Protocol: protocol,
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	_, ok := response.(UpdateRVTO2Addr400JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr400JSONResponse for invalid config, got %T", response)
	}
}

func TestUpdateRVTO2Addr_InvalidIPAddress(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Test invalid IP address
	invalidIP := "not-an-ip-address"
	config := []RVTO2AddrEntry{
		{
			Ip:       &invalidIP,
			Port:     8443,
			Protocol: "https",
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	_, ok := response.(UpdateRVTO2Addr400JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr400JSONResponse for invalid IP, got %T", response)
	}
}

func TestDeleteRVTO2Addr_WithConfiguration(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// First, set a configuration
	dnsAddr := "owner.example.com"
	config := []RVTO2AddrEntry{
		{
			Dns:      &dnsAddr,
			Port:     8080,
			Protocol: "https",
		},
	}

	_, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config,
	})
	if err != nil {
		t.Fatalf("UpdateRVTO2Addr failed: %v", err)
	}

	// Now delete it
	response, err := server.DeleteRVTO2Addr(context.Background(), DeleteRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("DeleteRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(DeleteRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected DeleteRVTO2Addr200JSONResponse, got %T", response)
	}

	// Should return the deleted configuration
	if len(resp) != 1 {
		t.Errorf("Expected 1 entry in deleted response, got %d", len(resp))
	}
	if resp[0].Dns == nil || *resp[0].Dns != dnsAddr {
		t.Errorf("Expected DNS %s in deleted response, got %v", dnsAddr, resp[0].Dns)
	}

	// Verify it's actually deleted
	getResponse, err := server.GetRVTO2Addr(context.Background(), GetRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("GetRVTO2Addr failed: %v", err)
	}

	getResp, ok := getResponse.(GetRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetRVTO2Addr200JSONResponse, got %T", getResponse)
	}

	if len(getResp) != 0 {
		t.Errorf("Expected empty configuration after delete, got %d entries", len(getResp))
	}
}

func TestDeleteRVTO2Addr_EmptyConfiguration(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Delete when nothing is configured
	response, err := server.DeleteRVTO2Addr(context.Background(), DeleteRVTO2AddrRequestObject{})
	if err != nil {
		t.Fatalf("DeleteRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(DeleteRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected DeleteRVTO2Addr200JSONResponse, got %T", response)
	}

	// Should return empty array
	if len(resp) != 0 {
		t.Errorf("Expected empty response when deleting empty config, got %d entries", len(resp))
	}
}

func TestUpdateRVTO2Addr_OverwritesExisting(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)

	// Set initial configuration
	dns1 := "owner-old.example.com"
	config1 := []RVTO2AddrEntry{
		{
			Dns:      &dns1,
			Port:     8080,
			Protocol: "http",
		},
	}

	_, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config1,
	})
	if err != nil {
		t.Fatalf("First UpdateRVTO2Addr failed: %v", err)
	}

	// Update with new configuration
	dns2 := "owner-new.example.com"
	config2 := []RVTO2AddrEntry{
		{
			Dns:      &dns2,
			Port:     8443,
			Protocol: "https",
		},
	}

	response, err := server.UpdateRVTO2Addr(context.Background(), UpdateRVTO2AddrRequestObject{
		Body: &config2,
	})
	if err != nil {
		t.Fatalf("Second UpdateRVTO2Addr failed: %v", err)
	}

	resp, ok := response.(UpdateRVTO2Addr200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRVTO2Addr200JSONResponse, got %T", response)
	}

	// Should have new configuration, not old
	if len(resp) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(resp))
	}
	if resp[0].Dns == nil || *resp[0].Dns != dns2 {
		t.Errorf("Expected new DNS %s, got %v", dns2, resp[0].Dns)
	}
	if resp[0].Port != 8443 {
		t.Errorf("Expected new port 8443, got %d", resp[0].Port)
	}
}

func TestRVTO2Addr_HTTPIntegration(t *testing.T) {
	_, rvto2addrState := setupTestDB(t)
	server := NewServer(rvto2addrState)
	strictHandler := NewStrictHandler(&server, nil)

	mux := http.NewServeMux()
	HandlerFromMux(strictHandler, mux)

	// Test GET empty
	req := httptest.NewRequest("GET", "/v1/rvto2addr", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var getResp []RVTO2AddrEntry
	if err := json.NewDecoder(w.Body).Decode(&getResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(getResp) != 0 {
		t.Errorf("Expected empty array, got %d entries", len(getResp))
	}

	// Test PUT
	dns := "owner.example.com"
	updateBody := []RVTO2AddrEntry{
		{
			Dns:      &dns,
			Port:     8443,
			Protocol: "https",
		},
	}
	bodyBytes, _ := json.Marshal(updateBody)

	req = httptest.NewRequest("PUT", "/v1/rvto2addr", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Test DELETE
	req = httptest.NewRequest("DELETE", "/v1/rvto2addr", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var deleteResp []RVTO2AddrEntry
	if err := json.NewDecoder(w.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("Failed to decode delete response: %v", err)
	}
	if len(deleteResp) != 1 {
		t.Errorf("Expected 1 entry in delete response, got %d", len(deleteResp))
	}
}
