// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Enable foreign key constraints in SQLite (required for CASCADE DELETE)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	return db
}

func TestCleanupExpiredBlobs(t *testing.T) {
	db := setupTestDB(t)
	state, err := InitRendezvousBlobDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize rendezvous blob DB: %v", err)
	}

	// Create test voucher and to1d
	guid := protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	voucher := &fdo.Voucher{
		Header: cbor.Bstr[fdo.VoucherHeader]{
			Val: fdo.VoucherHeader{
				GUID: guid,
			},
		},
	}
	to1d := &cose.Sign1[protocol.To1d, []byte]{}

	ctx := context.Background()

	// Insert an expired blob
	expiredTime := time.Now().Add(-1 * time.Hour)
	if err := state.SetRVBlob(ctx, voucher, to1d, expiredTime); err != nil {
		t.Fatalf("Failed to set expired blob: %v", err)
	}

	// Insert a non-expired blob
	guid2 := protocol.GUID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	voucher = &fdo.Voucher{
		Header: cbor.Bstr[fdo.VoucherHeader]{
			Val: fdo.VoucherHeader{
				GUID: guid2,
			},
		},
	}
	futureTime := time.Now().Add(1 * time.Hour)
	if err := state.SetRVBlob(ctx, voucher, to1d, futureTime); err != nil {
		t.Fatalf("Failed to set non-expired blob: %v", err)
	}

	// Verify both blobs exist in DB
	var count int64
	db.Model(&RvBlob{}).Count(&count)
	if count != 2 {
		t.Fatalf("Expected 2 blobs, got %d", count)
	}

	// Run cleanup
	deletedCount, err := state.CleanupExpiredBlobs(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("Expected 1 blob to be deleted, got %d", deletedCount)
	}

	// Verify only the non-expired blob remains
	db.Model(&RvBlob{}).Count(&count)
	if count != 1 {
		t.Fatalf("Expected 1 blob after cleanup, got %d", count)
	}

	// Verify the remaining blob is the non-expired one
	var rvBlob RvBlob
	if err := db.First(&rvBlob).Error; err != nil {
		t.Fatalf("Failed to retrieve remaining blob: %v", err)
	}
	expectedGUID := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	for i := range expectedGUID {
		if rvBlob.GUID[i] != expectedGUID[i] {
			t.Fatalf("Remaining blob has wrong GUID")
		}
	}
}
