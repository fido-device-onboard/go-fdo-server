// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"testing"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRVTO2AddrTestDB(t *testing.T) (*gorm.DB, *RVTO2AddrState) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	state, err := InitRVTO2AddrDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RVTO2Addr state: %v", err)
	}

	return db, state
}

func TestRVTO2AddrState_GetEmpty(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	addrs, err := state.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(addrs) != 0 {
		t.Errorf("Expected empty array, got %d entries", len(addrs))
	}
}

func TestRVTO2AddrState_UpdateAndGet(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Create test configuration
	dns := "owner.example.com"
	addrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
	}

	// Update
	err := state.Update(context.Background(), addrs)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Get and verify
	retrieved, err := state.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}

	if len(retrieved) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(retrieved))
	}

	if retrieved[0].DNSAddress == nil || *retrieved[0].DNSAddress != dns {
		t.Errorf("Expected DNS %s, got %v", dns, retrieved[0].DNSAddress)
	}
	if retrieved[0].Port != 8443 {
		t.Errorf("Expected port 8443, got %d", retrieved[0].Port)
	}
	if retrieved[0].TransportProtocol != protocol.HTTPSTransport {
		t.Errorf("Expected HTTPS transport, got %d", retrieved[0].TransportProtocol)
	}
}

func TestRVTO2AddrState_UpdateOverwrites(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Set initial configuration
	dns1 := "owner-old.example.com"
	addrs1 := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns1,
			Port:              8080,
			TransportProtocol: protocol.HTTPTransport,
		},
	}

	err := state.Update(context.Background(), addrs1)
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	// Update with new configuration
	dns2 := "owner-new.example.com"
	addrs2 := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns2,
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
	}

	err = state.Update(context.Background(), addrs2)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	// Verify new configuration
	retrieved, err := state.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after second update failed: %v", err)
	}

	if len(retrieved) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(retrieved))
	}

	if retrieved[0].DNSAddress == nil || *retrieved[0].DNSAddress != dns2 {
		t.Errorf("Expected new DNS %s, got %v", dns2, retrieved[0].DNSAddress)
	}
}

func TestRVTO2AddrState_DeleteReturnsValue(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Set a configuration
	dns := "owner.example.com"
	addrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns,
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
	}

	err := state.Update(context.Background(), addrs)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Delete and verify returned value
	deleted, err := state.Delete(context.Background())
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if len(deleted) != 1 {
		t.Fatalf("Expected 1 entry in deleted response, got %d", len(deleted))
	}

	if deleted[0].DNSAddress == nil || *deleted[0].DNSAddress != dns {
		t.Errorf("Expected DNS %s in deleted response, got %v", dns, deleted[0].DNSAddress)
	}

	// Verify it's actually deleted
	retrieved, err := state.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("Expected empty array after delete, got %d entries", len(retrieved))
	}
}

func TestRVTO2AddrState_DeleteEmpty(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Delete when nothing exists
	deleted, err := state.Delete(context.Background())
	if err != nil {
		t.Fatalf("Delete empty failed: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("Expected empty array when deleting empty config, got %d entries", len(deleted))
	}
}

func TestRVTO2AddrState_ValidationMissingBoth(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Try to update with entry missing both DNS and IP
	addrs := []protocol.RvTO2Addr{
		{
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
	}

	err := state.Update(context.Background(), addrs)
	if err == nil {
		t.Fatal("Expected error for missing both DNS and IP, got nil")
	}

	if err != ErrInvalidRVTO2Addr && !contains(err.Error(), "neither dns nor ip") {
		t.Errorf("Expected validation error, got: %v", err)
	}
}

func TestRVTO2AddrState_MultipleAddresses(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	dns1 := "owner-primary.example.com"
	dns2 := "owner-backup.example.com"
	addrs := []protocol.RvTO2Addr{
		{
			DNSAddress:        &dns1,
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
		{
			DNSAddress:        &dns2,
			Port:              8443,
			TransportProtocol: protocol.HTTPSTransport,
		},
	}

	err := state.Update(context.Background(), addrs)
	if err != nil {
		t.Fatalf("Update with multiple addresses failed: %v", err)
	}

	retrieved, err := state.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(retrieved))
	}

	if retrieved[0].DNSAddress == nil || *retrieved[0].DNSAddress != dns1 {
		t.Errorf("Expected first DNS %s, got %v", dns1, retrieved[0].DNSAddress)
	}
	if retrieved[1].DNSAddress == nil || *retrieved[1].DNSAddress != dns2 {
		t.Errorf("Expected second DNS %s, got %v", dns2, retrieved[1].DNSAddress)
	}
}

func TestRVTO2AddrState_CBORRoundTrip(t *testing.T) {
	_, state := setupRVTO2AddrTestDB(t)

	// Test with various transport protocols
	tests := []struct {
		name      string
		transport protocol.TransportProtocol
	}{
		{"TCP", protocol.TCPTransport},
		{"TLS", protocol.TLSTransport},
		{"HTTP", protocol.HTTPTransport},
		{"HTTPS", protocol.HTTPSTransport},
		{"CoAP", protocol.CoAPTransport},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns := "owner.example.com"
			addrs := []protocol.RvTO2Addr{
				{
					DNSAddress:        &dns,
					Port:              8443,
					TransportProtocol: tt.transport,
				},
			}

			err := state.Update(context.Background(), addrs)
			if err != nil {
				t.Fatalf("Update failed: %v", err)
			}

			retrieved, err := state.Get(context.Background())
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}

			if len(retrieved) != 1 {
				t.Fatalf("Expected 1 entry, got %d", len(retrieved))
			}

			if retrieved[0].TransportProtocol != tt.transport {
				t.Errorf("Expected transport %d, got %d", tt.transport, retrieved[0].TransportProtocol)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
