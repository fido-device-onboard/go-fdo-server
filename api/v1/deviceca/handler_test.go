// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package deviceca

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestContentNegotiationMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		acceptHeader string
		operationID  string
		expectedType string
	}{
		{
			name:         "No Accept header - ListTrustedDeviceCACerts defaults to JSON",
			acceptHeader: "",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "No Accept header - GetTrustedDeviceCACertByFingerprint defaults to JSON",
			acceptHeader: "",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/json",
		},
		{
			name:         "Accept: application/json",
			acceptHeader: "application/json",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/json",
		},
		{
			name:         "Accept: application/x-pem-file",
			acceptHeader: "application/x-pem-file",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: application/json with quality",
			acceptHeader: "application/json;q=0.9",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: multiple types, JSON first",
			acceptHeader: "application/json, application/x-pem-file",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/json",
		},
		{
			name:         "Accept: multiple types, PEM first",
			acceptHeader: "application/x-pem-file, application/json",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: wildcard",
			acceptHeader: "*/*",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: quality factors - PEM preferred",
			acceptHeader: "application/json;q=0.8, application/x-pem-file;q=0.9",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: quality factors - JSON preferred",
			acceptHeader: "application/json;q=0.9, application/x-pem-file;q=0.8",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: quality factors with spaces",
			acceptHeader: "application/json; q=0.5, application/x-pem-file; q=1.0",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: unsupported type falls back to default",
			acceptHeader: "text/html, application/xml",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock handler that captures the context
			var capturedContentType string
			mockHandler := func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
				if ct, ok := ctx.Value(contentTypeKey).(string); ok {
					capturedContentType = ct
				}
				return nil, nil
			}

			// Wrap with middleware
			wrappedHandler := ContentNegotiationMiddleware(mockHandler, tt.operationID)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}
			w := httptest.NewRecorder()

			// Call handler
			_, _ = wrappedHandler(context.Background(), w, req, nil)

			// Verify the content type was set correctly
			if capturedContentType != tt.expectedType {
				t.Errorf("Expected content type %q, got %q", tt.expectedType, capturedContentType)
			}
		})
	}
}

// setupTestDB creates an in-memory SQLite database and returns a TrustedDeviceCACertsState.
func setupTestDB(t *testing.T) *state.TrustedDeviceCACertsState {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	s, err := state.InitTrustedDeviceCACertsDB(database)
	if err != nil {
		t.Fatalf("Failed to initialize device CA state: %v", err)
	}
	return s
}

// generateTestCertPEM creates a self-signed certificate and returns its PEM encoding
// and SHA-256 fingerprint.
func generateTestCertPEM(t *testing.T, cn string) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	hash := sha256.Sum256(certDER)
	fingerprint := hex.EncodeToString(hash[:])

	return string(pemBlock), fingerprint
}

func TestImportTrustedDeviceCACerts_ValidPEM(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	certPEM, _ := generateTestCertPEM(t, "Test CA 1")

	request := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(certPEM),
	}

	resp, err := server.ImportTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ImportTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Detected != 1 {
		t.Errorf("Expected 1 detected, got %d", result.Detected)
	}
	if result.Imported != 1 {
		t.Errorf("Expected 1 imported, got %d", result.Imported)
	}
	if result.Skipped != 0 {
		t.Errorf("Expected 0 skipped, got %d", result.Skipped)
	}
	if result.Malformed != 0 {
		t.Errorf("Expected 0 malformed, got %d", result.Malformed)
	}
	if len(result.Certs) != 1 {
		t.Errorf("Expected 1 cert in response, got %d", len(result.Certs))
	}
}

func TestImportTrustedDeviceCACerts_MultipleCerts(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	cert1PEM, _ := generateTestCertPEM(t, "Test CA 1")
	cert2PEM, _ := generateTestCertPEM(t, "Test CA 2")

	combined := cert1PEM + cert2PEM
	request := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(combined),
	}

	resp, err := server.ImportTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ImportTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Detected != 2 {
		t.Errorf("Expected 2 detected, got %d", result.Detected)
	}
	if result.Imported != 2 {
		t.Errorf("Expected 2 imported, got %d", result.Imported)
	}
}

func TestImportTrustedDeviceCACerts_InvalidPEM(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	request := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader("this is not a valid PEM"),
	}

	resp, err := server.ImportTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ImportTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Detected != 0 {
		t.Errorf("Expected 0 detected, got %d", result.Detected)
	}
	if result.Imported != 0 {
		t.Errorf("Expected 0 imported, got %d", result.Imported)
	}
}

func TestImportTrustedDeviceCACerts_MalformedCertBlock(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	// Create a PEM block with CERTIFICATE type but garbage bytes
	malformedPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a real cert")})

	request := ImportTrustedDeviceCACertsRequestObject{
		Body: bytes.NewReader(malformedPEM),
	}

	resp, err := server.ImportTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ImportTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Malformed != 1 {
		t.Errorf("Expected 1 malformed, got %d", result.Malformed)
	}
	if result.Imported != 0 {
		t.Errorf("Expected 0 imported, got %d", result.Imported)
	}
}

func TestImportTrustedDeviceCACerts_Idempotent(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	certPEM, _ := generateTestCertPEM(t, "Test CA Idempotent")

	// Import once
	request := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(certPEM),
	}
	resp, err := server.ImportTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error on first import: %v", err)
	}
	first := resp.(ImportTrustedDeviceCACerts200JSONResponse)
	if first.Imported != 1 {
		t.Fatalf("Expected 1 imported on first import, got %d", first.Imported)
	}

	// Import same cert again
	request2 := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(certPEM),
	}
	resp2, err := server.ImportTrustedDeviceCACerts(ctx, request2)
	if err != nil {
		t.Fatalf("Unexpected error on second import: %v", err)
	}
	second := resp2.(ImportTrustedDeviceCACerts200JSONResponse)
	if second.Imported != 0 {
		t.Errorf("Expected 0 imported on duplicate import, got %d", second.Imported)
	}
	if second.Skipped != 1 {
		t.Errorf("Expected 1 skipped on duplicate import, got %d", second.Skipped)
	}
}

func TestListTrustedDeviceCACerts_Empty(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	request := ListTrustedDeviceCACertsRequestObject{}
	resp, err := server.ListTrustedDeviceCACerts(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ListTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
	if len(result.Certs) != 0 {
		t.Errorf("Expected 0 certs, got %d", len(result.Certs))
	}
	if result.Limit != 20 {
		t.Errorf("Expected default limit 20, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("Expected default offset 0, got %d", result.Offset)
	}
}

func TestListTrustedDeviceCACerts_AfterImport(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	cert1PEM, _ := generateTestCertPEM(t, "List Test CA 1")
	cert2PEM, _ := generateTestCertPEM(t, "List Test CA 2")

	// Import two certs
	importReq := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(cert1PEM + cert2PEM),
	}
	_, err := server.ImportTrustedDeviceCACerts(ctx, importReq)
	if err != nil {
		t.Fatalf("Failed to import: %v", err)
	}

	// List
	listReq := ListTrustedDeviceCACertsRequestObject{}
	resp, err := server.ListTrustedDeviceCACerts(ctx, listReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(ListTrustedDeviceCACerts200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Total != 2 {
		t.Errorf("Expected total 2, got %d", result.Total)
	}
	if len(result.Certs) != 2 {
		t.Errorf("Expected 2 certs, got %d", len(result.Certs))
	}
}

func TestListTrustedDeviceCACerts_WithPagination(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	// Import 3 certs
	for i := 0; i < 3; i++ {
		certPEM, _ := generateTestCertPEM(t, "Pagination CA")
		req := ImportTrustedDeviceCACertsRequestObject{Body: strings.NewReader(certPEM)}
		if _, err := server.ImportTrustedDeviceCACerts(ctx, req); err != nil {
			t.Fatalf("Import failed: %v", err)
		}
	}

	// List with limit=2
	limit := 2
	offset := 0
	listReq := ListTrustedDeviceCACertsRequestObject{
		Params: ListTrustedDeviceCACertsParams{
			Limit:  &limit,
			Offset: &offset,
		},
	}
	resp, err := server.ListTrustedDeviceCACerts(ctx, listReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result := resp.(ListTrustedDeviceCACerts200JSONResponse)
	if result.Total != 3 {
		t.Errorf("Expected total 3, got %d", result.Total)
	}
	if len(result.Certs) != 2 {
		t.Errorf("Expected 2 certs in page, got %d", len(result.Certs))
	}
	if result.Limit != 2 {
		t.Errorf("Expected limit 2, got %d", result.Limit)
	}
}

func TestGetTrustedDeviceCACertByFingerprint_Existing(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	certPEM, fingerprint := generateTestCertPEM(t, "Get Test CA")

	// Import
	importReq := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(certPEM),
	}
	_, err := server.ImportTrustedDeviceCACerts(ctx, importReq)
	if err != nil {
		t.Fatalf("Failed to import: %v", err)
	}

	// Get by fingerprint
	getReq := GetTrustedDeviceCACertByFingerprintRequestObject{
		Fingerprint: fingerprint,
	}
	resp, err := server.GetTrustedDeviceCACertByFingerprint(ctx, getReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := resp.(GetTrustedDeviceCACertByFingerprint200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", resp)
	}

	if result.Fingerprint != fingerprint {
		t.Errorf("Expected fingerprint %q, got %q", fingerprint, result.Fingerprint)
	}
	if !strings.Contains(result.Subject, "Get Test CA") {
		t.Errorf("Expected subject to contain 'Get Test CA', got %q", result.Subject)
	}
	if result.Pem == "" {
		t.Error("Expected non-empty PEM")
	}
}

func TestGetTrustedDeviceCACertByFingerprint_NotFound(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	getReq := GetTrustedDeviceCACertByFingerprintRequestObject{
		Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	resp, err := server.GetTrustedDeviceCACertByFingerprint(ctx, getReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := resp.(GetTrustedDeviceCACertByFingerprint404JSONResponse)
	if !ok {
		t.Fatalf("Expected 404 JSON response, got %T", resp)
	}
}

func TestDeleteTrustedDeviceCACert_Existing(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	certPEM, fingerprint := generateTestCertPEM(t, "Delete Test CA")

	// Import
	importReq := ImportTrustedDeviceCACertsRequestObject{
		Body: strings.NewReader(certPEM),
	}
	_, err := server.ImportTrustedDeviceCACerts(ctx, importReq)
	if err != nil {
		t.Fatalf("Failed to import: %v", err)
	}

	// Delete
	deleteReq := DeleteTrustedDeviceCACertRequestObject{
		Fingerprint: fingerprint,
	}
	resp, err := server.DeleteTrustedDeviceCACert(ctx, deleteReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := resp.(DeleteTrustedDeviceCACert204Response)
	if !ok {
		t.Fatalf("Expected 204 response, got %T", resp)
	}

	// Verify it's gone
	getReq := GetTrustedDeviceCACertByFingerprintRequestObject{
		Fingerprint: fingerprint,
	}
	getResp, err := server.GetTrustedDeviceCACertByFingerprint(ctx, getReq)
	if err != nil {
		t.Fatalf("Unexpected error on get after delete: %v", err)
	}

	_, ok = getResp.(GetTrustedDeviceCACertByFingerprint404JSONResponse)
	if !ok {
		t.Fatalf("Expected 404 after deletion, got %T", getResp)
	}
}

func TestDeleteTrustedDeviceCACert_NotFound(t *testing.T) {
	s := setupTestDB(t)
	server := NewServer(s)
	ctx := context.Background()

	deleteReq := DeleteTrustedDeviceCACertRequestObject{
		Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	resp, err := server.DeleteTrustedDeviceCACert(ctx, deleteReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := resp.(DeleteTrustedDeviceCACert404JSONResponse)
	if !ok {
		t.Fatalf("Expected 404 JSON response, got %T", resp)
	}
}
