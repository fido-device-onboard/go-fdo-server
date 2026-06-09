// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/testdata"
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

	// Initialize DeviceCA state and load an unrelated CA so the trusted list
	// is non-empty but does NOT include the voucher's actual device CA.
	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("failed to init device CA state: %v", err)
	}

	// Generate an unrelated self-signed CA certificate
	unrelatedCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate unrelated CA key: %v", err)
	}
	unrelatedCATemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Unrelated Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	unrelatedCADER, err := x509.CreateCertificate(rand.Reader, unrelatedCATemplate, unrelatedCATemplate, &unrelatedCAKey.PublicKey, unrelatedCAKey)
	if err != nil {
		t.Fatalf("failed to create unrelated CA cert: %v", err)
	}
	unrelatedCACert, err := x509.ParseCertificate(unrelatedCADER)
	if err != nil {
		t.Fatalf("failed to parse unrelated CA cert: %v", err)
	}

	ctx := context.Background()
	if _, err := deviceCAState.ImportDeviceCACertificates(ctx, []*x509.Certificate{unrelatedCACert}); err != nil {
		t.Fatalf("failed to import unrelated CA cert: %v", err)
	}

	// Generate owner key
	ownerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate owner key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(ownerKey, protocol.Secp384r1KeyType, nil)

	server := NewServer(voucherState, deviceCAState, ownerKeyState)

	// Load testdata voucher and extend it to our server key so it passes
	// ownership verification but fails device CA validation (wrong CA loaded)
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("failed to decode PEM")
	}
	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("failed to unmarshal voucher: %v", err)
	}

	// Load manufacturer key to extend the voucher
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse manufacturer key: %v", err)
	}

	// Extend voucher to our owner key
	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, ownerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("failed to extend voucher: %v", err)
	}

	extendedBytes, err := cbor.Marshal(extendedVoucher)
	if err != nil {
		t.Fatalf("failed to marshal extended voucher: %v", err)
	}
	extendedPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedBytes,
	})

	// Create import request
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(extendedPEM)),
	}

	// Import voucher - should fail because the trusted CA list contains
	// only an unrelated CA that did not sign the voucher's device cert chain
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the voucher failed verification (200 because detected>0, imported=0)
	switch r := response.(type) {
	case ImportOwnershipVouchers200JSONResponse:
		if r.Imported != 0 {
			t.Errorf("expected 0 imported, got %d", r.Imported)
		}
		if r.Failed != 1 {
			t.Errorf("expected 1 failed, got %d", r.Failed)
		}
		if len(r.Vouchers) != 0 {
			t.Errorf("expected 0 vouchers in result, got %d", len(r.Vouchers))
		}
	default:
		t.Errorf("unexpected response type: %T", response)
	}
}

func TestImportOwnershipVouchers_NonOwnedVoucher(t *testing.T) {
	db := setupTestDBForImport(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	// Generate owner key for the server AND a separate "other owner" key
	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}
	otherOwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate other owner key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(serverKey, protocol.Secp384r1KeyType, nil)

	// nil DeviceCAState so device CA check is skipped
	server := NewServer(voucherState, nil, ownerKeyState)

	// Load testdata voucher
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("failed to decode PEM")
	}
	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("failed to unmarshal voucher: %v", err)
	}

	// Load manufacturer key to extend the voucher
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse manufacturer key: %v", err)
	}

	// Extend voucher to the OTHER owner key (NOT the server's key)
	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, otherOwnerKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("failed to extend voucher: %v", err)
	}

	extendedBytes, err := cbor.Marshal(extendedVoucher)
	if err != nil {
		t.Fatalf("failed to marshal extended voucher: %v", err)
	}
	extendedPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedBytes,
	})

	// Create import request
	ctx := context.Background()
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(extendedPEM)),
	}

	// Import voucher — should succeed (not rejected)
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	switch r := response.(type) {
	case ImportOwnershipVouchers201JSONResponse:
		if r.Imported != 1 {
			t.Errorf("expected 1 imported, got %d", r.Imported)
		}
		if r.Skipped != 0 {
			t.Errorf("expected 0 skipped, got %d", r.Skipped)
		}
	default:
		t.Fatalf("unexpected response type: %T", response)
	}

	// Check the DB: ownership_verified should be false
	guid := extendedVoucher.Header.Val.GUID
	var dbVoucher state.Voucher
	if err := db.Where("guid = ?", guid[:]).First(&dbVoucher).Error; err != nil {
		t.Fatalf("failed to query voucher: %v", err)
	}
	if dbVoucher.OwnershipVerified {
		t.Error("expected ownership_verified to be false for non-owned voucher")
	}
}

func TestImportOwnershipVouchers_OwnedVoucher_SetsFlag(t *testing.T) {
	db := setupTestDBForImport(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	// Generate owner key for the server
	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(serverKey, protocol.Secp384r1KeyType, nil)

	// nil DeviceCAState so device CA check is skipped
	server := NewServer(voucherState, nil, ownerKeyState)

	// Load testdata voucher
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("failed to read test voucher: %v", err)
	}
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("failed to decode PEM")
	}
	var voucher fdo.Voucher
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("failed to unmarshal voucher: %v", err)
	}

	// Load manufacturer key to extend the voucher
	mfgKeyPEM, err := testdata.Files.ReadFile("mfg_key.pem")
	if err != nil {
		t.Fatalf("failed to read manufacturer key: %v", err)
	}
	mfgKeyBlock, _ := pem.Decode(mfgKeyPEM)
	if mfgKeyBlock == nil {
		t.Fatal("failed to decode manufacturer key PEM")
	}
	mfgKey, err := x509.ParseECPrivateKey(mfgKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse manufacturer key: %v", err)
	}

	// Extend voucher to the SERVER's key
	extendedVoucher, err := fdo.ExtendVoucher(&voucher, mfgKey, serverKey.Public().(*ecdsa.PublicKey), nil)
	if err != nil {
		t.Fatalf("failed to extend voucher: %v", err)
	}

	extendedBytes, err := cbor.Marshal(extendedVoucher)
	if err != nil {
		t.Fatalf("failed to marshal extended voucher: %v", err)
	}
	extendedPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedBytes,
	})

	// Create import request
	ctx := context.Background()
	request := ImportOwnershipVouchersRequestObject{
		Body: io.NopCloser(bytes.NewReader(extendedPEM)),
	}

	// Import voucher — should succeed
	response, err := server.ImportOwnershipVouchers(ctx, request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	switch r := response.(type) {
	case ImportOwnershipVouchers201JSONResponse:
		if r.Imported != 1 {
			t.Errorf("expected 1 imported, got %d", r.Imported)
		}
	default:
		t.Fatalf("unexpected response type: %T", response)
	}

	// Check the DB: ownership_verified should be true
	guid := extendedVoucher.Header.Val.GUID
	var dbVoucher state.Voucher
	if err := db.Where("guid = ?", guid[:]).First(&dbVoucher).Error; err != nil {
		t.Fatalf("failed to query voucher: %v", err)
	}
	if !dbVoucher.OwnershipVerified {
		t.Error("expected ownership_verified to be true for owned voucher")
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

	// Generate manufacturer key for the server
	mfgKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate manufacturer key: %v", err)
	}
	ownerKeyState := state.NewOwnerKeyPersistentState(mfgKey, protocol.Secp384r1KeyType, nil)

	server := NewServer(voucherState, deviceCAState, ownerKeyState)

	// Create manufacturer public key using protocol helper
	mfgPubKeyProto, err := protocol.NewPublicKey(protocol.Secp384r1KeyType, mfgKey.Public().(*ecdsa.PublicKey), false)
	if err != nil {
		t.Fatalf("failed to create protocol public key: %v", err)
	}

	// Create a voucher with nil cert chain (EPID device)
	// The manufacturer key in the voucher must match our server's key
	voucher := fdo.Voucher{
		Version:   101,
		CertChain: nil, // EPID device - no cert chain to verify
		Header: cbor.Bstr[fdo.VoucherHeader]{
			Val: fdo.VoucherHeader{
				Version:         101,
				GUID:            protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				RvInfo:          [][]protocol.RvInstruction{{}}, // At least one rendezvous instruction
				DeviceInfo:      "EPID Device",
				ManufacturerKey: *mfgPubKeyProto,
				CertChainHash:   nil, // Null for EPID devices
			},
		},
		Hmac:    protocol.Hmac{}, // Empty HMAC for test
		Entries: nil,             // Unextended voucher
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
		if r.Imported != 1 {
			t.Errorf("expected 1 imported, got %d", r.Imported)
		}
		if len(r.Vouchers) != 1 {
			t.Errorf("expected 1 voucher in result, got %d", len(r.Vouchers))
		}
	default:
		t.Errorf("unexpected response type: %T", response)
	}
}
