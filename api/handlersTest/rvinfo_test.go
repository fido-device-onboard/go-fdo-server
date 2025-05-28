package handlersTest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

func setupTestRvServer(t *testing.T) (*httptest.Server, *sqlite.DB, func()) {
	// Create temporary database
	tempFile, err := os.CreateTemp("", "test_rvinfo_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database: %v", err)
	}
	tempDBPath := tempFile.Name()
	tempFile.Close() // Close file handle so SQLite can use it

	state, err := sqlite.Open(tempDBPath, "")
	if err != nil {
		t.Fatal(err)
	}

	err = db.InitDb(state)
	if err != nil {
		t.Fatal(err)
	}

	rvInfo, err := rvinfo.FetchRvInfo()
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(handlers.RvInfoHandler(&rvInfo)))

	// Return cleanup function that removes the temp file
	cleanup := func() {
		os.Remove(tempDBPath)
	}

	return server, state, cleanup
}

func TestRVInfoHandler(t *testing.T) {
	// Initialize OpenAPI test helper
	openAPIHelper := NewOpenAPITestHelper(t)

	// Test 1: Data Validation - POST, GET, PUT, GET sequence
	t.Run("1-Data Validation - POST/GET/PUT/GET", func(t *testing.T) {
		type rvInfoTestCase struct {
			name        string
			contentType string
			postData    string
			putData     string
		}

		testCases := []rvInfoTestCase{
			{
				name:        "RVIPAddress - JSON Content Type",
				contentType: "application/json",
				postData:    `[[[2,"127.0.0.1"]]]`,
				putData:     `[[[2,"10.0.0.1"]]]`,
			},
			{
				name:        "RVIPAddress - Plain Text Content Type",
				contentType: "text/plain",
				postData:    `[[[2,"127.0.0.1"]]]`,
				putData:     `[[[2,"10.0.0.1"]]]`,
			},
			// TODO(kgiusti): re-enable these tests once issue #29 fixed
			// {
			// 	name:        "RVIPAddress and RVDns - JSON Content Type",
			// 	contentType: "application/json",
			// 	postData:    `[[[5,"localhost"]]]`,
			// 	putData:     `[[[2,"10.0.0.1"]]]`,
			// },
			// {
			// 	name:        "Multiple Directives - RVDevOnly, RVIPAddress, RVDevPort, RVOwnerOnly, RVDns, RVOwnerPort",
			// 	contentType: "application/json",
			// 	postData:    `[[[0], [2, "10.0.0.1"], [3, 8080]], [[1], [5, "example.org"], [4, 9090]]]`,
			// 	putData:     `[[[8, true], [9, "ssid"], [10, "wifipwd"]]]`,
			// },
			// {
			// 	name:        "RVMedium, RVProtocol, RVDelaysec - JSON Content Type",
			// 	contentType: "application/json",
			// 	postData:    `[[[11, 21], [12, 2], [13, 10]]]`,
			// 	putData:     `[[[11, 0], [12, 6]]]`,
			// },
			// {
			// 	name:        "RVBypass and RVExtRV - JSON Content Type",
			// 	contentType: "application/json",
			// 	postData:    `[[[14], [15, ["test"]]]]`,
			// 	putData:     `[[[14], [15, ["foo", 1, "a", true]]]]`,
			// },
			// {
			// 	name:        "RVSvCertHash - SHA256 and HMAC-SHA384",
			// 	contentType: "application/json",
			// 	postData:    `[[[0], [6, [-16, "2990c7a64101c392d21dc0f460c2c820452f59ad815095ce64640811b702fa94"]]]]`,
			// 	putData:     `[[[7, [6, "dd60d05810fe7fdadf090f7111bdb8d0b84c1377afc663f71f4347236961177c083913f143f06b4a88d05380b69aa631"]], [0]]]`,
			// },
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Setup test server with temporary database
				testServer, testState, testCleanup := setupTestRvServer(t)
				defer testServer.Close()
				defer testState.Close()
				defer testCleanup()

				// 2. POST operation
				postReq, postResp := ExecutePostRequest(t, testServer, "/api/v1/rvinfo", tc.postData, tc.contentType)
				defer postResp.Body.Close()

				// Validate POST response data first
				if err := VerifyResponseData(tc.postData, postResp); err != nil {
					t.Fatalf("POST response validation failed: %v", err)
				}

				// Then validate OpenAPI compliance
				openAPIHelper.ValidateRequestResponse(t, postReq, postResp)

				// 3. GET after POST
				getReq1, getResp1 := ExecuteGetRequest(t, testServer, "/api/v1/rvinfo")
				defer getResp1.Body.Close()

				// Compare GET response with POST data first
				if err := VerifyResponseData(tc.postData, getResp1); err != nil {
					t.Fatalf("GET response validation failed: %v", err)
				}

				// Then validate OpenAPI compliance
				openAPIHelper.ValidateRequestResponse(t, getReq1, getResp1)

				// 4. PUT operation
				putReq, putResp := ExecutePutRequest(t, testServer, "/api/v1/rvinfo", tc.putData, tc.contentType)
				defer putResp.Body.Close()

				// Validate PUT response data first
				if err := VerifyResponseData(tc.putData, putResp); err != nil {
					t.Fatalf("PUT response validation failed: %v", err)
				}

				// Then validate OpenAPI compliance
				openAPIHelper.ValidateRequestResponse(t, putReq, putResp)

				// 5. GET after PUT
				getReq2, getResp2 := ExecuteGetRequest(t, testServer, "/api/v1/rvinfo")
				defer getResp2.Body.Close()

				// Compare second GET response with PUT data first
				if err := VerifyResponseData(tc.putData, getResp2); err != nil {
					t.Fatalf("Second GET response validation failed: %v", err)
				}

				// Then validate OpenAPI compliance
				openAPIHelper.ValidateRequestResponse(t, getReq2, getResp2)

				t.Logf("✅ Data validation test passed for %s", tc.name)
			})
		}
	})

	// Test 2: DELETE - Invalid Method (should fail regardless of data)
	t.Run("2-DELETE /api/v1/rvinfo - Invalid Method", func(t *testing.T) {
		// Setup test server with temporary database
		testServer, testState, testCleanup := setupTestRvServer(t)
		defer testServer.Close()
		defer testState.Close()
		defer testCleanup()

		// Test invalid HTTP method
		client := &http.Client{}
		deleteReq, err := http.NewRequest(http.MethodDelete, testServer.URL+"/api/v1/rvinfo", nil)
		if err != nil {
			t.Fatalf("Failed to create DELETE request: %v", err)
		}

		response, err := client.Do(deleteReq)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		defer response.Body.Close()

		// Should return method not allowed
		if response.StatusCode < 400 {
			t.Errorf("Expected error status code (>=400), got %d", response.StatusCode)
		}

		t.Logf("✅ DELETE /api/v1/rvinfo - Server correctly rejected unsupported method (status: %d)", response.StatusCode)
	})

	// Test 3: POST - Invalid Content Type (should fail on content type)
	t.Run("3-POST /api/v1/rvinfo - Invalid Content Type", func(t *testing.T) {
		// Setup test server with temporary database
		testServer, testState, testCleanup := setupTestRvServer(t)
		defer testServer.Close()
		defer testState.Close()
		defer testCleanup()

		requestBodyData := []byte(`[[[2,"127.0.0.1"],[5,"localhost"],[4,8043]]]`)

		// Test server behavior - server should reject invalid content type
		client := &http.Client{}
		serverReq, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/rvinfo", bytes.NewReader(requestBodyData))
		if err != nil {
			t.Fatalf("Failed to create server request: %v", err)
		}
		serverReq.Header.Set("Content-Type", "application/xml") // Invalid content type

		response, err := client.Do(serverReq)
		if err != nil {
			t.Fatalf("Failed to execute server request: %v", err)
		}
		defer response.Body.Close()

		// Server should reject invalid content type with error status
		if response.StatusCode < 400 {
			t.Errorf("Expected server to reject invalid content type with error status (>=400), got %d", response.StatusCode)
		}

		t.Logf("✅ POST /api/v1/rvinfo - Server correctly rejected invalid content type (status: %d)", response.StatusCode)
	})

}
