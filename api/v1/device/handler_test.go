// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package device

import (
	"context"
	"encoding/hex"
	"encoding/pem"
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

	if err := database.AutoMigrate(&state.Voucher{}, &state.DeviceOnboarding{}); err != nil {
		t.Fatalf("Failed to migrate schema: %v", err)
	}

	return &state.VoucherPersistentState{DB: database}
}

func insertTestVoucher(t *testing.T, voucherState *state.VoucherPersistentState) string {
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

	voucherBytes, err := cbor.Marshal(&v)
	if err != nil {
		t.Fatalf("Failed to marshal voucher: %v", err)
	}

	guid := v.Header.Val.GUID
	dbVoucher := state.Voucher{
		GUID:       guid[:],
		CBOR:       voucherBytes,
		DeviceInfo: v.Header.Val.DeviceInfo,
	}
	if err := voucherState.DB.WithContext(ctx).Create(&dbVoucher).Error; err != nil {
		t.Fatalf("Failed to insert voucher: %v", err)
	}

	return hex.EncodeToString(guid[:])
}

func TestListDevices_Empty(t *testing.T) {
	voucherState := setupTestDB(t)
	server := NewServer(voucherState)
	ctx := context.Background()

	request := ListDevicesRequestObject{}
	response, err := server.ListDevices(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListDevices200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200, got %T", response)
	}
	if len(resp200) != 0 {
		t.Errorf("Expected empty list, got %d devices", len(resp200))
	}
}

func TestListDevices_WithVoucher(t *testing.T) {
	voucherState := setupTestDB(t)
	guidHex := insertTestVoucher(t, voucherState)
	server := NewServer(voucherState)
	ctx := context.Background()

	request := ListDevicesRequestObject{}
	response, err := server.ListDevices(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListDevices200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200, got %T", response)
	}
	if len(resp200) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(resp200))
	}
	if resp200[0].Guid != guidHex {
		t.Errorf("Expected GUID %s, got %s", guidHex, resp200[0].Guid)
	}
	if resp200[0].To2Completed {
		t.Error("Expected TO2 not completed")
	}
}

func TestListDevices_InvalidGUIDFilter(t *testing.T) {
	voucherState := setupTestDB(t)
	server := NewServer(voucherState)
	ctx := context.Background()

	invalidGUID := "xyz"
	request := ListDevicesRequestObject{
		Params: ListDevicesParams{
			OldGuid: &invalidGUID,
		},
	}
	response, err := server.ListDevices(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if _, ok := response.(ListDevices400TextResponse); !ok {
		t.Fatalf("Expected 400, got %T", response)
	}
}

func TestListDevices_ValidGUIDFilterNoMatch(t *testing.T) {
	voucherState := setupTestDB(t)
	insertTestVoucher(t, voucherState)
	server := NewServer(voucherState)
	ctx := context.Background()

	// A valid GUID that doesn't match any device
	noMatchGUID := "00000000000000000000000000000000"
	request := ListDevicesRequestObject{
		Params: ListDevicesParams{
			OldGuid: &noMatchGUID,
		},
	}
	response, err := server.ListDevices(ctx, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resp200, ok := response.(ListDevices200JSONResponse)
	if !ok {
		t.Fatalf("Expected 200, got %T", response)
	}
	if len(resp200) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(resp200))
	}
}
