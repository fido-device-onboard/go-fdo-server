// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Structure to hold contents of the configuration file
type FIDOServerConfig struct {
	Debug         bool                 `mapstructure:"debug"`
	Manufacturing *ManufacturingConfig `mapstructure:"manufacturing"`
	Rendezvous    *RendezvousConfig    `mapstructure:"rendezvous"`
	Owner         *OwnerConfig         `mapstructure:"owner"`
}

// Configuration for the servers HTTP endpoing
type HTTPConfig struct {
	UseTLS      bool   `mapstructure:"ssl"`
	InsecureTLS bool   `mapstructure:"insecure-tls"`
	Listen      string `mapstructure:"listen"`
	CertPath    string `mapstructure:"cert"`
	KeyPath     string `mapstructure:"key"`
}

// Add command line and configuration file parameters to viper/cobra
// for an HTTP server.
func addHTTPConfig(cmd *cobra.Command, configPrefix string) error {
	cmd.Flags().Bool("insecure-tls", false, "Listen with a self-signed TLS certificate")
	cmd.Flags().String("server-cert-path", "", "Path to server certificate")
	cmd.Flags().String("server-key-path", "", "Path to server private key")
	if err := viper.BindPFlag(configPrefix+".insecure-tls", cmd.Flags().Lookup("insecure-tls")); err != nil {
		return err
	}
	if err := viper.BindPFlag(configPrefix+".cert", cmd.Flags().Lookup("server-cert-path")); err != nil {
		return err
	}
	if err := viper.BindPFlag(configPrefix+".key", cmd.Flags().Lookup("server-key-path")); err != nil {
		return err
	}
	return nil
}

func (h *HTTPConfig) validate() error {
	if h.Listen == "" {
		return errors.New("the server's HTTP listen address is required")
	}
	if h.UseTLS && (h.CertPath == "" || h.KeyPath == "") {
		return errors.New("TLS requires a server certificate and key")
	}
	return nil
}

// Database configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

// TODO(kgiusti): move database to top level config, fix this:
func addDatabaseConfig(cmd *cobra.Command, configPrefix string) error {
	cmd.Flags().String("db-type", "sqlite", "Database type (sqlite or postgres)")
	cmd.Flags().String("db-dsn", "", "Database DSN (connection string)")
	if err := viper.BindPFlag(configPrefix+".type", cmd.Flags().Lookup("db-type")); err != nil {
		return err
	}
	if err := viper.BindPFlag(configPrefix+".dsn", cmd.Flags().Lookup("db-dsn")); err != nil {
		return err
	}
	return nil
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
