// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/fido-device-onboard/go-fdo/sqlite"
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

const (
	minPasswordLength = 8
)

// Database configuration
type DatabaseConfig struct {
	Path     string `mapstructure:"path"`
	Password string `mapstructure:"password"`
}

// Add command line and configuration file parameters to viper/cobra
// for a database
func addDatabaseConfig(cmd *cobra.Command, configPrefix string) error {
	cmd.Flags().String("db", "", "SQLite database file path")
	cmd.Flags().String("db-pass", "", "SQLite database encryption-at-rest passphrase")
	if err := viper.BindPFlag(configPrefix+".path", cmd.Flags().Lookup("db")); err != nil {
		return err
	}
	if err := viper.BindPFlag(configPrefix+".password", cmd.Flags().Lookup("db-pass")); err != nil {
		return err
	}
	return nil
}

func (db *DatabaseConfig) getState() (*sqlite.DB, error) {
	if db.Path == "" {
		return nil, errors.New("missing required path to the database (--db)")
	}
	// Check password length
	if len(db.Password) < minPasswordLength {
		return nil, fmt.Errorf("password must be at least %d characters long", minPasswordLength)
	}

	// Check password complexity
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString
	hasSpecial := regexp.MustCompile(`[!@#~$%^&*()_+{}:"<>?]`).MatchString

	if !hasNumber(db.Password) || !hasUpper(db.Password) || !hasSpecial(db.Password) {
		return nil, errors.New("password must include a number, an uppercase letter, and a special character")
	}

	return sqlite.Open(db.Path, db.Password)
}
