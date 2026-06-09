// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package owner

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestBackwardCompatibility_RVTO2Addr verifies that the old /v1/owner/redirect path
// works alongside the new /v1/rvto2addr path
func TestBackwardCompatibility_RVTO2Addr(t *testing.T) {
	// Setup test database and state
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&state.RVTO2Addr{}); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Generate test owner key
	ownerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}

	ownerState := &state.OwnerState{
		RVTO2Addr: &state.RVTO2AddrState{DB: db},
		OwnerKey:  state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp256r1KeyType, nil),
	}

	// Create handler
	handler := &Owner{
		State: ownerState,
	}

	mux := handler.Handler()

	tests := []struct {
		name                   string
		path                   string
		method                 string
		expectMethodNotAllowed bool // true if we expect 405 Method Not Allowed (e.g., DELETE on old API)
	}{
		{
			name:   "New path - GET /v2/rvto2addr",
			path:   "/api/v2/rvto2addr",
			method: http.MethodGet,
		},
		{
			name:   "Old path - GET /v1/owner/redirect",
			path:   "/api/v1/owner/redirect",
			method: http.MethodGet,
		},
		{
			name:   "New path - PUT /v2/rvto2addr",
			path:   "/api/v2/rvto2addr",
			method: http.MethodPut,
		},
		{
			name:   "Old path - PUT /v1/owner/redirect",
			path:   "/api/v1/owner/redirect",
			method: http.MethodPut,
		},
		{
			name:   "New path - DELETE /v2/rvto2addr",
			path:   "/api/v2/rvto2addr",
			method: http.MethodDelete,
		},
		{
			name:                   "Old path - DELETE /v1/owner/redirect",
			path:                   "/api/v1/owner/redirect",
			method:                 http.MethodDelete,
			expectMethodNotAllowed: true, // Old API doesn't support DELETE
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body string
			if tt.method == http.MethodPut {
				// Valid JSON for PUT requests
				body = `[{"dns":"example.com","port":"8080","protocol":"http"}]`
			}

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(body))
			if tt.method == http.MethodPut {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// Note: The old /v1/owner/redirect API and new /v1/rvto2addr API have different behaviors:
			// - GET: Old API returns 404 when no config exists, new API returns 200 with empty array
			// - PUT: Old API returns 404 when config doesn't exist, new API creates it
			// - DELETE: Old API doesn't support DELETE (405), new API supports it
			// For backward compatibility, we verify the handler is wired correctly
			if tt.expectMethodNotAllowed {
				if rr.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s expected 405 Method Not Allowed, got %d", tt.name, rr.Code)
				}
			} else {
				if rr.Code == http.StatusMethodNotAllowed {
					t.Errorf("%s returned 405 Method Not Allowed - handler not wired correctly", tt.name)
				}
			}

			// Log the response for debugging
			t.Logf("%s: Status %d", tt.name, rr.Code)
		})
	}
}

// TestBackwardCompatibility_Vouchers verifies that the old /v1/owner/vouchers path
// works alongside the new /v1/vouchers path
func TestBackwardCompatibility_Vouchers(t *testing.T) {
	// Setup test database and state
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("Failed to init device CA state: %v", err)
	}

	// Generate test owner key
	ownerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}

	ownerState := &state.OwnerState{
		Voucher:  &state.VoucherPersistentState{DB: db},
		DeviceCA: deviceCAState,
		OwnerKey: state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp256r1KeyType, nil),
	}

	// Create handler
	handler := &Owner{
		State: ownerState,
	}

	mux := handler.Handler()

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{
			name:   "New path - GET /v2/vouchers",
			path:   "/api/v2/vouchers",
			method: http.MethodGet,
		},
		{
			name:   "Old path - GET /v1/owner/vouchers",
			path:   "/api/v1/owner/vouchers",
			method: http.MethodGet,
		},
		{
			name:   "New path - POST /v2/vouchers",
			path:   "/api/v2/vouchers",
			method: http.MethodPost,
		},
		{
			name:   "Old path - POST /v1/owner/vouchers",
			path:   "/api/v1/owner/vouchers",
			method: http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body string
			if tt.method == http.MethodPost {
				// Empty body for POST (will fail validation but handler is wired)
				body = ""
			}

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(body))
			if tt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// Both old and new paths should return the same response
			// We don't check for specific status codes here because that's tested elsewhere
			// We just verify the handler is wired correctly (not 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("%s returned 404 - handler not wired correctly", tt.name)
			}

			// Log the response for debugging
			t.Logf("%s: Status %d", tt.name, rr.Code)
		})
	}
}
