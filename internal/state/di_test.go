// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"math/big"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDITestDB(t *testing.T) (*gorm.DB, *DISessionState) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Enable foreign key constraints
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	state, err := InitDISessionDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize DI session state: %v", err)
	}

	return db, state
}

// createDISession creates a new DI session and returns a context containing
// the session token.
func createDISession(t *testing.T, state *DISessionState) context.Context {
	t.Helper()

	ctx := context.Background()
	token, err := state.Token.NewToken(ctx, protocol.DIProtocol)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	return state.Token.TokenContext(ctx, token)
}

// generateTestCert creates a self-signed certificate for testing.
func generateTestCert(t *testing.T) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}

func TestDIInitDISessionDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	state, err := InitDISessionDB(db)
	if err != nil {
		t.Fatalf("InitDISessionDB failed: %v", err)
	}

	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if state.Token == nil {
		t.Fatal("Expected non-nil Token service")
	}
	if state.DB == nil {
		t.Fatal("Expected non-nil DB")
	}

	// Verify tables were created by checking that we can query them
	if !db.Migrator().HasTable(&DeviceInfo{}) {
		t.Error("Expected device_info table to exist")
	}
	if !db.Migrator().HasTable(&IncompleteVoucher{}) {
		t.Error("Expected incomplete_vouchers table to exist")
	}
	if !db.Migrator().HasTable(&Session{}) {
		t.Error("Expected sessions table to exist")
	}
}

func TestDISetDeviceCertChain_And_DeviceCertChain(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	cert := generateTestCert(t)
	chain := []*x509.Certificate{cert}

	// Store the certificate chain
	if err := state.SetDeviceCertChain(ctx, chain); err != nil {
		t.Fatalf("SetDeviceCertChain failed: %v", err)
	}

	// Retrieve and verify
	got, err := state.DeviceCertChain(ctx)
	if err != nil {
		t.Fatalf("DeviceCertChain failed: %v", err)
	}

	if len(got) != len(chain) {
		t.Fatalf("Expected %d certs, got %d", len(chain), len(got))
	}

	if !got[0].Equal(cert) {
		t.Error("Retrieved certificate does not match stored certificate")
	}
}

func TestDISetDeviceCertChain_MultipleCerts(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	cert1 := generateTestCert(t)
	cert2 := generateTestCert(t)
	chain := []*x509.Certificate{cert1, cert2}

	if err := state.SetDeviceCertChain(ctx, chain); err != nil {
		t.Fatalf("SetDeviceCertChain failed: %v", err)
	}

	got, err := state.DeviceCertChain(ctx)
	if err != nil {
		t.Fatalf("DeviceCertChain failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("Expected 2 certs, got %d", len(got))
	}

	if !got[0].Equal(cert1) {
		t.Error("First certificate does not match")
	}
	if !got[1].Equal(cert2) {
		t.Error("Second certificate does not match")
	}
}

func TestDISetDeviceCertChain_SecondCallNoError(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	cert1 := generateTestCert(t)
	cert2 := generateTestCert(t)

	// Store first chain
	if err := state.SetDeviceCertChain(ctx, []*x509.Certificate{cert1}); err != nil {
		t.Fatalf("SetDeviceCertChain (first) failed: %v", err)
	}

	// Calling again for the same session should not return an error
	if err := state.SetDeviceCertChain(ctx, []*x509.Certificate{cert2}); err != nil {
		t.Fatalf("SetDeviceCertChain (second call) failed: %v", err)
	}

	// Verify we can still retrieve a certificate chain
	got, err := state.DeviceCertChain(ctx)
	if err != nil {
		t.Fatalf("DeviceCertChain failed: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Expected at least one certificate in the chain")
	}
}

func TestDISetIncompleteVoucherHeader_And_IncompleteVoucherHeader(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	mfgKey, err := protocol.NewPublicKey(protocol.Secp256r1KeyType, &key.PublicKey, false)
	if err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	guid := protocol.GUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	header := &fdo.VoucherHeader{
		Version:         100,
		GUID:            guid,
		RvInfo:          nil,
		DeviceInfo:      "test-device",
		ManufacturerKey: *mfgKey,
	}

	// Store
	if err := state.SetIncompleteVoucherHeader(ctx, header); err != nil {
		t.Fatalf("SetIncompleteVoucherHeader failed: %v", err)
	}

	// Retrieve
	got, err := state.IncompleteVoucherHeader(ctx)
	if err != nil {
		t.Fatalf("IncompleteVoucherHeader failed: %v", err)
	}

	if got.Version != header.Version {
		t.Errorf("Version: expected %d, got %d", header.Version, got.Version)
	}
	if got.GUID != header.GUID {
		t.Errorf("GUID mismatch")
	}
	if got.DeviceInfo != header.DeviceInfo {
		t.Errorf("DeviceInfo: expected %q, got %q", header.DeviceInfo, got.DeviceInfo)
	}
}

func TestDIGetReplacementGUID(t *testing.T) {
	db, state := setupDITestDB(t)

	// Also migrate the ReplacementVoucher table since GetReplacementGUID queries it
	if err := db.AutoMigrate(&ReplacementVoucher{}); err != nil {
		t.Fatalf("Failed to migrate ReplacementVoucher: %v", err)
	}

	ctx := createDISession(t, state)

	// Extract the session ID for inserting test data
	token, ok := state.Token.TokenFromContext(ctx)
	if !ok {
		t.Fatal("Failed to get token from context")
	}

	sessionID, err := state.Token.getSessionID(state.Token.TokenContext(context.Background(), token))
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	expectedGUID := protocol.GUID{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00}

	// Insert a replacement voucher record
	rv := ReplacementVoucher{
		Session: sessionID,
		GUID:    expectedGUID[:],
	}
	if err := db.Create(&rv).Error; err != nil {
		t.Fatalf("Failed to insert replacement voucher: %v", err)
	}

	got, err := state.GetReplacementGUID(ctx)
	if err != nil {
		t.Fatalf("GetReplacementGUID failed: %v", err)
	}

	if got != expectedGUID {
		t.Errorf("Expected GUID %v, got %v", expectedGUID, got)
	}
}

func TestDIGetReplacementGUID_NotFound(t *testing.T) {
	db, state := setupDITestDB(t)

	if err := db.AutoMigrate(&ReplacementVoucher{}); err != nil {
		t.Fatalf("Failed to migrate ReplacementVoucher: %v", err)
	}

	ctx := createDISession(t, state)

	_, err := state.GetReplacementGUID(ctx)
	if err == nil {
		t.Fatal("Expected error for missing replacement voucher, got nil")
	}
	if !errors.Is(err, fdo.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func TestDIDeviceCertChain_NotFound(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	_, err := state.DeviceCertChain(ctx)
	if err == nil {
		t.Fatal("Expected error for missing cert chain, got nil")
	}
	if !errors.Is(err, fdo.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func TestDIIncompleteVoucherHeader_NotFound(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := createDISession(t, state)

	_, err := state.IncompleteVoucherHeader(ctx)
	if err == nil {
		t.Fatal("Expected error for missing voucher header, got nil")
	}
	if !errors.Is(err, fdo.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func TestDIMissingTokenInContext(t *testing.T) {
	_, state := setupDITestDB(t)
	ctx := context.Background() // no token

	t.Run("SetDeviceCertChain", func(t *testing.T) {
		err := state.SetDeviceCertChain(ctx, []*x509.Certificate{generateTestCert(t)})
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("DeviceCertChain", func(t *testing.T) {
		_, err := state.DeviceCertChain(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("SetIncompleteVoucherHeader", func(t *testing.T) {
		err := state.SetIncompleteVoucherHeader(ctx, &fdo.VoucherHeader{})
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("IncompleteVoucherHeader", func(t *testing.T) {
		_, err := state.IncompleteVoucherHeader(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("GetReplacementGUID", func(t *testing.T) {
		_, err := state.GetReplacementGUID(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})
}

func TestDINonExistentSession(t *testing.T) {
	_, state := setupDITestDB(t)

	// Inject a token that does not correspond to any session
	ctx := state.Token.TokenContext(context.Background(), "bm9uc3VjaHNlc3Npb24") // base64url of "nosuchsession"

	t.Run("SetDeviceCertChain", func(t *testing.T) {
		err := state.SetDeviceCertChain(ctx, []*x509.Certificate{generateTestCert(t)})
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("DeviceCertChain", func(t *testing.T) {
		_, err := state.DeviceCertChain(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("SetIncompleteVoucherHeader", func(t *testing.T) {
		err := state.SetIncompleteVoucherHeader(ctx, &fdo.VoucherHeader{})
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("IncompleteVoucherHeader", func(t *testing.T) {
		_, err := state.IncompleteVoucherHeader(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})

	t.Run("GetReplacementGUID", func(t *testing.T) {
		_, err := state.GetReplacementGUID(ctx)
		if !errors.Is(err, fdo.ErrInvalidSession) {
			t.Errorf("Expected ErrInvalidSession, got: %v", err)
		}
	})
}
