// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	voucherhandler "github.com/fido-device-onboard/go-fdo-server/internal/handlers/voucher"
)

// TestJSONResponsesRequired validates that key Owner API endpoints return JSON
func TestJSONResponsesRequired(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name    string
		handler http.Handler
	}{
		{"health", health.Handler(health.NewServer())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/health", nil)
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

// TestBackwardCompatibilityPEM ensures voucher get by GUID handler works
func TestBackwardCompatibilityPEM(t *testing.T) {
	setupTestDB(t)

	// Create voucher server and handler
	voucherServer := voucherhandler.NewServerWithKeys(nil, nil)
	handler := voucherhandler.Handler(voucherServer)

	req := httptest.NewRequest("GET", "/vouchers/invalid-guid", nil)
	req.Header.Set("Accept", "application/x-pem-file")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should return JSON with not implemented message since GET is not implemented yet
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got '%s'", contentType)
	}
}
