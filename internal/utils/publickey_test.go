// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

func TestEncodePublicKey_ECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	pk, err := EncodePublicKey(protocol.Secp256r1KeyType, protocol.X509KeyEnc, key.Public(), nil)
	if err != nil {
		t.Fatalf("EncodePublicKey failed: %v", err)
	}
	if pk == nil {
		t.Fatal("Expected non-nil PublicKey")
	}
}

func TestEncodePublicKey_ECDSA384(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	pk, err := EncodePublicKey(protocol.Secp384r1KeyType, protocol.X509KeyEnc, key.Public(), nil)
	if err != nil {
		t.Fatalf("EncodePublicKey failed: %v", err)
	}
	if pk == nil {
		t.Fatal("Expected non-nil PublicKey")
	}
}

func TestEncodePublicKey_RSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	pk, err := EncodePublicKey(protocol.Rsa2048RestrKeyType, protocol.X509KeyEnc, key.Public(), nil)
	if err != nil {
		t.Fatalf("EncodePublicKey failed: %v", err)
	}
	if pk == nil {
		t.Fatal("Expected non-nil PublicKey")
	}
}

func TestEncodePublicKey_ECDSAKeyType_WithRSAKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Passing an RSA key with an ECDSA key type should return an error, not panic
	_, err = EncodePublicKey(protocol.Secp256r1KeyType, protocol.X509KeyEnc, rsaKey.Public(), nil)
	if err == nil {
		t.Fatal("Expected error for mismatched key type, got nil")
	}
}

func TestEncodePublicKey_RSAKeyType_WithECDSAKey(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	// Passing an ECDSA key with an RSA key type should return an error, not panic
	_, err = EncodePublicKey(protocol.Rsa2048RestrKeyType, protocol.X509KeyEnc, ecKey.Public(), nil)
	if err == nil {
		t.Fatal("Expected error for mismatched key type, got nil")
	}
}

func TestEncodePublicKey_UnsupportedKeyType(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	_, err = EncodePublicKey(protocol.KeyType(255), protocol.X509KeyEnc, key.Public(), nil)
	if err == nil {
		t.Fatal("Expected error for unsupported key type, got nil")
	}
}

func TestEncodePublicKey_UnsupportedKeyEncoding(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	_, err = EncodePublicKey(protocol.Secp256r1KeyType, protocol.KeyEncoding(255), key.Public(), nil)
	if err == nil {
		t.Fatal("Expected error for unsupported key encoding, got nil")
	}
}

func TestEncodePublicKey_NilKey(t *testing.T) {
	_, err := EncodePublicKey(protocol.Secp256r1KeyType, protocol.X509KeyEnc, nil, nil)
	if err == nil {
		t.Fatal("Expected error for nil key, got nil")
	}
}

func TestEncodePublicKey_UnsupportedPublicKeyType(t *testing.T) {
	// ed25519 is not supported by FDO — should return an error for mismatched type
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ed25519 key: %v", err)
	}

	_, err = EncodePublicKey(protocol.Secp256r1KeyType, protocol.X509KeyEnc, edKey, nil)
	if err == nil {
		t.Fatal("Expected error for ed25519 key with ECDSA key type, got nil")
	}
}

func TestPublicKeysEqual(t *testing.T) {
	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key1: %v", err)
	}

	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key2: %v", err)
	}

	if !PublicKeysEqual(key1.Public(), key1.Public()) {
		t.Error("expected same key to be equal to itself")
	}

	if PublicKeysEqual(key1.Public(), key2.Public()) {
		t.Error("expected different keys to not be equal")
	}
}

func TestPublicKeysEqual_ReconstructedKeys(t *testing.T) {
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

			result := PublicKeysEqual(a, b)
			if result != tt.shouldEqual {
				t.Errorf("PublicKeysEqual() = %v, want %v for %s keys", result, tt.shouldEqual, tt.keyType)

				oldResult := fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
				t.Logf("Old broken string comparison would give: %v (demonstrates the bug)", oldResult)
			}
		})
	}
}

func TestPublicKeysEqual_UnsupportedType(t *testing.T) {
	type customKey struct {
		data []byte
	}

	key1 := customKey{data: []byte{1, 2, 3}}
	key2 := customKey{data: []byte{1, 2, 3}}

	result := PublicKeysEqual(key1, key2)
	if result {
		t.Error("Expected false for unsupported key type, got true")
	}
}
