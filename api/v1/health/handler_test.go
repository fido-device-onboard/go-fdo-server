package handlersTest

import (
	"encoding/json"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(handlers.HealthHandler))
	defer server.Close()

	response, _ := http.Get(server.URL)

	if response.StatusCode != http.StatusOK {
		t.Errorf("Status code is %v", response.StatusCode)
	}

	var responseBody handlers.HealthResponse
	err := json.NewDecoder(response.Body).Decode(&responseBody)
	if err != nil {
		t.Errorf("Unable to parse health response %v", err)
	}

	if responseBody.Status != "OK" {
		t.Errorf("Invalid status: %v", responseBody.Status)
	}

	// Check if Version and Status fields are not empty
	if responseBody.Version == "" && responseBody.Status == "" {
		t.Errorf("Invalid Health Response: %v", responseBody)
	}

}
