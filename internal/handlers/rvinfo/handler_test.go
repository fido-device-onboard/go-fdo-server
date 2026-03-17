// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

// setupTestDB creates a temporary SQLite database and RvInfo state for testing
func setupTestDB(t *testing.T) *state.RvInfoState {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "rvinfo_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	// Initialize gorm database directly
	gormDB, err := gorm.Open(sqlite.Open(tmpFile.Name()), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	rvInfoState, err := state.InitRvInfoDB(gormDB)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo state: %v", err)
	}

	return rvInfoState
}

// TestGetRendezvousInfo_Empty tests retrieving RV info when none exists
func TestGetRendezvousInfo_Empty(t *testing.T) {
	rvInfoState := setupTestDB(t)

	server := NewServer(rvInfoState)
	ctx := context.Background()
	req := GetRendezvousInfoRequestObject{}

	resp, err := server.GetRendezvousInfo(ctx, req)
	if err != nil {
		t.Fatalf("GetRendezvousInfo failed: %v", err)
	}

	// Should return 200 with empty array
	okResp, ok := resp.(GetRendezvousInfo200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetRendezvousInfo200JSONResponse, got %T", resp)
	}

	if len(okResp) != 0 {
		t.Errorf("Expected empty array, got %d items", len(okResp))
	}
}

// TestDeleteRendezvousInfo_NotFound tests deleting when no config exists
func TestDeleteRendezvousInfo_NotFound(t *testing.T) {
	rvInfoState := setupTestDB(t)

	server := NewServer(rvInfoState)
	ctx := context.Background()

	// Try to delete when nothing exists
	resp, err := server.DeleteRendezvousInfo(ctx, DeleteRendezvousInfoRequestObject{})
	if err != nil {
		t.Fatalf("DeleteRendezvousInfo failed: %v", err)
	}

	// Should return 200 with empty array when nothing exists
	okResp, ok := resp.(DeleteRendezvousInfo200JSONResponse)
	if !ok {
		t.Fatalf("Expected DeleteRendezvousInfo200JSONResponse, got %T", resp)
	}

	if len(okResp) != 0 {
		t.Errorf("Expected empty array, got %d items", len(okResp))
	}
}

// TestUpdateRendezvousInfo_Create tests creating new RV info
func TestUpdateRendezvousInfo_Create(t *testing.T) {
	rvInfoState := setupTestDB(t)

	server := NewServer(rvInfoState)
	ctx := context.Background()

	// Valid RV info in OpenAPI format (array of arrays of single-key objects)
	rvInfoJSON := `[[
		{"dns": "rv.example.com"},
		{"protocol": "https"},
		{"owner_port": 8443}
	]]`

	var rvInfo components.RVInfo
	if err := json.Unmarshal([]byte(rvInfoJSON), &rvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	req := UpdateRendezvousInfoRequestObject{
		Body: &rvInfo,
	}

	resp, err := server.UpdateRendezvousInfo(ctx, req)
	if err != nil {
		t.Fatalf("UpdateRendezvousInfo failed: %v", err)
	}

	// Should return 200 with the created config
	okResp, ok := resp.(UpdateRendezvousInfo200JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRendezvousInfo200JSONResponse, got %T", resp)
	}

	if len(okResp) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(okResp))
	}

	if len(okResp[0]) != 3 {
		t.Errorf("Expected 3 instructions in directive, got %d", len(okResp[0]))
	}
}

// TestGetRendezvousInfo_WithData tests retrieving RV info when it exists
func TestGetRendezvousInfo_WithData(t *testing.T) {
	rvInfoState := setupTestDB(t)

	server := NewServer(rvInfoState)
	ctx := context.Background()

	// First create RV info
	rvInfoJSON := `[[
		{"dns": "rv.example.com"},
		{"protocol": "http"},
		{"owner_port": 8080}
	]]`

	var rvInfo components.RVInfo
	if err := json.Unmarshal([]byte(rvInfoJSON), &rvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	createReq := UpdateRendezvousInfoRequestObject{
		Body: &rvInfo,
	}

	_, err := server.UpdateRendezvousInfo(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create RV info: %v", err)
	}

	// Now retrieve it
	getReq := GetRendezvousInfoRequestObject{}
	resp, err := server.GetRendezvousInfo(ctx, getReq)
	if err != nil {
		t.Fatalf("GetRendezvousInfo failed: %v", err)
	}

	// Should return 200 with data
	okResp, ok := resp.(GetRendezvousInfo200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetRendezvousInfo200JSONResponse, got %T", resp)
	}

	if len(okResp) != 1 {
		t.Errorf("Expected 1 directive, got %d", len(okResp))
	}

	// Verify the data matches what we stored
	if len(okResp[0]) != 3 {
		t.Errorf("Expected 3 instructions, got %d", len(okResp[0]))
	}
}

// TestUpdateRendezvousInfo_InvalidData tests validation error handling
func TestUpdateRendezvousInfo_InvalidData(t *testing.T) {
	rvInfoState := setupTestDB(t)

	server := NewServer(rvInfoState)
	ctx := context.Background()

	// Invalid RV info - missing DNS/IP (required by spec)
	invalidRvInfoJSON := `[[
		{"protocol": "http"},
		{"owner_port": 8080}
	]]`

	var invalidRvInfo components.RVInfo
	if err := json.Unmarshal([]byte(invalidRvInfoJSON), &invalidRvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	req := UpdateRendezvousInfoRequestObject{
		Body: &invalidRvInfo,
	}

	resp, err := server.UpdateRendezvousInfo(ctx, req)
	if err != nil {
		t.Fatalf("UpdateRendezvousInfo returned unexpected error: %v", err)
	}

	// Should return 400 Bad Request
	badResp, ok := resp.(UpdateRendezvousInfo400JSONResponse)
	if !ok {
		t.Fatalf("Expected UpdateRendezvousInfo400JSONResponse for invalid data, got %T", resp)
	}

	// Verify error message contains validation details
	if badResp.Message == "" {
		t.Errorf("Expected error message, got empty string")
	}
}
