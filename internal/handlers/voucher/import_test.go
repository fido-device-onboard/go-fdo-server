// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"encoding/pem"
	"io"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForImport(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}, &state.DeviceCACertificate{}); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func TestImportOwnershipVouchers_UntrustedDeviceCA(t *testing.T) {
	db := setupTestDBForImport(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	// Initialize DeviceCA state with empty cert pool (no trusted CAs)
	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("failed to init device CA state: %v", err)
	}

	server := NewServer(voucherState, deviceCAState)

	// Create a simple voucher with device cert chain
	voucher := fdo.Voucher{
		Version:   101,
		CertChain: &[]*cbor.X509Certificate{}, // Empty cert chain will fail verification
		Header: cbor.Bstr[fdo.VoucherHeader]{
			Val: fdo.VoucherHeader{
				GUID:       protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				DeviceInfo: "Test Device",
			},
		},
		Entries: []cose.Sign1Tag[fdo.VoucherEntryPayload, []byte]{
			{}, // At least one entry
		},
	}

	// Marshal to CBOR
	voucherBytes, err := cbor.Marshal(voucher)
	if err != nil {
		t.Fatalf("failed to marshal voucher: %v", err)
	}

	// Encode as PEM
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: voucherBytes,
	})

	// Create import request
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(pemBytes)),
	}

	// Import voucher - should fail due to untrusted device CA
	ctx := context.Background()
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the voucher was rejected
	switch r := response.(type) {
	case ImportOwnershipVouchers201JSONResponse:
		// Parse the response to check messages
		responseStr := string(r.union)
		if !strings.Contains(responseStr, "untrusted device CA") {
			t.Errorf("expected error message about untrusted device CA, got: %s", responseStr)
		}
		if !strings.Contains(responseStr, `"imported":0`) {
			t.Errorf("expected 0 vouchers to be imported, got: %s", responseStr)
		}
	default:
		t.Errorf("unexpected response type: %T", response)
	}
}

func TestImportOwnershipVouchers_NoCertChain(t *testing.T) {
	db := setupTestDBForImport(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	// Initialize DeviceCA state with empty cert pool
	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("failed to init device CA state: %v", err)
	}

	server := NewServer(voucherState, deviceCAState)

	// Create a voucher with nil cert chain (EPID device)
	// These should pass certificate verification
	voucher := fdo.Voucher{
		Version:   101,
		CertChain: nil, // EPID device - no cert chain to verify
		Header: cbor.Bstr[fdo.VoucherHeader]{
			Val: fdo.VoucherHeader{
				GUID:       protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				DeviceInfo: "EPID Device",
			},
		},
		Entries: []cose.Sign1Tag[fdo.VoucherEntryPayload, []byte]{
			{}, // At least one entry
		},
	}

	// Marshal to CBOR
	voucherBytes, err := cbor.Marshal(voucher)
	if err != nil {
		t.Fatalf("failed to marshal voucher: %v", err)
	}

	// Encode as PEM
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: voucherBytes,
	})

	// Create import request
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(pemBytes)),
	}

	// Import voucher - should succeed for EPID devices
	ctx := context.Background()
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the voucher was imported
	switch r := response.(type) {
	case ImportOwnershipVouchers201JSONResponse:
		responseStr := string(r.union)
		if !strings.Contains(responseStr, `"imported":1`) {
			t.Errorf("expected 1 voucher to be imported, got: %s", responseStr)
		}
	default:
		t.Errorf("unexpected response type: %T", response)
	}
}
