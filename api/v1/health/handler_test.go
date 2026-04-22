// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package health

import (
	"context"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *state.HealthState {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	healthState, err := state.InitHealthDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize Health database: %v", err)
	}

	return healthState
}

// TestGetHealth_Success verifies that GET /health returns 200 when database is healthy
func TestGetHealth_Success(t *testing.T) {
	healthState := setupTestDB(t)
	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 200 when database is healthy
	resp200, ok := resp.(GetHealth200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth200JSONResponse, got: %T", resp)
	}

	// Verify response fields
	healthStatus := resp200.HealthStatusJSONResponse
	if healthStatus.Status != "OK" {
		t.Errorf("Expected status 'OK', got: %s", healthStatus.Status)
	}

	if healthStatus.Version == "" {
		t.Error("Expected version to be non-empty")
	}

	if healthStatus.Message == "" {
		t.Error("Expected message to be non-empty")
	}

	expectedMessage := "the service is up and running"
	if healthStatus.Message != expectedMessage {
		t.Errorf("Expected message '%s', got: %s", expectedMessage, healthStatus.Message)
	}
}

// TestGetHealth_DatabaseError verifies that GET /health returns 500 when database is unavailable
func TestGetHealth_DatabaseError(t *testing.T) {
	// Create a database connection and then close it to simulate a database error
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	healthState, err := state.InitHealthDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize Health database: %v", err)
	}

	// Close the underlying database connection to force a ping error
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	sqlDB.Close()

	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 500 when database is not healthy
	resp500, ok := resp.(GetHealth500JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth500JSONResponse, got: %T", resp)
	}

	// Verify error response contains the expected message
	if resp500.Message != "database error" {
		t.Errorf("Expected error message 'database error', got: %s", resp500.Message)
	}
}

// TestGetHealth_PingFailure verifies proper error handling when Ping() fails
func TestGetHealth_PingFailure(t *testing.T) {
	// Create a database and close it to force ping to fail
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Get underlying SQL DB and close it
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	sqlDB.Close()

	healthState := &state.HealthState{DB: db}
	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return 500 when ping fails
	_, ok := resp.(GetHealth500JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth500JSONResponse, got: %T", resp)
	}
}

// TestHealthResponse_FieldsNotEmpty verifies that health response contains all required fields
func TestHealthResponse_FieldsNotEmpty(t *testing.T) {
	healthState := setupTestDB(t)
	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	resp200, ok := resp.(GetHealth200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth200JSONResponse, got: %T", resp)
	}

	healthStatus := resp200.HealthStatusJSONResponse

	// Check that all fields are populated
	if healthStatus.Version == "" {
		t.Error("Version field should not be empty")
	}
	if healthStatus.Status == "" {
		t.Error("Status field should not be empty")
	}
	if healthStatus.Message == "" {
		t.Error("Message field should not be empty")
	}
}

// TestHealthState_TypeConformance verifies the server implements the correct interface
func TestHealthState_TypeConformance(t *testing.T) {
	healthState := setupTestDB(t)
	server := NewServer(healthState)

	// This will fail at compile time if Server doesn't implement StrictServerInterface
	var _ StrictServerInterface = &server
}

// TestHealthResponse_StatusOK verifies the status field is always "OK" on success
func TestHealthResponse_StatusOK(t *testing.T) {
	healthState := setupTestDB(t)
	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	resp200, ok := resp.(GetHealth200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth200JSONResponse, got: %T", resp)
	}

	if resp200.HealthStatusJSONResponse.Status != "OK" {
		t.Errorf("Expected status to be 'OK', got: %s", resp200.HealthStatusJSONResponse.Status)
	}
}

// TestHealthError_Message verifies error response uses components.InternalServerError
func TestHealthError_Message(t *testing.T) {
	// Create a closed database to force an error
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	healthState := &state.HealthState{DB: db}

	// Close the database
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	sqlDB.Close()

	server := NewServer(healthState)

	resp, err := server.GetHealth(context.Background(), GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	resp500, ok := resp.(GetHealth500JSONResponse)
	if !ok {
		t.Fatalf("Expected GetHealth500JSONResponse, got: %T", resp)
	}

	// Verify it's using the components.InternalServerError type
	var _ components.InternalServerError = resp500.InternalServerError

	// Verify the message is set correctly
	if resp500.Message != "database error" {
		t.Errorf("Expected message 'database error', got: %s", resp500.Message)
	}
}
