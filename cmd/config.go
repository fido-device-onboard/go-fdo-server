// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
)

// Log configuration
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Configuration for the server's HTTP endpoint
type HTTPConfig struct {
	CertPath string `mapstructure:"cert"`
	KeyPath  string `mapstructure:"key"`
	IP       string `mapstructure:"ip"`
	Port     string `mapstructure:"port"`
}

// Device Certificate Authority
type DeviceCAConfig struct {
	CertPath string `mapstructure:"cert"` // path to certificate file
	KeyPath  string `mapstructure:"key"`  // path to key file
}

func (o *DeviceCAConfig) getDeviceCACertsAsPEM() (string, error) {
	slog.Debug("Reading device CA certificate file", "path", o.CertPath)
	if o.CertPath == "" {
		slog.Warn("Device CA certificate path is empty")
		return "", errors.New("device CA certificate path is empty")
	}
	deviceCA, err := os.ReadFile(o.CertPath)
	if err != nil {
		slog.Error("Failed to read device CA certificate file", "path", o.CertPath, "err", err)
		return "", fmt.Errorf("failed to read device CA cert from %s: %w", o.CertPath, err)
	}
	slog.Debug("Device CA certificate file read successfully", "path", o.CertPath, "size", len(deviceCA))
	return string(deviceCA), nil
}

// Structure to hold the common contents of the configuration file
type FDOServerConfig struct {
	Log  LogConfig      `mapstructure:"log"`
	DB   DatabaseConfig `mapstructure:"db"`
	HTTP HTTPConfig     `mapstructure:"http"`
}

// ListenAddress returns the concatenated IP:Port address for listening
func (h *HTTPConfig) ListenAddress() string {
	return h.IP + ":" + h.Port
}

// UseTLS returns true if TLS should be used (cert and key are both set)
func (h *HTTPConfig) UseTLS() bool {
	return h.CertPath != "" && h.KeyPath != ""
}

func (h *HTTPConfig) validate() error {
	slog.Debug("Validating HTTP configuration", "ip", h.IP, "port", h.Port, "tls", h.UseTLS())
	if h.IP == "" {
		slog.Error("HTTP IP address is required but not provided")
		return errors.New("the server's HTTP IP address is required")
	}
	if h.Port == "" {
		slog.Error("HTTP port is required but not provided")
		return errors.New("the server's HTTP port is required")
	}
	// Both cert and key must be set together or both must be unset
	if (h.CertPath == "" && h.KeyPath != "") || (h.CertPath != "" && h.KeyPath == "") {
		slog.Error("Invalid TLS configuration: cert and key must both be provided or both be empty",
			"cert", h.CertPath, "key", h.KeyPath)
		return errors.New("both certificate and key must be provided together, or neither")
	}
	if h.UseTLS() {
		slog.Debug("TLS enabled for HTTP server", "cert", h.CertPath, "key", h.KeyPath)
	} else {
		slog.Debug("TLS not enabled for HTTP server")
	}
	return nil
}

// Database configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

func (dc *DatabaseConfig) getState() (*db.State, error) {
	slog.Debug("Initializing database state", "type", dc.Type, "dsn", dc.DSN)
	if dc.DSN == "" {
		slog.Error("Database DSN is required but not provided")
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	dc.Type = strings.ToLower(dc.Type)
	slog.Debug("Validating database type", "type", dc.Type)
	if dc.Type != "sqlite" && dc.Type != "postgres" {
		slog.Error("Unsupported database type", "type", dc.Type, "supported", []string{"sqlite", "postgres"})
		return nil, fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dc.Type)
	}

	slog.Debug("Calling db.InitDb", "type", dc.Type)
	state, err := db.InitDb(dc.Type, dc.DSN)
	if err != nil {
		slog.Error("Failed to initialize database", "type", dc.Type, "dsn", dc.DSN, "err", err)
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Debug("Database state initialized successfully", "type", dc.Type)
	return state, nil
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
