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
	"encoding/hex"
	"encoding/pem"
	"io"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/testdata"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *state.VoucherPersistentState {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Auto-migrate voucher schema including the DeviceOnboarding tracking table
	if err := database.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}); err != nil {
		t.Fatalf("Failed to migrate voucher schema: %v", err)
	}

	return &state.VoucherPersistentState{DB: database}
}

type testData struct {
	validVoucherPEM        []byte
	corruptedPEM           []byte
	invalidCBORPEM         []byte
	invalidPEM             []byte
	ownerPublicKey         crypto.PublicKey // For unextended voucher (manufacturer key)
	extendedOwnerPublicKey crypto.PublicKey // For extended voucher (next owner key)
}

func setupTestData(t *testing.T) *testData {
	t.Helper()
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

	ownerPubKey, err := voucher.OwnerPublicKey()
	if err != nil {
		t.Fatalf("Failed to get owner public key: %v", err)
	}

	// Load manufacturer key and extend voucher (to create Entries for signature test)
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

	// Create corrupted voucher with invalid signature
	extendedVoucher.Entries[0].Signature = make([]byte, 96)
	for i := range extendedVoucher.Entries[0].Signature {
		extendedVoucher.Entries[0].Signature[i] = 0xFF
	}
	corruptedBytes, _ := cbor.Marshal(extendedVoucher)
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
		validVoucherPEM:        voucherPEM,
		corruptedPEM:           corruptedPEM,
		invalidCBORPEM:         invalidCBORPEM,
		invalidPEM:             []byte("This is not valid PEM data at all"),
		ownerPublicKey:         ownerPubKey,           // Manufacturer key (for unextended voucher)
		extendedOwnerPublicKey: nextOwnerKey.Public(), // Next owner key (for extended voucher)
	}
}

type voucherTestCase struct {
	name              string
	voucherData       []byte
	ownerKey          crypto.PublicKey // Owner key to configure handler with
	expectSuccess     bool
	expectedBodyOneOf []string
}

func TestInsertVoucherHandler(t *testing.T) {
	voucherState := setupTestDB(t)
	testData := setupTestData(t)
	ctx := context.Background()

	testCases := []voucherTestCase{
		{
			// Valid unextended voucher
			name:          "Valid voucher is accepted",
			voucherData:   testData.validVoucherPEM,
			ownerKey:      testData.ownerPublicKey, // Manufacturer key
			expectSuccess: true,
		},
		{
			// Extended voucher with corrupted signature
			name:          "Corrupted signature is rejected",
			voucherData:   testData.corruptedPEM,
			ownerKey:      testData.extendedOwnerPublicKey, // Next owner key
			expectSuccess: false,
			expectedBodyOneOf: []string{
				"Invalid ownership voucher",
			},
		},
		{
			// Verify voucher CBOR encoding integrity
			name:          "Invalid CBOR is rejected",
			voucherData:   testData.invalidCBORPEM,
			ownerKey:      testData.ownerPublicKey, // Doesn't matter, fails before key check
			expectSuccess: false,
			expectedBodyOneOf: []string{
				"Unable to decode cbor",
			},
		},
		{
			// Verify PEM encoding integrity
			name:          "Invalid PEM is rejected",
			voucherData:   testData.invalidPEM,
			ownerKey:      testData.ownerPublicKey, // Doesn't matter, fails before key check
			expectSuccess: false,
			expectedBodyOneOf: []string{
				"Unable to decode PEM content",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create server with appropriate owner key for this test case
			server := NewServer(voucherState, []crypto.PublicKey{tc.ownerKey})

			// Create request object
			request := InsertVoucherRequestObject{
				Body: io.NopCloser(bytes.NewReader(tc.voucherData)),
			}

			// Call handler
			response, err := server.InsertVoucher(ctx, request)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Check response type
			if tc.expectSuccess {
				if _, ok := response.(InsertVoucher200TextResponse); !ok {
					t.Errorf("Expected 200 response, got %T", response)
				}
			} else {
				// For error cases, check the response body contains expected message
				var body string
				switch resp := response.(type) {
				case InsertVoucher400TextResponse:
					body = string(resp)
				case InsertVoucher500TextResponse:
					body = string(resp)
				default:
					t.Fatalf("Expected error response (400 or 500), got %T", response)
				}

				if len(tc.expectedBodyOneOf) > 0 {
					found := false
					for _, expected := range tc.expectedBodyOneOf {
						if body == expected {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected one of %v, got: %s", tc.expectedBodyOneOf, body)
					}
				}
			}
		})
	}
}

// Valid voucher with wrong/fake owner key should be rejected.
func TestInsertVoucherHandler_WrongOwnerKey(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	wrongOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate wrong owner key: %v", err)
	}

	// Create server with wrong owner key
	server := NewServer(voucherState, []crypto.PublicKey{wrongOwnerKey.Public()})

	// Create request object
	request := InsertVoucherRequestObject{
		Body: io.NopCloser(bytes.NewReader(voucherPEM)),
	}

	// Call handler
	response, err := server.InsertVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify it's a 400 error response
	resp400, ok := response.(InsertVoucher400TextResponse)
	if !ok {
		t.Errorf("Expected 400 Bad Request for wrong owner key, got %T", response)
	}

	body := string(resp400)
	expectedError := "Invalid ownership voucher"
	if body != expectedError {
		t.Errorf("Expected error '%s', got: '%s'", expectedError, body)
	}
}

// Test vouchers with invalid header fields are rejected.
func TestInsertVoucherHandler_InvalidHeaderFields(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	// Load valid voucher as base
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}
	var baseVoucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &baseVoucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}
	ownerPubKey, err := baseVoucher.OwnerPublicKey()
	if err != nil {
		t.Fatalf("Failed to get owner public key: %v", err)
	}

	// TODO: This should reference a common constant once FDOProtocolVersion is moved to a common package
	// See: api/v1/voucher/handler.go verifyOwnershipVoucher function for related TODO
	const fdoProtocolVersion uint16 = 101 // FDO spec v1.1

	testCases := []struct {
		name          string
		modifyVoucher func(*fdo.Voucher)
		expectedError string
	}{
		{
			name: "Invalid protocol version",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Version = 999 // Invalid version
				v.Header.Val.Version = 999
			},
			expectedError: "Invalid ownership voucher",
		},
		{
			name: "Protocol version mismatch",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Version = fdoProtocolVersion
				v.Header.Val.Version = 100 // Mismatch
			},
			expectedError: "Invalid ownership voucher",
		},
		{
			name: "Zero GUID",
			modifyVoucher: func(v *fdo.Voucher) {
				// Set GUID to all zeros (invalid)
				for i := range v.Header.Val.GUID {
					v.Header.Val.GUID[i] = 0
				}
			},
			expectedError: "Invalid ownership voucher",
		},
		{
			name: "Empty DeviceInfo",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.DeviceInfo = ""
			},
			expectedError: "Invalid ownership voucher",
		},
		{
			name: "Invalid ManufacturerKey",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.ManufacturerKey.Type = 0 // Invalid type
			},
			expectedError: "Invalid ownership voucher",
		},
		{
			name: "Empty RvInfo",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.RvInfo = nil
			},
			expectedError: "Invalid ownership voucher",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy of the base voucher
			var voucher fdo.Voucher
			if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
				t.Fatalf("Failed to unmarshal voucher: %v", err)
			}
			tc.modifyVoucher(&voucher)
			modifiedBytes, err := cbor.Marshal(&voucher)
			if err != nil {
				t.Fatalf("Failed to marshal modified voucher: %v", err)
			}
			modifiedPEM := pem.EncodeToMemory(&pem.Block{
				Type:  "OWNERSHIP VOUCHER",
				Bytes: modifiedBytes,
			})

			// Create server with correct owner key
			server := NewServer(voucherState, []crypto.PublicKey{ownerPubKey})

			// Create request object
			request := InsertVoucherRequestObject{
				Body: io.NopCloser(bytes.NewReader(modifiedPEM)),
			}

			// Call handler
			response, err := server.InsertVoucher(ctx, request)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify it's a 400 error response
			resp400, ok := response.(InsertVoucher400TextResponse)
			if !ok {
				t.Errorf("Expected status 400 Bad Request, got %T", response)
			}
			body := string(resp400)
			if body != tc.expectedError {
				t.Errorf("Expected error '%s', got: '%s'", tc.expectedError, body)
			}
		})
	}
}

// setupTestVouchers inserts the testdata voucher into the database via AddVoucher and returns the
// voucher and its hex-encoded GUID.
func setupTestVouchers(t *testing.T, voucherState *state.VoucherPersistentState) (*fdo.Voucher, string) {
	t.Helper()

	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}

	var ov fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	if err := voucherState.AddVoucher(context.Background(), &ov); err != nil {
		t.Fatalf("Failed to add voucher to DB: %v", err)
	}

	guidHex := hex.EncodeToString(ov.Header.Val.GUID[:])
	return &ov, guidHex
}

func TestListVouchers_Empty(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	server := NewServer(voucherState, nil)
	request := ListVouchersRequestObject{}

	response, err := server.ListVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response)
	}
	if len(resp200) != 0 {
		t.Errorf("Expected empty list, got %d vouchers", len(resp200))
	}
}

func TestListVouchers_ReturnsInsertedVoucher(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	ov, guidHex := setupTestVouchers(t, voucherState)

	server := NewServer(voucherState, nil)
	request := ListVouchersRequestObject{}

	response, err := server.ListVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response)
	}
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 voucher, got %d", len(resp200))
	}
	if resp200[0].Guid != guidHex {
		t.Errorf("Expected GUID %s, got %s", guidHex, resp200[0].Guid)
	}
	if resp200[0].DeviceInfo != ov.Header.Val.DeviceInfo {
		t.Errorf("Expected DeviceInfo %q, got %q", ov.Header.Val.DeviceInfo, resp200[0].DeviceInfo)
	}
}

func TestListVouchers_FilterByGUID(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	_, guidHex := setupTestVouchers(t, voucherState)

	server := NewServer(voucherState, nil)

	// Filter with matching GUID
	request := ListVouchersRequestObject{
		Params: ListVouchersParams{
			Guid: &guidHex,
		},
	}

	response, err := server.ListVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response)
	}
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 voucher, got %d", len(resp200))
	}
	if resp200[0].Guid != guidHex {
		t.Errorf("Expected GUID %s, got %s", guidHex, resp200[0].Guid)
	}

	// Filter with non-matching GUID (valid hex but different)
	nonMatchingGUID := "00000000000000000000000000000001"
	request2 := ListVouchersRequestObject{
		Params: ListVouchersParams{
			Guid: &nonMatchingGUID,
		},
	}

	response2, err := server.ListVouchers(ctx, request2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200_2, ok := response2.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response2)
	}
	if len(resp200_2) != 0 {
		t.Errorf("Expected 0 vouchers for non-matching GUID, got %d", len(resp200_2))
	}
}

func TestListVouchers_FilterByDeviceInfo(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	ov, _ := setupTestVouchers(t, voucherState)
	deviceInfo := ov.Header.Val.DeviceInfo

	server := NewServer(voucherState, nil)

	// Filter with matching device info
	request := ListVouchersRequestObject{
		Params: ListVouchersParams{
			DeviceInfo: &deviceInfo,
		},
	}

	response, err := server.ListVouchers(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response)
	}
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 voucher, got %d", len(resp200))
	}
	if resp200[0].DeviceInfo != deviceInfo {
		t.Errorf("Expected DeviceInfo %q, got %q", deviceInfo, resp200[0].DeviceInfo)
	}

	// Filter with non-matching device info
	noMatch := "nonexistent-device"
	request2 := ListVouchersRequestObject{
		Params: ListVouchersParams{
			DeviceInfo: &noMatch,
		},
	}

	response2, err := server.ListVouchers(ctx, request2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200_2, ok := response2.(ListVouchers200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200 JSON response, got %T", response2)
	}
	if len(resp200_2) != 0 {
		t.Errorf("Expected 0 vouchers for non-matching device info, got %d", len(resp200_2))
	}
}

func TestGetVoucherByGUID_Existing(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	_, guidHex := setupTestVouchers(t, voucherState)

	server := NewServer(voucherState, nil)
	request := GetVoucherByGUIDRequestObject{Guid: guidHex}

	response, err := server.GetVoucherByGUID(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(GetVoucherByGUID200ApplicationxPemFileResponse)
	if !ok {
		t.Fatalf("Expected 200 PEM response, got %T", response)
	}

	// Read the PEM body and verify it can be decoded
	pemData, err := io.ReadAll(resp200.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("Response body is not valid PEM")
	}
	if block.Type != "OWNERSHIP VOUCHER" {
		t.Errorf("Expected PEM type 'OWNERSHIP VOUCHER', got %q", block.Type)
	}

	// Verify the CBOR inside can be unmarshaled back to a voucher
	var ov fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
		t.Fatalf("Failed to unmarshal voucher from response: %v", err)
	}

	responseGUID := hex.EncodeToString(ov.Header.Val.GUID[:])
	if responseGUID != guidHex {
		t.Errorf("Expected GUID %s, got %s", guidHex, responseGUID)
	}
}

func TestGetVoucherByGUID_InvalidGUID(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	server := NewServer(voucherState, nil)

	testCases := []struct {
		name string
		guid string
	}{
		{name: "Too short", guid: "abcdef"},
		{name: "Non-hex characters", guid: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{name: "Empty string", guid: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := GetVoucherByGUIDRequestObject{Guid: tc.guid}

			response, err := server.GetVoucherByGUID(ctx, request)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			resp400, ok := response.(GetVoucherByGUID400TextResponse)
			if !ok {
				t.Fatalf("Expected 400 response, got %T", response)
			}

			body := string(resp400)
			if body != "Invalid GUID" {
				t.Errorf("Expected 'Invalid GUID', got %q", body)
			}
		})
	}
}

func TestGetVoucherByGUID_NotFound(t *testing.T) {
	voucherState := setupTestDB(t)
	ctx := context.Background()

	server := NewServer(voucherState, nil)

	// Valid hex GUID format but not in database
	nonExistentGUID := "00000000000000000000000000000001"
	request := GetVoucherByGUIDRequestObject{Guid: nonExistentGUID}

	response, err := server.GetVoucherByGUID(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp404, ok := response.(GetVoucherByGUID404TextResponse)
	if !ok {
		t.Fatalf("Expected 404 response, got %T", response)
	}

	body := string(resp404)
	if body != "Voucher not found" {
		t.Errorf("Expected 'Voucher not found', got %q", body)
	}
}
