// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *state.RvInfoState {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	rvInfoState, err := state.InitRvInfoDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize RvInfo database: %v", err)
	}

	return rvInfoState
}

// TestGetRendezvousInfo_NotFound verifies that GET returns 404 when no config exists
func TestGetRendezvousInfo_NotFound(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	resp, err := server.GetRendezvousInfo(context.Background(), GetRendezvousInfoRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 404 when no config exists (v1 behavior)
	if _, ok := resp.(GetRendezvousInfo404TextResponse); !ok {
		t.Errorf("Expected GetRendezvousInfo404TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp404, ok := resp.(GetRendezvousInfo404TextResponse); ok {
		if string(resp404) != "No rvInfo found" {
			t.Errorf("Expected 'No rvInfo found', got: %s", string(resp404))
		}
	}
}

// TestGetRendezvousInfo_Success verifies that GET returns 200 with data when config exists
func TestGetRendezvousInfo_Success(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	// Create a test configuration using V1 components type
	testData := []byte(`[
		{
			"dns": "rv.example.com",
			"protocol": "https",
			"owner_port": "8443"
		}
	]`)

	// Unmarshal to V1 components.RVInfo
	var rvInfo components.RVInfo
	if err := json.Unmarshal(testData, &rvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Convert V1 format to protocol instructions
	rvInstructions, err := RVInfoV1ToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("Failed to convert to protocol format: %v", err)
	}

	// Insert the converted data
	err = rvInfoState.CreateRVInfo(context.Background(), rvInstructions)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	resp, err := server.GetRendezvousInfo(context.Background(), GetRendezvousInfoRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 200 when config exists
	_, ok := resp.(GetRendezvousInfo200JSONResponse)
	if !ok {
		t.Errorf("Expected GetRendezvousInfo200JSONResponse, got: %T", resp)
	}
}

// TestCreateRendezvousInfo_Success verifies that POST creates new config
func TestCreateRendezvousInfo_Success(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	// Create request body from raw JSON
	testJSON := []byte(`[
		{
			"dns": "rv.example.com",
			"protocol": "https",
			"owner_port": "8443"
		}
	]`)

	var requestBody components.RVInfo
	if err := json.Unmarshal(testJSON, &requestBody); err != nil {
		t.Fatalf("Failed to unmarshal test JSON: %v", err)
	}

	resp, err := server.CreateRendezvousInfo(context.Background(), CreateRendezvousInfoRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 201 on successful create
	_, ok := resp.(CreateRendezvousInfo201JSONResponse)
	if !ok {
		t.Errorf("Expected CreateRendezvousInfo201JSONResponse, got: %T", resp)
	}
}

// TestCreateRendezvousInfo_Conflict verifies that POST returns 409 when config already exists
func TestCreateRendezvousInfo_Conflict(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	// Create initial configuration
	testData := []byte(`[{"dns":"rv.example.com","protocol":"https","owner_port":"8443"}]`)
	var rvInfo components.RVInfo
	if err := json.Unmarshal(testData, &rvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}
	rvInstructions, err := RVInfoV1ToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("Failed to convert to protocol format: %v", err)
	}
	err = rvInfoState.CreateRVInfo(context.Background(), rvInstructions)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Try to create again - should fail with 409
	testJSON := []byte(`[{"dns":"rv2.example.com","protocol":"http","owner_port":"8080"}]`)
	var requestBody components.RVInfo
	if err := json.Unmarshal(testJSON, &requestBody); err != nil {
		t.Fatalf("Failed to unmarshal test JSON: %v", err)
	}

	resp, err := server.CreateRendezvousInfo(context.Background(), CreateRendezvousInfoRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 409 when config already exists
	if _, ok := resp.(CreateRendezvousInfo409TextResponse); !ok {
		t.Errorf("Expected CreateRendezvousInfo409TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp409, ok := resp.(CreateRendezvousInfo409TextResponse); ok {
		if string(resp409) != "rvInfo already exists" {
			t.Errorf("Expected 'rvInfo already exists', got: %s", string(resp409))
		}
	}
}

// TestUpdateRendezvousInfo_Success verifies that PUT updates existing config
func TestUpdateRendezvousInfo_Success(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	// Create initial configuration
	testData := []byte(`[{"dns":"rv.example.com","protocol":"http","owner_port":"8080"}]`)
	var rvInfo components.RVInfo
	if err := json.Unmarshal(testData, &rvInfo); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}
	rvInstructions, err := RVInfoV1ToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("Failed to convert to protocol format: %v", err)
	}
	err = rvInfoState.CreateRVInfo(context.Background(), rvInstructions)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Update with new data
	testJSON := []byte(`[{"dns":"rv-new.example.com","protocol":"https","owner_port":"8443"}]`)
	var requestBody components.RVInfo
	if err := json.Unmarshal(testJSON, &requestBody); err != nil {
		t.Fatalf("Failed to unmarshal test JSON: %v", err)
	}

	resp, err := server.UpdateRendezvousInfo(context.Background(), UpdateRendezvousInfoRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 200 on successful update
	_, ok := resp.(UpdateRendezvousInfo200JSONResponse)
	if !ok {
		t.Errorf("Expected UpdateRendezvousInfo200JSONResponse, got: %T", resp)
	}
}

// TestUpdateRendezvousInfo_NotFound verifies that PUT returns 404 when config doesn't exist
func TestUpdateRendezvousInfo_NotFound(t *testing.T) {
	rvInfoState := setupTestDB(t)
	server := NewServer(rvInfoState)

	// Try to update without creating first - should fail with 404
	testJSON := []byte(`[{"dns":"rv.example.com","protocol":"https","owner_port":"8443"}]`)
	var requestBody components.RVInfo
	if err := json.Unmarshal(testJSON, &requestBody); err != nil {
		t.Fatalf("Failed to unmarshal test JSON: %v", err)
	}

	resp, err := server.UpdateRendezvousInfo(context.Background(), UpdateRendezvousInfoRequestObject{
		Body: &requestBody,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 404 when config doesn't exist (v1 behavior - requires POST first)
	if _, ok := resp.(UpdateRendezvousInfo404TextResponse); !ok {
		t.Errorf("Expected UpdateRendezvousInfo404TextResponse, got: %T", resp)
	}

	// Verify error message
	if resp404, ok := resp.(UpdateRendezvousInfo404TextResponse); ok {
		if string(resp404) != "rvInfo does not exist" {
			t.Errorf("Expected 'rvInfo does not exist', got: %s", string(resp404))
		}
	}
}
