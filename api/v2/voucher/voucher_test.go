// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elnormous/contenttype"
	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/testdata"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testContext struct {
	db            *gorm.DB
	voucherState  *state.VoucherPersistentState
	deviceCAState *state.TrustedDeviceCACertsState
	ownerKey      *state.OwnerKeyPersistentState
}

func setupTestDB(t *testing.T) *testContext {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	voucherState, err := state.InitVoucherDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize voucher state: %v", err)
	}

	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize device CA state: %v", err)
	}
	loadTestDeviceCAs(t, deviceCAState)

	// Generate test owner key
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerPrivKey, protocol.Secp384r1KeyType, nil)

	return &testContext{
		db:            db,
		voucherState:  voucherState,
		deviceCAState: deviceCAState,
		ownerKey:      ownerKeyState,
	}
}

// loadTestDeviceCAs extracts device CA certificates from the test voucher's
// cert chain and imports them as trusted CAs.
func loadTestDeviceCAs(t *testing.T, deviceCAState *state.TrustedDeviceCACertsState) {
	t.Helper()

	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}

	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}

	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	if voucher.CertChain == nil || len(*voucher.CertChain) == 0 {
		t.Fatal("Test voucher has no cert chain")
	}

	var pemData strings.Builder
	for _, cert := range *voucher.CertChain {
		pemData.Write(pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}))
	}

	if _, err := deviceCAState.ImportDeviceCACertificatesFromPEM(context.Background(), pemData.String()); err != nil {
		t.Fatalf("Failed to import device CA certificates: %v", err)
	}

	if err := deviceCAState.LoadTrustedDeviceCAs(context.Background()); err != nil {
		t.Fatalf("Failed to reload trusted device CAs: %v", err)
	}
}

type testData struct {
	validVoucherPEM []byte
	corruptedPEM    []byte
	invalidCBORPEM  []byte
	invalidPEM      []byte
	ownerPublicKey  crypto.PublicKey
}

func setupTestData(t *testing.T, ownerKey *ecdsa.PrivateKey) *testData {
	// Load voucher from testdata on go-fdo library
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}

	// Parse voucher
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}

	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	// Load manufacturer key to extend the voucher to match our owner key
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("Failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("Failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse manufacturer key: %v", err)
	}

	// Extend voucher with our owner key so it will pass ownership verification
	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, ownerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("Failed to extend voucher: %v", err)
	}

	extendedVoucherBytes, _ := cbor.Marshal(extendedVoucher)
	extendedVoucherPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedVoucherBytes,
	})

	// Create corrupted voucher with invalid signature (but still owned by us)
	corruptedVoucher := *extendedVoucher
	corruptedVoucher.Entries[0].Signature = make([]byte, 96)
	for i := range corruptedVoucher.Entries[0].Signature {
		corruptedVoucher.Entries[0].Signature[i] = 0xFF
	}
	corruptedBytes, _ := cbor.Marshal(&corruptedVoucher)
	corruptedPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: corruptedBytes,
	})

	// Create invalid CBOR voucher
	invalidCBOR := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xAB, 0xCD}
	invalidCBORPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: invalidCBOR,
	})

	return &testData{
		validVoucherPEM: extendedVoucherPEM,
		corruptedPEM:    corruptedPEM,
		invalidCBORPEM:  invalidCBORPEM,
		invalidPEM:      []byte("This is not valid PEM data at all"),
		ownerPublicKey:  ownerKey.Public(),
	}
}

type voucherTestCase struct {
	name           string
	voucherData    []byte
	expectStatus   int // Expected HTTP status code (201 if imported>0, 200 if detected>0, 400 if detected=0)
	expectImported int // Expected number of imported vouchers
	expectDetected int // Expected number of detected vouchers
	expectFailed   int // Expected number of failed vouchers (malformed, verification failure, etc.)
	expectSkipped  int // Expected number of skipped vouchers (duplicates)
}

func TestInsertVoucherHandler(t *testing.T) {
	// Generate owner key that will be used for the test
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}

	testData := setupTestData(t, ownerPrivKey)

	testCases := []voucherTestCase{
		{
			name:           "Valid voucher is accepted",
			voucherData:    testData.validVoucherPEM,
			expectImported: 1,
			expectDetected: 1,
		},
		{
			name:           "Corrupted signature is failed",
			voucherData:    testData.corruptedPEM,
			expectDetected: 1,
			expectFailed:   1,
		},
		{
			name:           "Invalid CBOR is failed",
			voucherData:    testData.invalidCBORPEM,
			expectDetected: 1,
			expectFailed:   1,
		},
		{
			name:         "Non-PEM data returns 400",
			voucherData:  testData.invalidPEM,
			expectStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh test context for each test case with the same owner key
			testCtx := setupTestDB(t)
			// Use the same owner key that was used to prepare the test data
			testCtx.ownerKey = state.NewOwnerKeyPersistentState(ownerPrivKey, protocol.Secp384r1KeyType, nil)

			// Create server
			server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
			strictHandler := NewStrictHandler(&server, []StrictMiddlewareFunc{middleware.ContentNegotiationMiddleware[StrictHandlerFunc]([]contenttype.MediaType{
				contenttype.NewMediaType("application/json"),
				contenttype.NewMediaType("application/x-pem-file"),
			}, "application/json")})

			// Create mux and wire handler
			mux := http.NewServeMux()
			HandlerFromMux(strictHandler, mux)

			// Create HTTP request - use path without version prefix
			req := httptest.NewRequest(http.MethodPost, "/vouchers", bytes.NewReader(tc.voucherData))
			req.Header.Set("Content-Type", "application/x-pem-file")
			rec := httptest.NewRecorder()

			// Call handler
			mux.ServeHTTP(rec, req)

			expectedStatus := tc.expectStatus
			if expectedStatus == 0 {
				if tc.expectDetected == 0 {
					expectedStatus = http.StatusBadRequest
				} else if tc.expectImported > 0 {
					expectedStatus = http.StatusCreated
				} else {
					expectedStatus = http.StatusOK
				}
			}
			if rec.Code != expectedStatus {
				t.Fatalf("Expected status %d, got %d: %s", expectedStatus, rec.Code, rec.Body.String())
			}

			// Parse JSON response as OwnershipVouchersImportResult
			var result OwnershipVouchersImportResult
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatalf("Failed to parse JSON response: %v\nBody: %s", err, rec.Body.String())
			}

			if result.Imported != tc.expectImported {
				t.Errorf("Expected %d imported, got %d", tc.expectImported, result.Imported)
			}
			if result.Detected != tc.expectDetected {
				t.Errorf("Expected %d detected, got %d", tc.expectDetected, result.Detected)
			}
			if result.Failed != tc.expectFailed {
				t.Errorf("Expected %d failed, got %d", tc.expectFailed, result.Failed)
			}
			if result.Skipped != tc.expectSkipped {
				t.Errorf("Expected %d skipped, got %d", tc.expectSkipped, result.Skipped)
			}
			if len(result.Vouchers) != tc.expectImported {
				t.Errorf("Expected %d vouchers in result, got %d", tc.expectImported, len(result.Vouchers))
			}
		})
	}
}

// V2 API validates owner keys during import - vouchers must belong to this owner server.
func TestInsertVoucherHandler_WrongOwnerKey(t *testing.T) {
	// Create owner key for test voucher
	voucherOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate voucher owner key: %v", err)
	}

	// Create test voucher that belongs to voucherOwnerKey
	testData := setupTestData(t, voucherOwnerKey)

	// Create test context with a DIFFERENT owner key
	testCtx := setupTestDB(t)
	wrongOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate wrong owner key: %v", err)
	}

	// Create owner key state with wrong key
	wrongOwnerKeyState := state.NewOwnerKeyPersistentState(wrongOwnerKey, protocol.Secp384r1KeyType, nil)

	// Create server
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, wrongOwnerKeyState)
	strictHandler := NewStrictHandler(&server, []StrictMiddlewareFunc{middleware.ContentNegotiationMiddleware[StrictHandlerFunc]([]contenttype.MediaType{
		contenttype.NewMediaType("application/json"),
		contenttype.NewMediaType("application/x-pem-file"),
	}, "application/json")})

	// Create mux and wire handler
	mux := http.NewServeMux()
	HandlerFromMux(strictHandler, mux)

	req := httptest.NewRequest(http.MethodPost, "/vouchers", bytes.NewReader(testData.validVoucherPEM))
	req.Header.Set("Content-Type", "application/x-pem-file")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// V2 API now accepts vouchers with any owner key
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rec.Code)
	}

	var result OwnershipVouchersImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if result.Imported != 1 {
		t.Errorf("Expected 1 imported, got %d", result.Imported)
	}
	if result.Detected != 1 {
		t.Errorf("Expected 1 detected, got %d", result.Detected)
	}
	if result.Skipped != 0 {
		t.Errorf("Expected 0 skipped, got %d", result.Skipped)
	}
}

// TestConvertVoucherToAPI verifies that vouchers can be properly converted to API format
func TestConvertVoucherToAPI(t *testing.T) {
	// Load test voucher
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}

	// Parse voucher from PEM
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}

	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	// Extend the voucher to create entries
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("Failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("Failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse manufacturer key: %v", err)
	}

	nextOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}

	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, nextOwnerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("Failed to extend voucher: %v", err)
	}

	// Convert to API format
	apiVoucher, err := convertVoucherToAPI(extendedVoucher)
	if err != nil {
		t.Fatalf("Failed to convert voucher to API format: %v", err)
	}

	// Verify basic fields
	if len(string(apiVoucher.Guid)) != 32 { // GUID is 16 bytes = 32 hex characters
		t.Errorf("Expected GUID to be 32 hex characters, got %d", len(string(apiVoucher.Guid)))
	}

	if apiVoucher.DeviceInfo == "" {
		t.Error("DeviceInfo should not be empty")
	}

	if apiVoucher.ProtocolVersion != 101 { // FDO spec v1.1
		t.Errorf("Expected protocol version 101, got %d", apiVoucher.ProtocolVersion)
	}

	// Verify entries were converted
	if len(apiVoucher.Entries) != 1 {
		t.Errorf("Expected 1 entry (from extension), got %d", len(apiVoucher.Entries))
	}

	if apiVoucher.NumEntries != 1 {
		t.Errorf("Expected NumEntries to be 1, got %d", apiVoucher.NumEntries)
	}

	// Verify header fields
	if apiVoucher.Header.Guid != string(apiVoucher.Guid) {
		t.Error("Header GUID should match voucher GUID")
	}

	if apiVoucher.Header.DeviceInfo != string(apiVoucher.DeviceInfo) {
		t.Error("Header DeviceInfo should match voucher DeviceInfo")
	}

	// Verify HMAC was converted
	if apiVoucher.Hmac.Value == "" {
		t.Error("HMAC value should not be empty")
	}

	if apiVoucher.Hmac.Algorithm == "" {
		t.Error("HMAC algorithm should not be empty")
	}

	// Verify entry has required fields
	entry := apiVoucher.Entries[0]
	if entry.HeaderHash.Value == "" {
		t.Error("Entry HeaderHash should not be empty")
	}

	if entry.PreviousHash.Value == "" {
		t.Error("Entry PreviousHash should not be empty")
	}

	if entry.PublicKey.Value == "" {
		t.Error("Entry PublicKey value should not be empty")
	}
}

// importTestVoucher is a helper that imports a test voucher and returns its GUID.
func importTestVoucher(t *testing.T, server *Server, testData *testData) string {
	t.Helper()
	ctx := context.Background()
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(testData.validVoucherPEM)),
	}
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Failed to import test voucher: %v", err)
	}
	result, ok := response.(ImportOwnershipVouchers201JSONResponse)
	if !ok {
		t.Fatalf("Expected ImportOwnershipVouchers201JSONResponse, got %T", response)
	}
	if result.Imported != 1 {
		t.Fatalf("Expected 1 imported voucher, got %d", result.Imported)
	}
	return string(result.Vouchers[0].Voucher.Guid)
}

// newTestServer creates a Server with a fresh database and the given owner key,
// imports the test voucher, and returns the server and GUID.
func newTestServer(t *testing.T, ownerPrivKey *ecdsa.PrivateKey) (*Server, *testData, string) {
	t.Helper()
	testCtx := setupTestDB(t)
	testCtx.ownerKey = state.NewOwnerKeyPersistentState(ownerPrivKey, protocol.Secp384r1KeyType, nil)
	td := setupTestData(t, ownerPrivKey)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
	guid := importTestVoucher(t, &server, td)
	return &server, td, guid
}

func TestListOwnershipVouchers_Empty(t *testing.T) {
	testCtx := setupTestDB(t)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
	ctx := context.Background()

	request := ListOwnershipVouchersRequestObject{}
	response, err := server.ListOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ListOwnershipVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected ListOwnershipVouchers200JSONResponse, got %T", response)
	}

	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
	if len(result.Vouchers) != 0 {
		t.Errorf("Expected 0 vouchers, got %d", len(result.Vouchers))
	}
	if result.Limit != 20 {
		t.Errorf("Expected default limit 20, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("Expected default offset 0, got %d", result.Offset)
	}
}

func TestListOwnershipVouchers_WithVoucher(t *testing.T) {
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	server, _, guid := newTestServer(t, ownerPrivKey)
	ctx := context.Background()

	request := ListOwnershipVouchersRequestObject{}
	response, err := server.ListOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ListOwnershipVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected ListOwnershipVouchers200JSONResponse, got %T", response)
	}

	if result.Total != 1 {
		t.Errorf("Expected total 1, got %d", result.Total)
	}
	if len(result.Vouchers) != 1 {
		t.Errorf("Expected 1 voucher, got %d", len(result.Vouchers))
	}
	if string(result.Vouchers[0].Voucher.Guid) != guid {
		t.Errorf("Expected GUID %s, got %s", guid, result.Vouchers[0].Voucher.Guid)
	}
	if result.Vouchers[0].Voucher.DeviceInfo == "" {
		t.Error("Expected non-empty DeviceInfo")
	}
}

func TestListOwnershipVouchers_Pagination(t *testing.T) {
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	server, _, _ := newTestServer(t, ownerPrivKey)
	ctx := context.Background()

	// With limit=1, offset=0 we should get 1 voucher out of 1 total
	limit := 1
	offset := 0
	request := ListOwnershipVouchersRequestObject{
		Params: ListOwnershipVouchersParams{
			Limit:  &limit,
			Offset: &offset,
		},
	}
	response, err := server.ListOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ListOwnershipVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected ListOwnershipVouchers200JSONResponse, got %T", response)
	}

	if result.Total != 1 {
		t.Errorf("Expected total 1, got %d", result.Total)
	}
	if len(result.Vouchers) != 1 {
		t.Errorf("Expected 1 voucher, got %d", len(result.Vouchers))
	}
	if result.Limit != 1 {
		t.Errorf("Expected limit 1, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("Expected offset 0, got %d", result.Offset)
	}

	// With offset=1, we should get 0 vouchers (past the end)
	offset = 1
	request = ListOwnershipVouchersRequestObject{
		Params: ListOwnershipVouchersParams{
			Limit:  &limit,
			Offset: &offset,
		},
	}
	response, err = server.ListOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok = response.(ListOwnershipVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected ListOwnershipVouchers200JSONResponse, got %T", response)
	}

	if result.Total != 1 {
		t.Errorf("Expected total 1 (unchanged), got %d", result.Total)
	}
	if len(result.Vouchers) != 0 {
		t.Errorf("Expected 0 vouchers with offset past end, got %d", len(result.Vouchers))
	}
}

func TestGetOwnershipVoucherByGuid_Found(t *testing.T) {
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	server, _, guid := newTestServer(t, ownerPrivKey)
	ctx := context.Background()

	request := GetOwnershipVoucherByGuidRequestObject{Guid: guid}
	response, err := server.GetOwnershipVoucherByGuid(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(GetOwnershipVoucherByGuid200JSONResponse)
	if !ok {
		t.Fatalf("Expected GetOwnershipVoucherByGuid200JSONResponse, got %T", response)
	}

	if string(result.Guid) != guid {
		t.Errorf("Expected GUID %s, got %s", guid, result.Guid)
	}
	if result.DeviceInfo == "" {
		t.Error("Expected non-empty DeviceInfo")
	}
	if result.ProtocolVersion != 101 {
		t.Errorf("Expected protocol version 101, got %d", result.ProtocolVersion)
	}
	if result.NumEntries < 1 {
		t.Errorf("Expected at least 1 entry, got %d", result.NumEntries)
	}
	if result.Hmac.Value == "" {
		t.Error("Expected non-empty HMAC value")
	}
}

func TestGetOwnershipVoucherByGuid_NotFound(t *testing.T) {
	testCtx := setupTestDB(t)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
	ctx := context.Background()

	request := GetOwnershipVoucherByGuidRequestObject{Guid: "00000000000000000000000000000000"}
	response, err := server.GetOwnershipVoucherByGuid(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := response.(GetOwnershipVoucherByGuid404JSONResponse)
	if !ok {
		t.Fatalf("Expected GetOwnershipVoucherByGuid404JSONResponse, got %T", response)
	}
}

func TestDeleteOwnershipVoucher_Found(t *testing.T) {
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	server, _, guid := newTestServer(t, ownerPrivKey)
	ctx := context.Background()

	// Delete the voucher
	request := DeleteOwnershipVoucherRequestObject{Guid: guid}
	response, err := server.DeleteOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := response.(DeleteOwnershipVoucher204Response)
	if !ok {
		t.Fatalf("Expected DeleteOwnershipVoucher204Response, got %T", response)
	}

	// Verify the voucher is actually gone
	getRequest := GetOwnershipVoucherByGuidRequestObject{Guid: guid}
	getResponse, err := server.GetOwnershipVoucherByGuid(ctx, getRequest)
	if err != nil {
		t.Fatalf("Unexpected error on get after delete: %v", err)
	}
	_, ok = getResponse.(GetOwnershipVoucherByGuid404JSONResponse)
	if !ok {
		t.Fatalf("Expected 404 after deletion, got %T", getResponse)
	}
}

func TestDeleteOwnershipVoucher_NotFound(t *testing.T) {
	testCtx := setupTestDB(t)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
	ctx := context.Background()

	request := DeleteOwnershipVoucherRequestObject{Guid: "00000000000000000000000000000000"}
	response, err := server.DeleteOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := response.(DeleteOwnershipVoucher404JSONResponse)
	if !ok {
		t.Fatalf("Expected DeleteOwnershipVoucher404JSONResponse, got %T", response)
	}
}

func TestExtendOwnershipVoucher_Success(t *testing.T) {
	ownerPrivKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}
	server, _, guid := newTestServer(t, ownerPrivKey)
	ctx := context.Background()

	// Generate the next owner key to extend to
	nextOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(nextOwnerKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal next owner public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyDER})

	request := ExtendOwnershipVoucherRequestObject{
		Guid: guid,
		Body: io.NopCloser(bytes.NewReader(pubKeyPEM)),
	}
	response, err := server.ExtendOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ExtendOwnershipVoucher200ApplicationxPemFileResponse)
	if !ok {
		t.Fatalf("Expected ExtendOwnershipVoucher200ApplicationxPemFileResponse, got %T", response)
	}

	// Read the PEM response body
	pemBytes, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Verify the response is valid PEM
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("Response is not valid PEM")
	}
	if block.Type != "OWNERSHIP VOUCHER" {
		t.Errorf("Expected PEM type 'OWNERSHIP VOUCHER', got '%s'", block.Type)
	}

	// Verify the extended voucher has one more entry
	var extendedVoucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &extendedVoucher); err != nil {
		t.Fatalf("Failed to unmarshal extended voucher: %v", err)
	}

	// The original testdata voucher was extended once during setupTestData,
	// then extended again here, so we expect 2 entries.
	if len(extendedVoucher.Entries) != 2 {
		t.Errorf("Expected 2 entries after extension, got %d", len(extendedVoucher.Entries))
	}
}

func TestExtendOwnershipVoucher_NotFound(t *testing.T) {
	testCtx := setupTestDB(t)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, testCtx.ownerKey)
	ctx := context.Background()

	// Generate a public key for the extend body
	nextOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(nextOwnerKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal next owner public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyDER})

	request := ExtendOwnershipVoucherRequestObject{
		Guid: "00000000000000000000000000000000",
		Body: io.NopCloser(bytes.NewReader(pubKeyPEM)),
	}
	response, err := server.ExtendOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, ok := response.(ExtendOwnershipVoucher404JSONResponse)
	if !ok {
		t.Fatalf("Expected ExtendOwnershipVoucher404JSONResponse, got %T", response)
	}
}

func TestExtendOwnershipVoucher_NilOwnerKeyState(t *testing.T) {
	testCtx := setupTestDB(t)
	// Create a server with nil OwnerKeyState
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, nil)
	ctx := context.Background()

	// Generate a public key for the extend body
	nextOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(nextOwnerKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal next owner public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyDER})

	request := ExtendOwnershipVoucherRequestObject{
		Guid: "00000000000000000000000000000000",
		Body: io.NopCloser(bytes.NewReader(pubKeyPEM)),
	}
	response, err := server.ExtendOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ExtendOwnershipVoucher403JSONResponse)
	if !ok {
		t.Fatalf("Expected ExtendOwnershipVoucher403JSONResponse, got %T", response)
	}

	if result.Message != "No signing key configured" {
		t.Errorf("Expected message 'No signing key configured', got '%s'", result.Message)
	}
}

func TestExtendOwnershipVoucher_NonOwned(t *testing.T) {
	// Create manufacturer key to extend the testdata voucher
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("Failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("Failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse manufacturer key: %v", err)
	}

	// Generate a "different owner" key -- the voucher will be extended to this key
	differentOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate different owner key: %v", err)
	}

	// Load test voucher from testdata
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}
	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	// Extend voucher to the different owner key (not the server's key)
	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, differentOwnerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("Failed to extend voucher: %v", err)
	}

	// Set up DB and import the voucher
	testCtx := setupTestDB(t)
	ctx := context.Background()
	if err := testCtx.voucherState.AddVoucher(ctx, extendedVoucher); err != nil {
		t.Fatalf("Failed to add voucher to DB: %v", err)
	}

	// Get the GUID from the voucher header
	guidHex := fmt.Sprintf("%x", extendedVoucher.Header.Val.GUID[:])

	// Generate the server's owner key (different from differentOwnerKey)
	serverOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate server owner key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(serverOwnerKey, protocol.Secp384r1KeyType, nil)

	// Create server with the server's key (which does NOT own the voucher)
	server := NewServer(testCtx.voucherState, testCtx.deviceCAState, ownerKeyState)

	// Generate a new key PEM for the extend request body
	nextKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(nextKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal next owner public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyDER})

	request := ExtendOwnershipVoucherRequestObject{
		Guid: guidHex,
		Body: io.NopCloser(bytes.NewReader(pubKeyPEM)),
	}
	response, err := server.ExtendOwnershipVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, ok := response.(ExtendOwnershipVoucher403JSONResponse)
	if !ok {
		t.Fatalf("Expected ExtendOwnershipVoucher403JSONResponse, got %T", response)
	}

	if result.Message != "Server does not own this voucher" {
		t.Errorf("Expected message 'Server does not own this voucher', got '%s'", result.Message)
	}
}
