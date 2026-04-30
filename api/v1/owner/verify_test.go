// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package owner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func TestVerifyVoucher_NoEntries(t *testing.T) {
	ownerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate owner key: %v", err)
	}

	db := setupTestDB(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	// Initialize DeviceCA state with empty cert pool (required for verification)
	deviceCAState, err := state.InitTrustedDeviceCACertsDB(db)
	if err != nil {
		t.Fatalf("failed to init device CA state: %v", err)
	}

	ownerState := &state.OwnerState{
		Voucher:  voucherState,
		DeviceCA: deviceCAState,
	}

	// Create voucher with no entries
	voucher := fdo.Voucher{
		Version: 101,
		Entries: []cose.Sign1Tag[fdo.VoucherEntryPayload, []byte]{}, // Empty entries
	}

	ctx := context.Background()
	err = VerifyVoucher(ctx, voucher, ownerKey, ownerState, false)
	if err == nil {
		t.Error("expected error for voucher with no entries")
	}
	if err != nil && err.Error() != "voucher has no ownership entries" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVoucherState_Exists(t *testing.T) {
	db := setupTestDB(t)
	voucherState := &state.VoucherPersistentState{DB: db}

	guid := protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// Test non-existent voucher
	ctx := context.Background()
	exists, err := voucherState.Exists(ctx, guid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected voucher to not exist")
	}

	// Add voucher to database
	dbVoucher := state.Voucher{
		GUID:       guid[:],
		CBOR:       []byte{1, 2, 3},
		DeviceInfo: "Test Device",
	}
	if err := db.Create(&dbVoucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	// Test existing voucher
	exists, err = voucherState.Exists(ctx, guid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected voucher to exist")
	}
}
