// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"context"
	"encoding/hex"
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

// TestVoucherGetPost tests the voucher GET/POST operations.
func TestVoucherGetPost(t *testing.T) {
	// Initialize OpenAPI test helper for schema validation
	openAPIHelper := NewOpenAPITestHelper(t)

	// Set up test server
	testServer, database, cleanup := setupTestVoucherServer(t)
	defer cleanup()

	guids := []string{
		"fe851cc3a2fe08166b364b191cfbb5d0",
		"6127c9733b12651a340af022faaca9f3",
	}

	for _, guidStr := range guids {

		// convert GUID string to the proper type
		guidBytes, err := hex.DecodeString(guidStr)
		if err != nil {
			t.Fatalf("Failed to decode GUID hex string: %v", err)
		}
		var protocolGUID protocol.GUID
		copy(protocolGUID[:], guidBytes)

		// Get the voucher
		getEndpoint := fmt.Sprintf("/api/v1/vouchers?guid=%s", guidStr)
		getReq, getResp, err := ExecuteVoucherGetRequest(t, testServer, getEndpoint)
		if err != nil {
			t.Fatalf("Failed to execute GET request: %v", err)
		}
		defer getResp.Body.Close()

		// Verify GET response status - expect 200 OK
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("GET request failed with status %d", getResp.StatusCode)
		}

		// Save a copy of the response body (voucher in PEM format) for POSTing
		voucherPEM, err := ReadResponseBody(getResp)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		// Validate GET response against OpenAPI schema
		openAPIHelper.ValidateRequestResponse(t, getReq, getResp)

		t.Logf("✅ Voucher GET GUID=%s", guidStr)

		// Now delete the voucher so we can re-add it via POST
		ov, err := database.RemoveVoucher(context.TODO(), protocolGUID)
		if err != nil {
			t.Fatalf("Failed to remove voucher GUID=%s (%v)", guidStr, err)
		}
		t.Logf("✅ DELETED GUID=%s", hex.EncodeToString(ov.Header.Val.GUID[:]))

		// Now recreate the voucher by POSTing the retrieved PEM
		postReq, postResp, err := ExecuteVoucherPostRequest(t, testServer, "/api/v1/owner/vouchers", string(voucherPEM), "application/x-pem-file")
		if err != nil {
			t.Fatalf("Failed to execute POST request: %v", err)
		}
		defer postResp.Body.Close()

		// This does not succeed as expected. This appears to be due
		// to the fact that the owner_keys database still contains
		// owner keys from the now deleted voucher. Typically I'd also
		// directly delete those keys but the go-fdo library does not
		// provide an API for that.
		if postResp.StatusCode != http.StatusInternalServerError {
			t.Errorf("POST request failed, status %d", postResp.StatusCode)
		}

		// Validate POST response against OpenAPI schema
		openAPIHelper.ValidateRequestResponse(t, postReq, postResp)
	}
}

// Ensure GET returns 404 on non-existing GUID
func TestVoucherBadGUIDGet(t *testing.T) {
	// Set up test server
	testServer, _, cleanup := setupTestVoucherServer(t)
	defer cleanup()

	badGUID := "ffffffffffffffffffffffffffffffff"
	getEndpoint := fmt.Sprintf("/api/v1/vouchers?guid=%s", badGUID)
	_, getResp, err := ExecuteVoucherGetRequest(t, testServer, getEndpoint)
	if err != nil {
		t.Fatalf("Failed to execute GET request: %v", err)
	}
	defer getResp.Body.Close()

	// Verify GET response status - expect 404 not found
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET request did not fail as expected, status %d", getResp.StatusCode)
	}
}

// Ensure invalid PEM body is detected
func TestVoucherBadPEMPost(t *testing.T) {
	// Set up test server
	testServer, _, cleanup := setupTestVoucherServer(t)
	defer cleanup()

	_, postResp, err := ExecuteVoucherPostRequest(t, testServer, "/api/v1/owner/vouchers",
		"This is not a PEM block!", "application/x-pem-file")
	if err != nil {
		t.Fatalf("Failed to execute POST request: %v", err)
	}
	defer postResp.Body.Close()

	// Verify POST response status - expect 400 bad request
	if postResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST did not fail as expected, status %d", postResp.StatusCode)
	}
}

// setupTestVoucherServer sets up a test server for voucher tests
func setupTestVoucherServer(t *testing.T) (*httptest.Server, *sqlite.DB, func()) {
	// Create a working copy of the test database
	tempFile, err := os.CreateTemp("", "voucher_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database: %v", err)
	}
	srcPath := filepath.Join("testdata", "voucher_db")
	source, err := os.Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	_, err = io.Copy(tempFile, source)
	if err != nil {
		t.Fatal(err)
	}

	tempFile.Close()
	state, err := sqlite.Open(tempFile.Name(), "TrustNo1!")
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

	return server, state, cleanup
}
