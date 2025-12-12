// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
)

// Inline test helper for GUID extraction
func createVoucherByGUIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			r.SetPathValue("guid", parts[len(parts)-1])
		}
		handlers.GetVoucherByGUIDHandler(w, r)
	}
}

// TestJSONResponsesRequired validates that key Owner API endpoints return JSON
func TestJSONResponsesRequired(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"health", handlers.HealthHandler},
		{"owner_info", handlers.OwnerInfoHandler},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)

			if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Errorf("Expected JSON content type, got '%s'", rec.Header().Get("Content-Type"))
			}

			var data interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
				t.Errorf("Invalid JSON: %v", err)
			}
		})
	}
}

// TestBackwardCompatibilityPEM ensures Accept header handling works
func TestBackwardCompatibilityPEM(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest("GET", "/api/v1/vouchers/invalid", nil)
	req.Header.Set("Accept", "application/x-pem-file")
	rec := httptest.NewRecorder()
	createVoucherByGUIDHandler().ServeHTTP(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") && !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Expected text/plain or JSON for backward compatibility, got '%s'", contentType)
	}
}
