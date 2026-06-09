// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetOwnershipVerified(t *testing.T) {
	db, state := setupVoucherTestDB(t)
	ctx := context.Background()

	voucher := createTestVoucher(t, 0xAA, "ownership-test-device")
	if err := state.AddVoucher(ctx, voucher); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}
	guid := voucher.Header.Val.GUID

	// Default should be false
	var row Voucher
	if err := db.Where("guid = ?", guid[:]).First(&row).Error; err != nil {
		t.Fatalf("Failed to read voucher row: %v", err)
	}
	if row.OwnershipVerified {
		t.Error("Expected ownership_verified to default to false")
	}

	// Set to true
	if err := state.SetOwnershipVerified(ctx, guid, true); err != nil {
		t.Fatalf("SetOwnershipVerified(true) failed: %v", err)
	}
	if err := db.Where("guid = ?", guid[:]).First(&row).Error; err != nil {
		t.Fatalf("Failed to re-read voucher row: %v", err)
	}
	if !row.OwnershipVerified {
		t.Error("Expected ownership_verified to be true after setting it")
	}

	// Set back to false
	if err := state.SetOwnershipVerified(ctx, guid, false); err != nil {
		t.Fatalf("SetOwnershipVerified(false) failed: %v", err)
	}
	if err := db.Where("guid = ?", guid[:]).First(&row).Error; err != nil {
		t.Fatalf("Failed to re-read voucher row: %v", err)
	}
	if row.OwnershipVerified {
		t.Error("Expected ownership_verified to be false after clearing it")
	}
}

func TestSetOwnershipVerified_NotFound(t *testing.T) {
	_, state := setupVoucherTestDB(t)
	ctx := context.Background()

	// Use a GUID that doesn't exist in the database.
	var missingGUID protocol.GUID
	missingGUID[15] = 0xFF

	err := state.SetOwnershipVerified(ctx, missingGUID, true)
	if !errors.Is(err, fdo.ErrNotFound) {
		t.Errorf("Expected fdo.ErrNotFound for missing GUID, got: %v", err)
	}
}

func TestReplaceVoucherInTxRaw_DefaultOwnershipVerified(t *testing.T) {
	// Verify that replaceVoucherInTxRaw creates the new voucher row with
	// ownership_verified=false (the GORM column default).
	//
	// A full ExtendVoucher test is deferred because it requires generating
	// real crypto keys (ECDSA/RSA) and constructing a valid FDO voucher
	// chain, which is beyond the scope of this unit-level coverage.
	db, state := setupVoucherTestDB(t)
	ctx := context.Background()

	// Create an initial voucher and mark it as ownership-verified.
	original := createTestVoucher(t, 0xD0, "replace-test-device")
	if err := state.AddVoucher(ctx, original); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}
	oldGUID := original.Header.Val.GUID
	if err := state.SetOwnershipVerified(ctx, oldGUID, true); err != nil {
		t.Fatalf("Failed to set ownership_verified: %v", err)
	}

	// Build a replacement voucher with a different GUID suffix.
	replacement := createTestVoucher(t, 0xD1, "replace-test-device-new")
	replacementBytes, err := cbor.Marshal(replacement)
	if err != nil {
		t.Fatalf("Failed to marshal replacement voucher: %v", err)
	}

	// Call replaceVoucherInTxRaw inside a transaction.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return replaceVoucherInTxRaw(tx, oldGUID, replacement, replacementBytes, false)
	}); err != nil {
		t.Fatalf("replaceVoucherInTxRaw failed: %v", err)
	}

	// The new row should have ownership_verified=false by default.
	var row Voucher
	newGUID := replacement.Header.Val.GUID
	if err := db.Where("guid = ?", newGUID[:]).First(&row).Error; err != nil {
		t.Fatalf("Failed to read replacement voucher row: %v", err)
	}
	if row.OwnershipVerified {
		t.Error("Expected ownership_verified to be false on replacement voucher row")
	}
}

func TestListPendingTO0Vouchers_OnlyOwned(t *testing.T) {
	db, state := setupVoucherTestDB(t)
	ctx := context.Background()

	// Insert two vouchers with device_onboarding records (to2_completed=false)
	ownedVoucher := createTestVoucher(t, 0xBB, "owned-device")
	unownedVoucher := createTestVoucher(t, 0xCC, "unowned-device")

	if err := state.AddVoucher(ctx, ownedVoucher); err != nil {
		t.Fatalf("Failed to add owned voucher: %v", err)
	}
	if err := state.AddVoucher(ctx, unownedVoucher); err != nil {
		t.Fatalf("Failed to add unowned voucher: %v", err)
	}

	// Mark only the first voucher as ownership-verified
	if err := db.Model(&Voucher{}).
		Where("guid = ?", ownedVoucher.Header.Val.GUID[:]).
		Update("ownership_verified", true).Error; err != nil {
		t.Fatalf("Failed to set ownership_verified: %v", err)
	}

	// ListPendingTO0Vouchers should return only the owned voucher
	pending, err := state.ListPendingTO0Vouchers(ctx)
	if err != nil {
		t.Fatalf("ListPendingTO0Vouchers failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending voucher, got %d", len(pending))
	}
	if pending[0].GUID[15] != 0xBB {
		t.Errorf("Expected owned voucher (GUID suffix 0xBB), got 0x%02X", pending[0].GUID[15])
	}
}

func TestMigrateOwnershipVerified(t *testing.T) {
	db, state := setupVoucherTestDB(t)
	ctx := context.Background()

	// Create two vouchers — both will have the same owner key from testdata
	v1 := createTestVoucher(t, 0xE1, "migrate-dev-1")
	v2 := createTestVoucher(t, 0xE2, "migrate-dev-2")
	if err := state.AddVoucher(ctx, v1); err != nil {
		t.Fatalf("Failed to add v1: %v", err)
	}
	if err := state.AddVoucher(ctx, v2); err != nil {
		t.Fatalf("Failed to add v2: %v", err)
	}

	// Extract the owner public key from the test voucher
	ownerPubKey, err := v1.OwnerPublicKey()
	if err != nil {
		t.Fatalf("Failed to extract owner public key: %v", err)
	}

	// Run migration with the matching key
	owned, total, err := state.MigrateOwnershipVerified(ctx, ownerPubKey)
	if err != nil {
		t.Fatalf("MigrateOwnershipVerified failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected total=2, got %d", total)
	}
	if owned != 2 {
		t.Errorf("Expected owned=2, got %d", owned)
	}

	// Verify both are marked as owned in the DB
	var rows []Voucher
	if err := db.Where("ownership_verified = ?", true).Find(&rows).Error; err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Expected 2 ownership-verified rows, got %d", len(rows))
	}

	// Run again with a different key — should clear ownership
	otherKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	owned, total, err = state.MigrateOwnershipVerified(ctx, otherKey.Public())
	if err != nil {
		t.Fatalf("MigrateOwnershipVerified with other key failed: %v", err)
	}
	if owned != 0 {
		t.Errorf("Expected owned=0 with different key, got %d", owned)
	}
	if total != 2 {
		t.Errorf("Expected total=2, got %d", total)
	}

	// Verify all flags are now false in the DB
	var verified []Voucher
	if err := db.Where("ownership_verified = ?", true).Find(&verified).Error; err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(verified) != 0 {
		t.Errorf("Expected 0 ownership-verified rows after migration with different key, got %d", len(verified))
	}
}

func TestMigrateOwnershipVerified_NilKey(t *testing.T) {
	_, state := setupVoucherTestDB(t)
	ctx := context.Background()

	v := createTestVoucher(t, 0xF0, "nil-key-dev")
	if err := state.AddVoucher(ctx, v); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}

	owned, total, err := state.MigrateOwnershipVerified(ctx, nil)
	if err != nil {
		t.Fatalf("MigrateOwnershipVerified(nil) failed: %v", err)
	}
	if owned != 0 || total != 0 {
		t.Errorf("Expected (0,0) with nil key, got (%d,%d)", owned, total)
	}
}

func TestNeedsOwnershipMigration_NewColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// First init — column doesn't exist, flag should be set
	state, err := InitVoucherDB(db)
	if err != nil {
		t.Fatalf("First InitVoucherDB failed: %v", err)
	}
	if !state.NeedsOwnershipMigration {
		t.Error("Expected NeedsOwnershipMigration=true on first init")
	}

	// Second init — column already exists, flag should not be set
	state2, err := InitVoucherDB(db)
	if err != nil {
		t.Fatalf("Second InitVoucherDB failed: %v", err)
	}
	if state2.NeedsOwnershipMigration {
		t.Error("Expected NeedsOwnershipMigration=false on second init")
	}
}
