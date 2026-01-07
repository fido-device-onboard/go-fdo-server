package handlersTest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
)

func TestHealthHandler(t *testing.T) {

	healthServer := health.NewServer()
	handler := health.Handler(healthServer)
	server := httptest.NewServer(handler)
	defer server.Close()

	response, _ := http.Get(server.URL + "/health")

	if response.StatusCode != http.StatusOK {
		t.Errorf("Status code is %v", response.StatusCode)
	}

	var responseBody health.HealthResponse
	err := json.NewDecoder(response.Body).Decode(&responseBody)
	if err != nil {
		t.Errorf("Unable to parse health response %v", err)
	}

	if responseBody.Status != "healthy" {
		t.Errorf("Invalid status: %v", responseBody.Status)
	}

	// Check if Version is present (but don't enforce strict version matching)
	if responseBody.Version == nil || *responseBody.Version == "" {
		t.Errorf("Version should not be empty, got %v", responseBody.Version)
	}
	if (responseBody.Version == nil || *responseBody.Version == "") && responseBody.Status == "" {
		t.Errorf("Invalid Health Response: %v", responseBody)
	}

}
