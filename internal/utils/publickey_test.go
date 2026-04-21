// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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
