// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"encoding/hex"
	"encoding/pem"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/testdata"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoucherTestDB(t *testing.T) (*gorm.DB, *VoucherPersistentState) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	state, err := InitVoucherDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize voucher state: %v", err)
	}

	return db, state
}

func createTestVoucher(t *testing.T, guidSuffix byte, deviceInfo string) *fdo.Voucher {
	t.Helper()

	// Load base voucher from testdata
	voucherPEM, err := testdata.Files.ReadFile("ov.pem")
	if err != nil {
		t.Fatalf("Failed to read test voucher: %v", err)
	}

	// Parse it
	var voucher fdo.Voucher
	block, _ := pem.Decode(voucherPEM)
	if block == nil {
		t.Fatal("Failed to decode PEM")
	}
	if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
		t.Fatalf("Failed to unmarshal voucher: %v", err)
	}

	// Modify GUID and device info
	voucher.Header.Val.GUID[15] = guidSuffix
	voucher.Header.Val.DeviceInfo = deviceInfo

	return &voucher
}

func TestVoucherState_ListVouchers_Empty(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	vouchers, total, err := state.ListVouchers(context.Background(), 10, 0, nil, nil, nil, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers failed: %v", err)
	}

	if len(vouchers) != 0 {
		t.Errorf("Expected 0 vouchers, got %d", len(vouchers))
	}
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestVoucherState_ListVouchers_Pagination(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add 5 test vouchers
	for i := byte(1); i <= 5; i++ {
		voucher := createTestVoucher(t, i, "Device"+string(rune('A'+i-1)))
		if err := state.AddVoucher(context.Background(), voucher); err != nil {
			t.Fatalf("Failed to add voucher %d: %v", i, err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	tests := []struct {
		name          string
		limit         int
		offset        int
		expectedCount int
		expectedTotal int64
		expectedFirst byte // suffix of first GUID
	}{
		{"First page, limit 2", 2, 0, 2, 5, 1},
		{"Second page, limit 2", 2, 2, 2, 5, 3},
		{"Third page, limit 2", 2, 4, 1, 5, 5},
		{"All results", 10, 0, 5, 5, 1},
		{"Offset beyond results", 10, 10, 0, 5, 0},
		{"Large limit", 100, 0, 5, 5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vouchers, total, err := state.ListVouchers(context.Background(), tt.limit, tt.offset, nil, nil, nil, "created_at", "asc")
			if err != nil {
				t.Fatalf("ListVouchers failed: %v", err)
			}

			if len(vouchers) != tt.expectedCount {
				t.Errorf("Expected %d vouchers, got %d", tt.expectedCount, len(vouchers))
			}
			if total != tt.expectedTotal {
				t.Errorf("Expected total %d, got %d", tt.expectedTotal, total)
			}
			if tt.expectedFirst > 0 && len(vouchers) > 0 {
				if vouchers[0].GUID[15] != tt.expectedFirst {
					t.Errorf("Expected first voucher GUID suffix %d, got %d", tt.expectedFirst, vouchers[0].GUID[15])
				}
			}
		})
	}
}

func TestVoucherState_ListVouchers_FilterByGuid(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add test vouchers
	voucher1 := createTestVoucher(t, 1, "Device1")
	voucher2 := createTestVoucher(t, 2, "Device2")
	if err := state.AddVoucher(context.Background(), voucher1); err != nil {
		t.Fatalf("Failed to add voucher 1: %v", err)
	}
	if err := state.AddVoucher(context.Background(), voucher2); err != nil {
		t.Fatalf("Failed to add voucher 2: %v", err)
	}

	// Filter by specific GUID
	guidHex := hex.EncodeToString(voucher1.Header.Val.GUID[:])
	vouchers, total, err := state.ListVouchers(context.Background(), 10, 0, &guidHex, nil, nil, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers failed: %v", err)
	}

	if len(vouchers) != 1 {
		t.Errorf("Expected 1 voucher, got %d", len(vouchers))
	}
	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(vouchers) > 0 {
		if hex.EncodeToString(vouchers[0].GUID) != guidHex {
			t.Errorf("Expected GUID %s, got %s", guidHex, hex.EncodeToString(vouchers[0].GUID))
		}
	}
}

func TestVoucherState_ListVouchers_FilterByDeviceInfo(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add test vouchers
	voucher1 := createTestVoucher(t, 1, "Device-Alpha")
	voucher2 := createTestVoucher(t, 2, "Device-Beta")
	voucher3 := createTestVoucher(t, 3, "Device-Alpha")
	if err := state.AddVoucher(context.Background(), voucher1); err != nil {
		t.Fatalf("Failed to add voucher 1: %v", err)
	}
	if err := state.AddVoucher(context.Background(), voucher2); err != nil {
		t.Fatalf("Failed to add voucher 2: %v", err)
	}
	if err := state.AddVoucher(context.Background(), voucher3); err != nil {
		t.Fatalf("Failed to add voucher 3: %v", err)
	}

	// Filter by device info
	deviceInfo := "Device-Alpha"
	vouchers, total, err := state.ListVouchers(context.Background(), 10, 0, nil, &deviceInfo, nil, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers failed: %v", err)
	}

	if len(vouchers) != 2 {
		t.Errorf("Expected 2 vouchers, got %d", len(vouchers))
	}
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
}

func TestVoucherState_ListVouchers_Search(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add test vouchers with distinct device info
	voucher1 := createTestVoucher(t, 1, "Raspberry-Pi-4")
	voucher2 := createTestVoucher(t, 2, "Arduino-Uno")
	voucher3 := createTestVoucher(t, 3, "Raspberry-Pi-Zero")
	if err := state.AddVoucher(context.Background(), voucher1); err != nil {
		t.Fatalf("Failed to add voucher 1: %v", err)
	}
	if err := state.AddVoucher(context.Background(), voucher2); err != nil {
		t.Fatalf("Failed to add voucher 2: %v", err)
	}
	if err := state.AddVoucher(context.Background(), voucher3); err != nil {
		t.Fatalf("Failed to add voucher 3: %v", err)
	}

	// Search by device info pattern
	search := "Raspberry"
	vouchers, total, err := state.ListVouchers(context.Background(), 10, 0, nil, nil, &search, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers with search failed: %v", err)
	}

	if len(vouchers) != 2 {
		t.Errorf("Expected 2 vouchers matching 'Raspberry', got %d", len(vouchers))
	}
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}

	// Search should also work with GUID (hex)
	guidHex := hex.EncodeToString(voucher2.Header.Val.GUID[:])
	searchGuid := guidHex[:8] // Search by first 8 chars of GUID
	vouchers, total, err = state.ListVouchers(context.Background(), 10, 0, nil, nil, &searchGuid, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers with GUID search failed: %v", err)
	}

	if len(vouchers) < 1 {
		t.Errorf("Expected at least 1 voucher matching GUID prefix, got %d", len(vouchers))
	}
}

func TestVoucherState_ListVouchers_Sorting(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add vouchers with known order
	voucher1 := createTestVoucher(t, 1, "Zeta")
	voucher2 := createTestVoucher(t, 2, "Alpha")
	voucher3 := createTestVoucher(t, 3, "Gamma")

	// Add in specific order with delays to ensure different timestamps
	if err := state.AddVoucher(context.Background(), voucher2); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := state.AddVoucher(context.Background(), voucher3); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := state.AddVoucher(context.Background(), voucher1); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}

	tests := []struct {
		name          string
		sortBy        string
		sortOrder     string
		expectedFirst byte // GUID suffix of first result
	}{
		{"Sort by created_at asc", "created_at", "asc", 2},
		{"Sort by created_at desc", "created_at", "desc", 1},
		{"Sort by device_info asc", "device_info", "asc", 2},   // Alpha
		{"Sort by device_info desc", "device_info", "desc", 1}, // Zeta
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vouchers, _, err := state.ListVouchers(context.Background(), 10, 0, nil, nil, nil, tt.sortBy, tt.sortOrder)
			if err != nil {
				t.Fatalf("ListVouchers failed: %v", err)
			}

			if len(vouchers) != 3 {
				t.Fatalf("Expected 3 vouchers, got %d", len(vouchers))
			}

			if vouchers[0].GUID[15] != tt.expectedFirst {
				t.Errorf("Expected first voucher GUID suffix %d, got %d", tt.expectedFirst, vouchers[0].GUID[15])
			}
		})
	}
}

func TestVoucherState_ListVouchers_DefaultSorting(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add vouchers
	voucher1 := createTestVoucher(t, 1, "Device1")
	voucher2 := createTestVoucher(t, 2, "Device2")
	if err := state.AddVoucher(context.Background(), voucher1); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := state.AddVoucher(context.Background(), voucher2); err != nil {
		t.Fatalf("Failed to add voucher: %v", err)
	}

	// Test default sorting (empty strings should default to created_at asc)
	vouchers, _, err := state.ListVouchers(context.Background(), 10, 0, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("ListVouchers with default sort failed: %v", err)
	}

	if len(vouchers) != 2 {
		t.Fatalf("Expected 2 vouchers, got %d", len(vouchers))
	}

	// First should be the first added (GUID suffix 1)
	if vouchers[0].GUID[15] != 1 {
		t.Errorf("Expected first voucher GUID suffix 1 with default sorting, got %d", vouchers[0].GUID[15])
	}
}

func TestVoucherState_ListVouchers_CombinedFilters(t *testing.T) {
	_, state := setupVoucherTestDB(t)

	// Add various vouchers
	for i := byte(1); i <= 10; i++ {
		deviceInfo := "Device-Type-A"
		if i > 5 {
			deviceInfo = "Device-Type-B"
		}
		voucher := createTestVoucher(t, i, deviceInfo)
		if err := state.AddVoucher(context.Background(), voucher); err != nil {
			t.Fatalf("Failed to add voucher %d: %v", i, err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Filter by device info and use pagination
	deviceInfo := "Device-Type-A"
	vouchers, total, err := state.ListVouchers(context.Background(), 3, 0, nil, &deviceInfo, nil, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListVouchers with combined filters failed: %v", err)
	}

	if len(vouchers) != 3 {
		t.Errorf("Expected 3 vouchers (limited), got %d", len(vouchers))
	}
	if total != 5 {
		t.Errorf("Expected total 5 (matching filter), got %d", total)
	}
}
