package handlersTest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

func setupTestTo0Server(t *testing.T) (*httptest.Server, *sqlite.DB, func()) {
	// Create temporary database
	tempFile, err := os.CreateTemp("", "test_to0_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database: %v", err)
	}
	tempDBPath := tempFile.Name()
	tempFile.Close()

	state, err := sqlite.Open(tempDBPath, "")
	if err != nil {
		t.Fatal(err)
	}

	err = db.InitDb(state)
	if err != nil {
		t.Fatal(err)
	}

	// Setup RV info for TO0 handler
	rvInfo, err := rvinfo.FetchRvInfo()
	if err != nil {
		t.Fatal(err)
	}

	// Create test server with TO0 handler
	server := httptest.NewServer(http.HandlerFunc(handlers.To0Handler(&rvInfo, state)))

	cleanup := func() {
		server.Close()
		state.Close()
		os.Remove(tempDBPath)
	}

	return server, state, cleanup
}

func TestTo0Handler(t *testing.T) {
	t.Run("POST /api/v1/to0/{guid} - Invalid GUID", func(t *testing.T) {
		server, state, cleanup := setupTestTo0Server(t)
		defer cleanup()
		defer state.Close()

		// Test with invalid GUID format
		invalidGUID := "invalid-guid-format"
		endpoint := "/api/v1/to0/" + invalidGUID

		// Execute request
		client := &http.Client{}
		execReq, err := http.NewRequest(http.MethodPost, server.URL+endpoint, nil)
		if err != nil {
			t.Fatalf("Failed to create execution request: %v", err)
		}

		response, err := client.Do(execReq)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		defer response.Body.Close()

		// Should return 400 Bad Request for invalid GUID
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, response.StatusCode)
		}

		t.Logf("✅ POST /api/v1/to0/%s - Server correctly rejected invalid GUID", invalidGUID)
	})

	t.Run("GET /api/v1/to0/{guid} - Method Not Allowed", func(t *testing.T) {
		server, state, cleanup := setupTestTo0Server(t)
		defer cleanup()
		defer state.Close()

		validGUID := "c83b50ba1b733d12c96c5cbfe9ac3168"
		endpoint := "/api/v1/to0/" + validGUID

		// Test invalid HTTP method (GET instead of POST)
		client := &http.Client{}
		response, err := client.Get(server.URL + endpoint)
		if err != nil {
			t.Fatalf("Failed to execute GET request: %v", err)
		}
		defer response.Body.Close()

		// Should return method not allowed (handler only accepts POST)
		if response.StatusCode < 400 {
			t.Errorf("Expected error status code (>=400), got %d", response.StatusCode)
		}

		t.Logf("✅ GET /api/v1/to0/%s - Server correctly rejected unsupported method", validGUID)
	})

	t.Run("POST /api/v1/to0/ - Empty GUID", func(t *testing.T) {
		server, state, cleanup := setupTestTo0Server(t)
		defer cleanup()
		defer state.Close()

		// Test with empty GUID (just the base path)
		endpoint := "/api/v1/to0/"

		client := &http.Client{}
		execReq, err := http.NewRequest(http.MethodPost, server.URL+endpoint, nil)
		if err != nil {
			t.Fatalf("Failed to create execution request: %v", err)
		}

		response, err := client.Do(execReq)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}
		defer response.Body.Close()

		// Should return 400 Bad Request for empty GUID
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, response.StatusCode)
		}

		t.Logf("✅ POST /api/v1/to0/ - Server correctly rejected empty GUID")
	})
}
