// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeKeyFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}
	return path
}

func TestParsePrivateKey_PKCS8_PEM_ECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal PKCS8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	path := writeKeyFile(t, t.TempDir(), "key.pem", pemBytes)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("Expected *ecdsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_PKCS8_PEM_RSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal PKCS8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	path := writeKeyFile(t, t.TempDir(), "key.pem", pemBytes)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		t.Fatalf("Expected *rsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_SEC1_PEM(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal EC key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	path := writeKeyFile(t, t.TempDir(), "key.pem", pemBytes)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("Expected *ecdsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_PKCS1_PEM(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})

	path := writeKeyFile(t, t.TempDir(), "key.pem", pemBytes)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		t.Fatalf("Expected *rsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_PKCS8_DER(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal PKCS8: %v", err)
	}

	path := writeKeyFile(t, t.TempDir(), "key.der", der)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("Expected *ecdsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_SEC1_DER(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal EC key: %v", err)
	}

	path := writeKeyFile(t, t.TempDir(), "key.der", der)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("Expected *ecdsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_PKCS1_DER(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)

	path := writeKeyFile(t, t.TempDir(), "key.der", der)
	signer, err := parsePrivateKey(path)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		t.Fatalf("Expected *rsa.PrivateKey, got %T", signer)
	}
}

func TestParsePrivateKey_InvalidData(t *testing.T) {
	path := writeKeyFile(t, t.TempDir(), "garbage.pem", []byte("not a key"))
	_, err := parsePrivateKey(path)
	if err == nil {
		t.Fatal("Expected error for invalid key data, got nil")
	}
}

func TestParsePrivateKey_InvalidPEMContent(t *testing.T) {
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{0xFF, 0xFF}})
	path := writeKeyFile(t, t.TempDir(), "bad.pem", pemBytes)
	_, err := parsePrivateKey(path)
	if err == nil {
		t.Fatal("Expected error for invalid PEM content, got nil")
	}
}

func TestParsePrivateKey_NonExistentFile(t *testing.T) {
	_, err := parsePrivateKey("/nonexistent/path/key.pem")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}
