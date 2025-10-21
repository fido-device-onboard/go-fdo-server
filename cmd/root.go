// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hermannm.dev/devlog"
)

var (
	debug    bool
	logLevel slog.LevelVar
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			return fmt.Errorf("failed to get config flag: %w", err)
		}
		if configFilePath != "" {
			slog.Debug("Loading server configuration file", "path", configFilePath)
			viper.SetConfigFile(configFilePath)
			if err := viper.ReadInConfig(); err != nil {
				return fmt.Errorf("configuration file read failed: %w", err)
			}
		}
		debug = viper.GetBool("debug")
		if debug {
			logLevel.Set(slog.LevelDebug)
		}
		return nil
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

// Setup the root command line. Used by the unit tests to reset state between tests.
func rootCmdInit() {
	rootCmd.PersistentFlags().String("config", "", "Pathname of the configuration file")
	rootCmd.PersistentFlags().Bool("debug", false, "Print debug contents")
	if err := viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug")); err != nil {
		panic(err)
	}
}

func init() {
	slog.SetDefault(slog.New(devlog.NewHandler(os.Stdout, &devlog.Options{
		Level: &logLevel,
	})))
	rootCmdInit()
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
