// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

// ExecuteVoucherPostRequest executes a POST request with voucher data (no value wrapper)
// This is voucher-specific because vouchers are complete JSON objects that don't need
// the {"value": ...} wrapper that other endpoints expect
// Returns the validation request, response, and any error that occurred during execution
func ExecuteVoucherPostRequest(t *testing.T, server *httptest.Server, endpoint, data, contentType string) (*http.Request, *http.Response, error) {
	// Create validation request for OpenAPI validation (use data directly, no wrapper)
	validationReq, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create POST validation request: %w", err)
	}
	validationReq.Header.Set("Content-Type", contentType)

	// Create and execute actual request (use data directly, no wrapper)
	client := &http.Client{}
	execReq, err := http.NewRequest(http.MethodPost, server.URL+endpoint, strings.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create POST execution request: %w", err)
	}
	execReq.Header.Set("Content-Type", contentType)

	response, err := client.Do(execReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute POST request: %w", err)
	}

	return validationReq, response, nil
}

// ExecuteVoucherGetRequest executes a GET request for voucher data
// Returns the validation request, response, and any error that occurred during execution
func ExecuteVoucherGetRequest(t *testing.T, server *httptest.Server, endpoint string) (*http.Request, *http.Response, error) {
	// Create validation request for OpenAPI validation
	validationReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GET validation request: %w", err)
	}

	// Execute actual request
	client := &http.Client{}
	response, err := client.Get(server.URL + endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute GET request: %w", err)
	}

	return validationReq, response, nil
}

// TestVoucherHandler tests the basic voucher POST and GET functionality
func TestVoucherHandler(t *testing.T) {
	// Initialize OpenAPI test helper for schema validation
	openAPIHelper := NewOpenAPITestHelper(t)

	// Load voucher test files
	voucherFiles := loadVoucherFiles(t)

	// Test each voucher file
	for _, voucherFile := range voucherFiles {
		t.Run(voucherFile.name, func(t *testing.T) {
			// Set up test server for this subtest
			testServer, cleanup := setupTestVoucherServer(t)
			defer cleanup()

			// Execute POST request with raw voucher data (no value wrapper)
			postReq, postResp, err := ExecuteVoucherPostRequest(t, testServer, "/api/v1/owner/vouchers", voucherFile.content, "application/json")
			if err != nil {
				t.Fatalf("Failed to execute POST request: %v", err)
			}
			defer postResp.Body.Close()

			// Verify POST response status - expect 201 Created
			if postResp.StatusCode != http.StatusCreated {
				t.Fatalf("POST request failed with status %d", postResp.StatusCode)
			}

			// Extract GUID from POST response body
			postBody, err := io.ReadAll(postResp.Body)
			if err != nil {
				t.Fatalf("Failed to read POST response body: %v", err)
			}
			guidHex := strings.TrimSpace(string(postBody))

			// Reset POST response body for OpenAPI validation
			postResp.Body = io.NopCloser(strings.NewReader(string(postBody)))

			// Validate POST response against OpenAPI schema
			openAPIHelper.ValidateRequestResponse(t, postReq, postResp)

			// Execute GET request to retrieve the voucher
			getEndpoint := fmt.Sprintf("/api/v1/vouchers?guid=%s", guidHex)
			getReq, getResp, err := ExecuteVoucherGetRequest(t, testServer, getEndpoint)
			if err != nil {
				t.Fatalf("Failed to execute GET request: %v", err)
			}
			defer getResp.Body.Close()

			// Verify GET response status - expect 200 OK
			if getResp.StatusCode != http.StatusOK {
				t.Fatalf("GET request failed with status %d", getResp.StatusCode)
			}

			// Validate GET response against OpenAPI schema
			openAPIHelper.ValidateRequestResponse(t, getReq, getResp)
		})
	}

}

// TestVoucherHandler_NonExistingGUID tests GET request with a non-existing GUID
func TestVoucherHandler_NonExistingGUID(t *testing.T) {
	// Set up test server
	testServer, cleanup := setupTestVoucherServer(t)
	defer cleanup()

	// Use a non-existing GUID (valid format but not in database)
	nonExistingGUID := "ffffffffffffffffffffffffffffffff"
	getEndpoint := fmt.Sprintf("/api/v1/vouchers?guid=%s", nonExistingGUID)

	// Execute GET request without OpenAPI validation
	client := &http.Client{}
	response, err := client.Get(testServer.URL + getEndpoint)
	if err != nil {
		t.Fatalf("Failed to execute GET request: %v", err)
	}
	defer response.Body.Close()

	// Verify GET response status - expect 404 Not Found
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected status 404 Not Found for non-existing GUID, got status %d", response.StatusCode)
	}
}

// TestVoucherHandler_InvalidVoucher tests POST request with invalid voucher data
func TestVoucherHandler_InvalidVoucher(t *testing.T) {
	// Set up test server
	testServer, cleanup := setupTestVoucherServer(t)
	defer cleanup()

	// Create invalid voucher data (missing required fields, invalid structure)
	invalidVoucherData := `{
		"voucher": {
			"guid": "invalid-guid-format",
			"cbor": "not-valid-cbor-data"
		},
		"owner_keys": [
			{
				"type": "invalid-type",
				"pkcs8": "invalid-pkcs8"
			}
		]
	}`

	// Execute POST request with invalid voucher data
	client := &http.Client{}
	response, err := client.Post(
		testServer.URL+"/api/v1/owner/vouchers",
		"application/json",
		strings.NewReader(invalidVoucherData),
	)
	if err != nil {
		t.Fatalf("Failed to execute POST request: %v", err)
	}
	defer response.Body.Close()

	// Verify POST response status - expect 400 Bad Request or 422 Unprocessable Entity
	if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("Expected status 400 Bad Request or 422 Unprocessable Entity for invalid voucher, got status %d", response.StatusCode)
	}
}

// VoucherFile represents a voucher test file
type VoucherFile struct {
	name     string // test name
	filename string // file name
	content  string // raw file content
}

// loadVoucherFiles loads all voucher files from testdata directory as raw text
func loadVoucherFiles(t *testing.T) []VoucherFile {
	testDataDir := filepath.Join("testdata")

	// Read all files in testdata directory
	files, err := os.ReadDir(testDataDir)
	if err != nil {
		t.Fatalf("Failed to read testdata directory: %v", err)
	}

	var voucherFiles []VoucherFile
	for _, file := range files {
		// Check if it's a voucher file: not a directory, starts with "ov."
		if !file.IsDir() && strings.HasPrefix(file.Name(), "ov.") {
			// Load voucher file as raw text
			filePath := filepath.Join(testDataDir, file.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read voucher file %s: %v", file.Name(), err)
			}

			// Extract GUID from filename (after "ov.")
			guidPart := strings.TrimPrefix(file.Name(), "ov.")

			voucherFile := VoucherFile{
				name:     fmt.Sprintf("Voucher_GUID_%s", guidPart[:8]), // Use first 8 chars of GUID
				filename: file.Name(),
				content:  string(content),
			}

			voucherFiles = append(voucherFiles, voucherFile)
		}
	}

	if len(voucherFiles) == 0 {
		t.Fatalf("No voucher test files found in testdata directory")
	}

	return voucherFiles
}

// setupTestVoucherServer sets up a test server for voucher tests
func setupTestVoucherServer(t *testing.T) (*httptest.Server, func()) {
	// Create temporary database file
	tempFile, err := os.CreateTemp("", "voucher_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database: %v", err)
	}
	tempFile.Close()

	state, err := sqlite.Open(tempFile.Name(), "")
	if err != nil {
		t.Fatal(err)
	}

	err = db.InitDb(state)
	if err != nil {
		t.Fatal(err)
	}

	// Create test server with both voucher handlers
	rvInfo := [][]protocol.RvInstruction{} // Initialize empty RvInfo for handler
	mux := http.NewServeMux()

	// Add both POST and GET voucher endpoints
	mux.HandleFunc("/api/v1/owner/vouchers", handlers.InsertVoucherHandler(&rvInfo))
	mux.HandleFunc("/api/v1/vouchers", handlers.GetVoucherHandler)

	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		state.Close()
		os.Remove(tempFile.Name())
	}

	return server, cleanup
}
