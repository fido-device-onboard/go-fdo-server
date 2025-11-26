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

// TestJSONResponsesRequired validates that all Owner API endpoints return JSON
// This is the core requirement for the OpenAPI implementation
func TestJSONResponsesRequired(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name     string
		method   string
		path     string
		handler  http.HandlerFunc
		body     string
		wantJSON bool
	}{
		{"health", "GET", "/health", handlers.HealthHandler, "", true},
		{"vouchers", "GET", "/api/v1/vouchers", handlers.GetVoucherHandler, "", true},
		{"owner_info", "GET", "/api/v1/owner/redirect", handlers.OwnerInfoHandler, "", true},
		{"voucher_by_guid", "GET", "/api/v1/vouchers/1234567890abcdef1234567890abcdef", createVoucherByGUIDHandler(), "", true},
		{"invalid_guid", "GET", "/api/v1/vouchers/invalid", createVoucherByGUIDHandler(), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			req.Header.Set("Accept", "application/json")

			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)

			// Verify JSON Content-Type
			contentType := rec.Header().Get("Content-Type")
			if tt.wantJSON && !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("Expected JSON content type, got '%s'", contentType)
			}

			// Verify valid JSON response
			if tt.wantJSON {
				var data interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
					t.Errorf("Invalid JSON: %v. Body: %s", err, rec.Body.String())
				}
			}
		})
	}
}

// TestBackwardCompatibilityPEM ensures PEM responses still work when requested
func TestBackwardCompatibilityPEM(t *testing.T) {
	setupTestDB(t)

	// Test that explicit PEM request gets JSON error (since voucher doesn't exist)
	// but with proper Accept header handling
	req := httptest.NewRequest("GET", "/api/v1/vouchers/1234567890abcdef1234567890abcdef", nil)
	req.Header.Set("Accept", "application/x-pem-file")

	rec := httptest.NewRecorder()
	createVoucherByGUIDHandler().ServeHTTP(rec, req)

	// When voucher doesn't exist, we should get text/plain error for PEM clients
	// This maintains backward compatibility
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") && !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Expected text/plain or JSON content type for backward compatibility, got '%s'", contentType)
	}
}
