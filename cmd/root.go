// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
	"hermannm.dev/devlog"
)

var (
	dbPath         string
	dbPass         string
	debug          bool
	logLevel       slog.LevelVar
	insecureTLS    bool
	serverCertPath string
	serverKeyPath  string
)

var rootCmd = &cobra.Command{
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	Use:   "go-fdo-server",
	Short: "Server implementation of FIDO Device Onboard specification in Go",
	Long: `Server implementation of the three main FDO servers. It can act
	as a Manufacturer, Owner and Rendezvous.

	The server also provides APIs to interact with the various servers implementations.
`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug {
			logLevel.Set(slog.LevelDebug)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	slog.SetDefault(slog.New(devlog.NewHandler(os.Stdout, &devlog.Options{
		Level: &logLevel,
	})))

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Print debug contents")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "SQLite database file path")
	rootCmd.PersistentFlags().StringVar(&dbPass, "db-pass", "", "SQLite database encryption-at-rest passphrase")
	rootCmd.MarkPersistentFlagRequired("db")
	rootCmd.MarkPersistentFlagRequired("db-pass")
	rootCmd.PersistentFlags().BoolVar(&insecureTLS, "insecure-tls", false, "Listen with a self-signed TLS certificate")
	rootCmd.PersistentFlags().StringVar(&serverCertPath, "server-cert-path", "", "Path to server certificate")
	rootCmd.PersistentFlags().StringVar(&serverKeyPath, "server-key-path", "", "Path to server private key")
}

const (
	minPasswordLength = 8
)

func getState() (*sqlite.DB, error) {
	if dbPath == "" {
		return nil, errors.New("db flag is required")
	}

	if dbPass == "" {
		return nil, errors.New("db password is empty")
	}

	err := validatePassword(dbPass)
	if err != nil {
		return nil, err
	}

	return sqlite.Open(dbPath, dbPass)
}

func validatePassword(dbPass string) error {
	// Check password length
	if len(dbPass) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", minPasswordLength)
	}

	// Check password complexity
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString
	hasSpecial := regexp.MustCompile(`[!@#~$%^&*()_+{}:"<>?]`).MatchString

	if !hasNumber(dbPass) || !hasUpper(dbPass) || !hasSpecial(dbPass) {
		return errors.New("password must include a number, an uppercase letter, and a special character")
	}

	return nil
}

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
