// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package owner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
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

func TestPublicKeysEqual(t *testing.T) {
	// Test with ECDSA keys
	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key1: %v", err)
	}

	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key2: %v", err)
	}

	// Same key should equal itself
	if !publicKeysEqual(key1.Public(), key1.Public()) {
		t.Error("expected same key to be equal to itself")
	}

	// Different keys should not be equal
	if publicKeysEqual(key1.Public(), key2.Public()) {
		t.Error("expected different keys to not be equal")
	}
}

// TestPublicKeysEqualReconstructedKeys verifies that keys reconstructed from
// serialization are properly compared. This test demonstrates the fix for the
// unreliable fmt.Sprintf fallback that compared pointer addresses.
func TestPublicKeysEqualReconstructedKeys(t *testing.T) {
	tests := []struct {
		name        string
		keyType     string
		generateKey func() (interface{}, interface{})
		shouldEqual bool
	}{
		{
			name:    "RSA keys - reconstructed from same values",
			keyType: "RSA",
			generateKey: func() (interface{}, interface{}) {
				key, _ := rsa.GenerateKey(rand.Reader, 2048)
				original := key.Public().(*rsa.PublicKey)
				// Simulate key reconstruction (e.g., from database or network)
				reconstructed := &rsa.PublicKey{
					N: original.N,
					E: original.E,
				}
				return original, reconstructed
			},
			shouldEqual: true,
		},
		{
			name:    "ECDSA keys - same instance",
			keyType: "ECDSA",
			generateKey: func() (interface{}, interface{}) {
				key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				pub := key.Public()
				// Same instance should equal itself
				return pub, pub
			},
			shouldEqual: true,
		},
		{
			name:    "RSA keys - different key values",
			keyType: "RSA",
			generateKey: func() (interface{}, interface{}) {
				key1, _ := rsa.GenerateKey(rand.Reader, 2048)
				key2, _ := rsa.GenerateKey(rand.Reader, 2048)
				return key1.Public(), key2.Public()
			},
			shouldEqual: false,
		},
		{
			name:    "ECDSA keys - different key values",
			keyType: "ECDSA",
			generateKey: func() (interface{}, interface{}) {
				key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				return key1.Public(), key2.Public()
			},
			shouldEqual: false,
		},
		{
			name:    "RSA vs ECDSA - different types",
			keyType: "Mixed",
			generateKey: func() (interface{}, interface{}) {
				rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
				ecdsaKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				return rsaKey.Public(), ecdsaKey.Public()
			},
			shouldEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := tt.generateKey()

			result := publicKeysEqual(a, b)
			if result != tt.shouldEqual {
				t.Errorf("publicKeysEqual() = %v, want %v for %s keys", result, tt.shouldEqual, tt.keyType)

				// Additional diagnostic: show what the old broken implementation would do
				// (This demonstrates why fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) is unreliable)
				oldResult := fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
				t.Logf("Old broken string comparison would give: %v (demonstrates the bug)", oldResult)
			}
		})
	}
}

// TestPublicKeysEqualUnsupportedType verifies that unsupported key types
// return false rather than causing a panic or giving false positives
func TestPublicKeysEqualUnsupportedType(t *testing.T) {
	// Create a custom type that doesn't implement Equal
	type customKey struct {
		data []byte
	}

	key1 := customKey{data: []byte{1, 2, 3}}
	key2 := customKey{data: []byte{1, 2, 3}}

	// Even though the values are the same, we should return false for unsupported types
	// This is safer than the old fmt.Sprintf approach which could give false positives
	result := publicKeysEqual(key1, key2)
	if result {
		t.Error("Expected false for unsupported key type, got true")
	}
}
