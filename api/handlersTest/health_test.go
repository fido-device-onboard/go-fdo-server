package handlersTest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/api/openapi"
	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

func TestHealthHandler(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(handlers.HealthHandler))
	defer server.Close()

	response, _ := http.Get(server.URL)

	if response.StatusCode != http.StatusOK {
		t.Errorf("Status code is %v", response.StatusCode)
	}

	var responseBody openapi.HealthResponse
	err := json.NewDecoder(response.Body).Decode(&responseBody)
	if err != nil {
		t.Errorf("Unable to parse health response %v", err)
	}

	if responseBody.Status != "OK" {
		t.Errorf("Invalid status: %v", responseBody.Status)
	}

	// Check if Version matches the actual version and Status fields are not empty
	if responseBody.Version != version.VERSION {
		t.Errorf("Expected version %q, got %q", version.VERSION, responseBody.Version)
	}
	if responseBody.Version == "" && responseBody.Status == "" {
		t.Errorf("Invalid Health Response: %v", responseBody)
	}

}
