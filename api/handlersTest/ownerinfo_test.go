package handlersTest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

func setupTestOwnerServer(t *testing.T) (*httptest.Server, *sqlite.DB, func()) {
	// Create temporary database file
	tempFile, err := os.CreateTemp("", "ownerinfo_test_*.db")
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

	server := httptest.NewServer(http.HandlerFunc(handlers.OwnerInfoHandler))

	cleanup := func() {
		server.Close()
		state.Close()
		os.Remove(tempFile.Name())
	}

	return server, state, cleanup
}

func TestOwnerInfoHandler(t *testing.T) {
	// Initialize OpenAPI test helper
	openAPIHelper := NewOpenAPITestHelper(t)

	// Test cases for data validation
	testCases := []struct {
		name        string
		contentType string
		postData    string
		putData     string
	}{
		{
			// Test case 1 - Basic OwnerInfo with application/json
			name:        "Data Validation - JSON Content Type",
			contentType: "application/json",
			postData:    `[["127.0.0.1", "localhost", 8043, 1]]`,
			putData:     `[["192.168.1.1", "example.com", 8080, 2]]`,
		},
		{
			// Test case 2 - Basic OwnerInfo with text/plain
			name:        "Data Validation - Plain Text Content Type",
			contentType: "text/plain",
			postData:    `[["127.0.0.1", "localhost", 8043, 1]]`,
			putData:     `[["192.168.1.1", "example.com", 8080, 2]]`,
		},
		{
			// Test case 3: null IP Address/DNS entry
			name:        "Data Validation - Null Values JSON Content Type",
			contentType: "application/json",
			postData:    `[[null, "localhost", 8043, 3]]`,
			putData:     `[["192.168.1.1", null, 8080, 4]]`,
		},
		{
			// Test case 4: IPv6/v4 Address, ProtoHTTPS/ProtoCoAPS
			name:        "Data Validation - IPv6 Address JSON Content Type",
			contentType: "application/json",
			postData:    `[["fd00:0:0:1::92", null, 9999, 5]]`,
			putData:     `[["192.168.1.1", null, 8080, 6]]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup isolated test server for this subtest
			server, state, cleanup := setupTestOwnerServer(t)
			defer cleanup()
			defer state.Close()

			// POST: Create OwnerInfo
			postReq, postResp := ExecutePostRequest(t, server, "/api/v1/owner/redirect", tc.postData, tc.contentType)
			defer postResp.Body.Close()

			// Verify response data matches expected data
			if err := VerifyResponseData(tc.postData, postResp); err != nil {
				t.Fatalf("POST response validation failed: %v", err)
			}

			// Then validate OpenAPI compliance
			openAPIHelper.ValidateRequestResponse(t, postReq, postResp)

			// GET: Retrieve OwnerInfo
			getReq1, getResp1 := ExecuteGetRequest(t, server, "/api/v1/owner/redirect")
			defer getResp1.Body.Close()

			// Verify response data matches expected data
			if err := VerifyResponseData(tc.postData, getResp1); err != nil {
				t.Fatalf("GET response validation failed: %v", err)
			}

			// Then validate OpenAPI compliance
			openAPIHelper.ValidateRequestResponse(t, getReq1, getResp1)

			// PUT: Update OwnerInfo
			putReq, putResp := ExecutePutRequest(t, server, "/api/v1/owner/redirect", tc.putData, tc.contentType)
			defer putResp.Body.Close()

			// Verify response data matches expected data
			if err := VerifyResponseData(tc.putData, putResp); err != nil {
				t.Fatalf("PUT response validation failed: %v", err)
			}

			// Then validate OpenAPI compliance
			openAPIHelper.ValidateRequestResponse(t, putReq, putResp)

			// GET: Verify the update
			getReq2, getResp2 := ExecuteGetRequest(t, server, "/api/v1/owner/redirect")
			defer getResp2.Body.Close()

			// Verify response data matches expected data
			if err := VerifyResponseData(tc.putData, getResp2); err != nil {
				t.Fatalf("Final GET response validation failed: %v", err)
			}

			// Then validate OpenAPI compliance
			openAPIHelper.ValidateRequestResponse(t, getReq2, getResp2)

			t.Logf("✅ Data validation test passed for %s", tc.name)
		})
	}

	// Test invalid HTTP method
	t.Run("PATCH Invalid Method", func(t *testing.T) {
		server, state, cleanup := setupTestOwnerServer(t)
		defer cleanup()
		defer state.Close()

		client := &http.Client{}
		patchReq, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/owner/redirect", nil)
		if err != nil {
			t.Fatalf("Failed to create PATCH request: %v", err)
		}

		response, err := client.Do(patchReq)
		if err != nil {
			t.Fatalf("Failed to execute PATCH request: %v", err)
		}
		defer response.Body.Close()

		// Should return method not allowed
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, response.StatusCode)
		}

		t.Logf("✅ PATCH Invalid Method - Server correctly rejected unsupported method")
	})

	// Test invalid content type
	t.Run("POST Invalid Content Type", func(t *testing.T) {
		server, state, cleanup := setupTestOwnerServer(t)
		defer cleanup()
		defer state.Close()

		client := &http.Client{}
		postReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/owner/redirect", bytes.NewReader([]byte(`[["127.0.0.1", "localhost", 8043, 2]]`)))
		if err != nil {
			t.Fatalf("Failed to create POST request: %v", err)
		}
		postReq.Header.Set("Content-Type", "application/xml")

		response, err := client.Do(postReq)
		if err != nil {
			t.Fatalf("Failed to execute POST request: %v", err)
		}
		defer response.Body.Close()

		// Should return bad request due to unsupported content type
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, response.StatusCode)
		}

		t.Logf("✅ POST Invalid Content Type - Server correctly rejected invalid content type")
	})
}
