package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

func parsePrivateKey(keyPath string) (crypto.Signer, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(b)
	if err == nil {
		return key.(crypto.Signer), nil
	}
	if strings.Contains(err.Error(), "ParseECPrivateKey") {
		key, err = x509.ParseECPrivateKey(b)
		if err != nil {
			return nil, err
		}
		return key.(crypto.Signer), nil
	}
	if strings.Contains(err.Error(), "ParsePKCS1PrivateKey") {
		key, err = x509.ParsePKCS1PrivateKey(b)
		if err != nil {
			return nil, err
		}
		return key.(crypto.Signer), nil
	}
	return nil, fmt.Errorf("unable to parse private key %s: %v", keyPath, err)
}

func getPrivateKeyType(key any) (protocol.KeyType, error) {
	switch ktype := key.(type) {
	case *rsa.PrivateKey:
		switch ktype.N.BitLen() {
		case 2048:
			return protocol.Rsa2048RestrKeyType, nil
			// case 3072: TODO: add support for 3072 bit keys
		}
	case *ecdsa.PrivateKey:
		switch ktype.Curve.Params().BitSize {
		case 256:
			return protocol.Secp256r1KeyType, nil
		case 384:
			return protocol.Secp384r1KeyType, nil
		}
	}
	return 0, fmt.Errorf("unsupported key provided")
}

// parseHTTPAddress parses an address string in the format "host:port" and returns
// the host and port components. Supports IPv4, IPv6 addresses, and DNS names.
// Returns an error if the format is invalid.
func parseHTTPAddress(addr string) (ip, port string, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("invalid address format: %w", err)
	}
	if host == "" {
		return "", "", fmt.Errorf("invalid address format: host cannot be empty")
	}
	if portStr == "" {
		return "", "", fmt.Errorf("invalid address format: port cannot be empty")
	}
	return host, portStr, nil
}

// loadCertificateFromFile reads a PEM-encoded certificate from a file and returns it as []*x509.Certificate
func loadCertificateFromFile(filePath string) ([]*x509.Certificate, error) {
	slog.Debug("Loading certificate from file", "path", filePath)
	if filePath == "" {
		slog.Debug("Certificate file path is empty, skipping")
		return nil, nil
	}
	certData, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("Failed to read certificate file", "path", filePath, "err", err)
		return nil, fmt.Errorf("failed to read certificate from %s: %w", filePath, err)
	}
	slog.Debug("Certificate file read successfully", "path", filePath, "size", len(certData))

	blk, _ := pem.Decode(certData)
	if blk == nil {
		slog.Error("Failed to decode PEM certificate", "path", filePath)
		return nil, fmt.Errorf("unable to decode PEM certificate from %s", filePath)
	}
	slog.Debug("PEM block decoded successfully", "type", blk.Type, "size", len(blk.Bytes))

	parsedCert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		slog.Error("Failed to parse X.509 certificate", "path", filePath, "err", err)
		return nil, fmt.Errorf("unable to parse certificate from %s: %w", filePath, err)
	}
	slog.Info("Certificate loaded successfully", "path", filePath, "subject", parsedCert.Subject.String(), "issuer", parsedCert.Issuer.String())

	return []*x509.Certificate{parsedCert}, nil
}
