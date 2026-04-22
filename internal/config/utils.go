package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

func parsePrivateKey(keyPath string) (crypto.Signer, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Decode PEM if present, otherwise use raw DER bytes
	derBytes := data
	if blk, _ := pem.Decode(data); blk != nil {
		derBytes = blk.Bytes
	}

	// Try PKCS#8 (most common modern format, supports both RSA and ECDSA)
	if key, err := x509.ParsePKCS8PrivateKey(derBytes); err == nil {
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("parsed PKCS#8 key from %s does not implement crypto.Signer", keyPath)
		}
		return signer, nil
	}

	// Try SEC1 / RFC 5915 (EC private key)
	if key, err := x509.ParseECPrivateKey(derBytes); err == nil {
		return key, nil
	}

	// Try PKCS#1 (RSA private key)
	if key, err := x509.ParsePKCS1PrivateKey(derBytes); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("unable to parse private key %s: unsupported format", keyPath)
}

func getPrivateKeyType(key any) (protocol.KeyType, error) {
	switch ktype := key.(type) {
	case *rsa.PrivateKey:
		switch ktype.N.BitLen() {
		case 2048:
			return protocol.Rsa2048RestrKeyType, nil
		case 3072:
			return protocol.RsaPkcsKeyType, nil
		default:
			return 0, fmt.Errorf("unsupported RSA key size: %d bits (FDO only supports 2048 and 3072)", ktype.N.BitLen())
		}
	case *ecdsa.PrivateKey:
		switch ktype.Curve.Params().BitSize {
		case 256:
			return protocol.Secp256r1KeyType, nil
		case 384:
			return protocol.Secp384r1KeyType, nil
		default:
			return 0, fmt.Errorf("unsupported ECDSA curve size: %d bits (FDO only supports 256 and 384)", ktype.Curve.Params().BitSize)
		}
	}
	return 0, fmt.Errorf("unsupported key type: %T", key)
}

// loadCertificateFromFile reads PEM-encoded certificate(s) from a file and returns them as []*x509.Certificate.
// Supports both single certificates and certificate bundles (multiple certificates in one file).
func loadCertificateFromFile(filePath string) ([]*x509.Certificate, error) {
	slog.Debug("Loading certificate from file", "path", filePath)
	if filePath == "" {
		slog.Debug("Certificate file path is empty, skipping")
		return nil, nil
	}
	certData, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("Failed to read certificate file", "path", filePath, "error", err)
		return nil, fmt.Errorf("failed to read certificate from %s: %w", filePath, err)
	}
	slog.Debug("Certificate file read successfully", "path", filePath, "size", len(certData))

	var certs []*x509.Certificate
	rest := certData

	// Loop through all PEM blocks in the file to support certificate bundles
	for {
		blk, remaining := pem.Decode(rest)
		if blk == nil {
			break
		}
		slog.Debug("PEM block decoded successfully", "type", blk.Type, "size", len(blk.Bytes))

		parsedCert, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			slog.Error("Failed to parse X.509 certificate", "path", filePath, "error", err)
			return nil, fmt.Errorf("unable to parse certificate from %s: %w", filePath, err)
		}

		certs = append(certs, parsedCert)
		slog.Debug("Certificate parsed successfully", "subject", parsedCert.Subject.String(), "issuer", parsedCert.Issuer.String())

		rest = remaining
	}

	if len(certs) == 0 {
		slog.Error("No certificates found in file", "path", filePath)
		return nil, fmt.Errorf("unable to decode PEM certificate from %s", filePath)
	}

	slog.Info("Certificates loaded successfully", "path", filePath, "count", len(certs))
	return certs, nil
}
