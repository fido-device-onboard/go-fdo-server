// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/api/openapi"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/testdata"
)

func setupTestDB(t *testing.T) {
	_, err := db.InitDb("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}
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
	name               string
	voucherData        []byte
	ownerKey           crypto.PublicKey // Owner key to configure handler with
	expectedStatusCode int
	expectedBodyOneOf  []string
}

func TestInsertVoucherHandler(t *testing.T) {
	setupTestDB(t)
	testData := setupTestData(t)

	testCases := []voucherTestCase{
		{
			// Valid unextended voucher
			name:               "Valid voucher is accepted",
			voucherData:        testData.validVoucherPEM,
			ownerKey:           testData.ownerPublicKey, // Manufacturer key
			expectedStatusCode: http.StatusOK,
		},
		{
			// Extended voucher with corrupted signature
			name:               "Corrupted signature is rejected",
			voucherData:        testData.corruptedPEM,
			ownerKey:           testData.extendedOwnerPublicKey, // Next owner key
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyOneOf: []string{
				"verification failed",
			},
		},
		{
			// Verify voucher CBOR encoding integrity
			name:               "Invalid CBOR is rejected",
			voucherData:        testData.invalidCBORPEM,
			ownerKey:           testData.ownerPublicKey, // Doesn't matter, fails before key check
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyOneOf: []string{
				"decode CBOR",
			},
		},
		{
			// Verify PEM encoding integrity
			name:               "Invalid PEM is rejected",
			voucherData:        testData.invalidPEM,
			ownerKey:           testData.ownerPublicKey, // Doesn't matter, fails before key check
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyOneOf: []string{
				"remaining PEM content",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create HTTP request
			req := httptest.NewRequest(http.MethodPost, "/api/v1/owner/vouchers", bytes.NewReader(tc.voucherData))
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()

			// Create handler with appropriate owner key for this test case
			handler := handlers.InsertVoucherHandler([]crypto.PublicKey{tc.ownerKey})

			// Call handler
			handler(rec, req)

			// Verify status code
			if rec.Code != tc.expectedStatusCode {
				t.Errorf("Expected status %d, got %d: %s", tc.expectedStatusCode, rec.Code, rec.Body.String())
			}

			if len(tc.expectedBodyOneOf) > 0 {
				body := rec.Body.String()
				found := false
				for _, expected := range tc.expectedBodyOneOf {
					if strings.Contains(body, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected one of %v to be contained in response", tc.expectedBodyOneOf)
				}
			}
		})
	}
}

// Valid voucher with wrong/fake owner key should be rejected.
func TestInsertVoucherHandler_WrongOwnerKey(t *testing.T) {
	setupTestDB(t)

	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	wrongOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate wrong owner key: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/owner/vouchers", bytes.NewReader(voucherPEM))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler := handlers.InsertVoucherHandler([]crypto.PublicKey{wrongOwnerKey.Public()})
	handler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request for wrong owner key, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "owner key does not match") {
		t.Errorf("Expected error about owner key mismatch")
	}
}

// Test vouchers with invalid header fields are rejected.
func TestInsertVoucherHandler_InvalidHeaderFields(t *testing.T) {
	setupTestDB(t)

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
	// See: api/handlers/vouchers.go VerifyOwnershipVoucher function for related TODO
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
			expectedError: "Invalid ownership voucher\n",
		},
		{
			name: "Protocol version mismatch",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Version = fdoProtocolVersion
				v.Header.Val.Version = 100 // Mismatch
			},
			expectedError: "Invalid ownership voucher\n",
		},
		{
			name: "Zero GUID",
			modifyVoucher: func(v *fdo.Voucher) {
				// Set GUID to all zeros (invalid)
				for i := range v.Header.Val.GUID {
					v.Header.Val.GUID[i] = 0
				}
			},
			expectedError: "Invalid ownership voucher\n",
		},
		{
			name: "Empty DeviceInfo",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.DeviceInfo = ""
			},
			expectedError: "Invalid ownership voucher\n",
		},
		{
			name: "Invalid ManufacturerKey",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.ManufacturerKey.Type = 0 // Invalid type
			},
			expectedError: "Invalid ownership voucher\n",
		},
		{
			name: "Empty RvInfo",
			modifyVoucher: func(v *fdo.Voucher) {
				v.Header.Val.RvInfo = nil
			},
			expectedError: "Invalid ownership voucher\n",
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/owner/vouchers", bytes.NewReader(modifiedPEM))
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()

			// Create handler with correct owner key and verify response for each invalid voucher
			handler := handlers.InsertVoucherHandler([]crypto.PublicKey{ownerPubKey})
			handler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Voucher verification failed") {
				t.Errorf("Expected 'Voucher verification failed' in JSON response")
			}
		})
	}
}

// TestInsertVoucherHandler_ContentTypeValidation tests content type validation with CI compatibility
func TestInsertVoucherHandler_ContentTypeValidation(t *testing.T) {
	setupTestDB(t)
	testData := setupTestData(t)
	pemString := string(testData.validVoucherPEM)

	// Create valid JSON voucher response for CI compatibility test
	voucherBytes, _ := pem.Decode([]byte(pemString))
	if voucherBytes == nil {
		t.Fatalf("Failed to decode test PEM")
	}
	validVoucherJSON := fmt.Sprintf(`{"voucher":"%s","encoding":"pem","guid":"455fd681c576f8b3b51135c7f9e82e92"}`,
		base64.StdEncoding.EncodeToString(voucherBytes.Bytes))

	// Test content type validation
	testCases := []struct {
		name        string
		data        string
		contentType string
		expectCode  int
		description string
	}{
		{"valid_pem", pemString, "application/x-pem-file", http.StatusOK, "Direct PEM upload (preferred)"},
		{"valid_pem_no_content_type", pemString, "", http.StatusOK, "PEM without explicit content-type"},
		{"valid_ci_json_in_form", validVoucherJSON, "application/x-www-form-urlencoded", http.StatusOK, "CI: JSON voucher in form data"},
		{"rejected_invalid_form_data", pemString, "application/x-www-form-urlencoded", http.StatusBadRequest, "Form data without JSON"},
		{"rejected_invalid_json_form", `{"invalid":"data"}`, "application/x-www-form-urlencoded", http.StatusBadRequest, "Form data with invalid JSON"},
		{"rejected_json", `{"voucher":"test"}`, "application/json", http.StatusUnsupportedMediaType, "Direct JSON (not supported)"},
		{"rejected_text", pemString, "text/plain", http.StatusUnsupportedMediaType, "Text content type"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/owner/vouchers", strings.NewReader(tc.data))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()

			handler := handlers.InsertVoucherHandler([]crypto.PublicKey{testData.ownerPublicKey})
			handler(rec, req)

			if rec.Code != tc.expectCode {
				t.Errorf("Test '%s': Expected status %d, got %d: %s", tc.description, tc.expectCode, rec.Code, rec.Body.String())
			}

			if tc.expectCode == http.StatusOK {
				var response openapi.VoucherInsertResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
				}
				if response.Processed != 1 || response.Inserted != 1 {
					t.Errorf("Expected 1 processed and 1 inserted, got %d processed and %d inserted", response.Processed, response.Inserted)
				}
			} else if tc.expectCode == http.StatusUnsupportedMediaType {
				if !strings.Contains(rec.Body.String(), "Unsupported content type") {
					t.Errorf("Expected 'Unsupported content type' in response for %s", tc.description)
				}
			} else if tc.expectCode == http.StatusBadRequest {
				if !strings.Contains(rec.Body.String(), "Invalid") {
					t.Errorf("Expected 'Invalid' error in response for %s", tc.description)
				}
			}
		})
	}
}
