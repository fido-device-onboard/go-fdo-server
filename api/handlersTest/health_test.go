package handlersTest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
)

func TestHealthHandler(t *testing.T) {
	// Initialize OpenAPI test helper
	openAPIHelper := NewOpenAPITestHelper(t)

	t.Run("GET /health - Success", func(t *testing.T) {
		// Create request
		req, err := http.NewRequest(http.MethodGet, "/health", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		// Create response recorder
		recorder := httptest.NewRecorder()

		// Execute handler
		handlers.HealthHandler(recorder, req)

		// Basic functional tests
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, recorder.Code)
		}

		var responseBody handlers.HealthResponse
		if err := json.NewDecoder(recorder.Body).Decode(&responseBody); err != nil {
			t.Errorf("Unable to parse health response: %v", err)
		}

		if responseBody.Status != "OK" {
			t.Errorf("Expected status 'OK', got '%s'", responseBody.Status)
		}

		if responseBody.Version == "" {
			t.Error("Version should not be empty")
		}

		// OpenAPI contract validation
		openAPIHelper.ValidateRequestResponse(t, req, recorder.Result())

		t.Logf("✅ GET /health - OpenAPI validation passed")
	})

	t.Run("POST /health - Method Not Allowed", func(t *testing.T) {
		// Test invalid method
		req, err := http.NewRequest(http.MethodPost, "/health", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		// Create response recorder
		recorder := httptest.NewRecorder()

		// Execute handler
		handlers.HealthHandler(recorder, req)

		// Should return 405 Method Not Allowed
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, recorder.Code)
		}

		// Note: We don't validate this against OpenAPI spec because POST /health
		// is not defined in the spec - this test verifies the handler's behavior
		// for unsupported methods, which is outside the OpenAPI contract

		t.Logf("✅ POST /health - Handler correctly returned 405 Method Not Allowed")
	})
}
