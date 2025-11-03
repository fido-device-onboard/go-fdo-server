// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
)

// Structure to hold contents of the configuration file
type FIDOServerConfig struct {
	Log           LogConfig            `mapstructure:"log"`
	DB            DatabaseConfig       `mapstructure:"db"`
	HTTP          HTTPConfig           `mapstructure:"http"`
	Manufacturing *ManufacturingConfig `mapstructure:"manufacturing"`
	Owner         *OwnerConfig         `mapstructure:"owner"`
}

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

// ListenAddress returns the concatenated IP:Port address for listening
func (h *HTTPConfig) ListenAddress() string {
	return h.IP + ":" + h.Port
}

// UseTLS returns true if TLS should be used (cert and key are both set)
func (h *HTTPConfig) UseTLS() bool {
	return h.CertPath != "" && h.KeyPath != ""
}

func (h *HTTPConfig) validate() error {
	if h.IP == "" {
		return errors.New("the server's HTTP IP address is required")
	}
	if h.Port == "" {
		return errors.New("the server's HTTP port is required")
	}
	// Both cert and key must be set together or both must be unset
	if (h.CertPath == "" && h.KeyPath != "") || (h.CertPath != "" && h.KeyPath == "") {
		return errors.New("both certificate and key must be provided together, or neither")
	}
	return nil
}

// Database configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

func (dc *DatabaseConfig) getState() (*db.State, error) {
	if dc.DSN == "" {
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	dc.Type = strings.ToLower(dc.Type)
	if dc.Type != "sqlite" && dc.Type != "postgres" {
		return nil, fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dc.Type)
	}

	return db.InitDb(dc.Type, dc.DSN)
}
