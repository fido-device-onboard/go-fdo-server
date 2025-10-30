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
	"strings"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
	"hermannm.dev/devlog"
)

var (
	dbType         string
	dbDSN          string
	debug          bool
	logLevel       slog.LevelVar
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
	rootCmd.PersistentFlags().StringVar(&dbType, "db-type", "sqlite", "Database type (sqlite or postgres)")
	rootCmd.PersistentFlags().StringVar(&dbDSN, "db-dsn", "", "Database DSN (connection string)")
	rootCmd.MarkPersistentFlagRequired("db-dsn")
	rootCmd.PersistentFlags().StringVar(&serverCertPath, "server-cert-path", "", "Path to server certificate")
	rootCmd.PersistentFlags().StringVar(&serverKeyPath, "server-key-path", "", "Path to server private key")
}

// useTLS returns true if both server cert and key paths are provided
func useTLS() bool {
	return serverCertPath != "" && serverKeyPath != ""
}

func getDBConfig() (string, string, error) {
	if dbDSN == "" {
		return "", "", errors.New("db-dsn flag is required")
	}

	// Validate database type
	normalizedType := strings.ToLower(dbType)
	if normalizedType != "sqlite" && normalizedType != "postgres" {
		return "", "", fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dbType)
	}

	return normalizedType, dbDSN, nil
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
