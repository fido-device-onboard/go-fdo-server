// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package resell

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
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

	if err := database.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}, &state.ReplacementVoucher{}); err != nil {
		t.Fatalf("Failed to migrate schema: %v", err)
	}

	return &state.VoucherPersistentState{DB: database}
}

// insertTestVoucher loads the testdata voucher, extends it with the given owner key,
// and inserts it into the database. Returns the extended voucher's GUID hex and the owner key.
func insertTestVoucher(t *testing.T, voucherState *state.VoucherPersistentState) (string, *ecdsa.PrivateKey) {
	t.Helper()
	ctx := context.Background()

	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM from testdata")
	}

	var v fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &v); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	// Load manufacturer key and extend the voucher so we have an owner key we control
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

	ownerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate owner key: %v", err)
	}

	extended, err := fdo.ExtendVoucher(&v, mfgKey, ownerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("Failed to extend voucher: %v", err)
	}

	// Store voucher in DB
	voucherBytes, err := cbor.Marshal(extended)
	if err != nil {
		t.Fatalf("Failed to marshal voucher: %v", err)
	}

	guid := extended.Header.Val.GUID
	dbVoucher := state.Voucher{
		GUID:       guid[:],
		CBOR:       voucherBytes,
		DeviceInfo: extended.Header.Val.DeviceInfo,
	}
	if err := voucherState.DB.WithContext(ctx).Create(&dbVoucher).Error; err != nil {
		t.Fatalf("Failed to insert voucher: %v", err)
	}

	return hex.EncodeToString(guid[:]), ownerKey
}

func newOwnerPEM(t *testing.T) []byte {
	t.Helper()
	nextOwnerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate next owner key: %v", err)
	}
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(nextOwnerKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal public key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})
}

func TestResellVoucher_InvalidGUID(t *testing.T) {
	voucherState := setupTestDB(t)
	ownerKey, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	tests := []struct {
		name     string
		guid     string
		expected string
	}{
		{"too short", "abc", "GUID is not a valid GUID"},
		{"non-hex characters", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "GUID is not a valid GUID"},
		{"too long", "3f4e5d6c7b8a9f0e1d2c3b4a5f6e7d8cab", "GUID is not a valid GUID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := ResellVoucherRequestObject{
				Guid: tc.guid,
				Body: io.NopCloser(bytes.NewReader(newOwnerPEM(t))),
			}
			response, err := server.ResellVoucher(ctx, request)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			resp400, ok := response.(ResellVoucher400TextResponse)
			if !ok {
				t.Fatalf("Expected 400, got %T", response)
			}
			if string(resp400) != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, string(resp400))
			}
		})
	}
}

func TestResellVoucher_InvalidPEM(t *testing.T) {
	voucherState := setupTestDB(t)
	ownerKey, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	request := ResellVoucherRequestObject{
		Guid: "3f4e5d6c7b8a9f0e1d2c3b4a5f6e7d8c",
		Body: io.NopCloser(bytes.NewReader([]byte("not valid PEM data"))),
	}
	response, err := server.ResellVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if _, ok := response.(ResellVoucher400TextResponse); !ok {
		t.Fatalf("Expected 400, got %T", response)
	}
}

func TestResellVoucher_PayloadTooLarge(t *testing.T) {
	voucherState := setupTestDB(t)
	ownerKey, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	// Create a payload larger than 1MB and wrap with MaxBytesReader
	// to simulate the BodySizeMiddleware behavior
	const limit = 1 << 20 // 1MB
	largeBody := []byte(strings.Repeat("A", limit+100))
	w := httptest.NewRecorder()
	limitedBody := http.MaxBytesReader(w, io.NopCloser(bytes.NewReader(largeBody)), limit)
	request := ResellVoucherRequestObject{
		Guid: "3f4e5d6c7b8a9f0e1d2c3b4a5f6e7d8c",
		Body: limitedBody,
	}
	response, err := server.ResellVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	resp413, ok := response.(ResellVoucher413TextResponse)
	if !ok {
		t.Fatalf("Expected 413, got %T", response)
	}
	if string(resp413) != "Request payload too large" {
		t.Errorf("Expected 'Request payload too large', got %q", string(resp413))
	}
}

func TestResellVoucher_InvalidPublicKey(t *testing.T) {
	voucherState := setupTestDB(t)
	guidHex, ownerKey := insertTestVoucher(t, voucherState)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	// PEM block with garbage bytes (not a valid PKIX public key)
	badPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: []byte{0xFF, 0xFF, 0xFF},
	})

	request := ResellVoucherRequestObject{
		Guid: guidHex,
		Body: io.NopCloser(bytes.NewReader(badPEM)),
	}
	response, err := server.ResellVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if _, ok := response.(ResellVoucher400TextResponse); !ok {
		t.Fatalf("Expected 400, got %T", response)
	}
}

func TestResellVoucher_NonExistentVoucher(t *testing.T) {
	voucherState := setupTestDB(t)
	ownerKey, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	request := ResellVoucherRequestObject{
		Guid: "3f4e5d6c7b8a9f0e1d2c3b4a5f6e7d8c",
		Body: io.NopCloser(bytes.NewReader(newOwnerPEM(t))),
	}
	response, err := server.ResellVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if _, ok := response.(ResellVoucher404TextResponse); !ok {
		t.Fatalf("Expected 404, got %T", response)
	}
}

func TestResellVoucher_Success(t *testing.T) {
	voucherState := setupTestDB(t)
	guidHex, ownerKey := insertTestVoucher(t, voucherState)
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)
	server := NewServer(voucherState, ownerKeyState)
	ctx := context.Background()

	request := ResellVoucherRequestObject{
		Guid: guidHex,
		Body: io.NopCloser(bytes.NewReader(newOwnerPEM(t))),
	}
	response, err := server.ResellVoucher(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ResellVoucher200ApplicationxPemFileResponse)
	if !ok {
		t.Fatalf("Expected 200, got %T", response)
	}

	// Read the response body and verify it's valid PEM
	body, err := io.ReadAll(resp200.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	block, _ := pem.Decode(body)
	if block == nil {
		t.Fatal("Response is not valid PEM")
	}
	if block.Type != "OWNERSHIP VOUCHER" {
		t.Errorf("Expected PEM type 'OWNERSHIP VOUCHER', got %q", block.Type)
	}

	// Verify the CBOR content is a valid voucher
	var v fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &v); err != nil {
		t.Fatalf("Failed to unmarshal response voucher: %v", err)
	}
}
