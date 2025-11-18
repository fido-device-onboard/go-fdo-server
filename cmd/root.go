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
	"net"
	"os"
	"strings"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hermannm.dev/devlog"
)

var (
	logLevel          slog.LevelVar
	configSearchPaths = []string{ // searched starting with index 0
		"$HOME/.config/go-fdo-server/",
		"/etc/go-fdo-server/",
		"/usr/share/go-fdo-server/",
	}
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
		// bootstrap debug logging early to include configuration loading
		level, _ := cmd.Flags().GetString("log-level")
		if strings.ToLower(level) == "debug" {
			logLevel.Set(slog.LevelDebug)
		}
		logLevel.Set(slog.LevelDebug)

		configFilePath, err := cmd.Flags().GetString("config")
		if err != nil {
			return fmt.Errorf("failed to get config flag: %w", err)
		}
		if configFilePath != "" {
			slog.Debug("Loading server configuration file", "path", configFilePath)
			viper.SetConfigFile(configFilePath)
			err = viper.ReadInConfig()
			if err != nil {
				return fmt.Errorf("configuration file read failed: %w", err)
			}
		} else {
			// base filename, no suffix e.g. "manufacturing"
			filename := cmd.Name()
			viper.SetConfigName(filename)
			for _, path := range configSearchPaths {
				viper.AddConfigPath(path)
			}
			err = viper.ReadInConfig()
			if err != nil {
				if errors.As(err, &viper.ConfigFileNotFoundError{}) {
					// Config file not found is acceptable - try command-line flags
					slog.Info("configuration file not found")
				} else {
					return fmt.Errorf("configuration file read failed: %w", err)
				}
			}
		}

		switch strings.ToLower(viper.GetString("log.level")) {
		case "debug":
			logLevel.Set(slog.LevelDebug)
		case "info":
			logLevel.Set(slog.LevelInfo)
		case "warn":
			logLevel.Set(slog.LevelWarn)
		case "error":
			logLevel.Set(slog.LevelError)
		}

		// Parse HTTP address from positional argument if provided
		if len(args) > 0 {
			ip, port, err := parseHTTPAddress(args[0])
			if err != nil {
				return fmt.Errorf("invalid http_address: %w", err)
			}
			viper.Set("http.ip", ip)
			viper.Set("http.port", port)
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
	rootCmd.PersistentFlags().String("log-level", "info", "Set logging level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("db-type", "sqlite", "Database type (sqlite or postgres)")
	rootCmd.PersistentFlags().String("db-dsn", "", "Database DSN (connection string)")
	rootCmd.PersistentFlags().String("http-cert", "", "Path to server certificate")
	rootCmd.PersistentFlags().String("http-key", "", "Path to server private key")
	if err := viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("db.type", rootCmd.PersistentFlags().Lookup("db-type")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("db.dsn", rootCmd.PersistentFlags().Lookup("db-dsn")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("http.cert", rootCmd.PersistentFlags().Lookup("http-cert")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("http.key", rootCmd.PersistentFlags().Lookup("http-key")); err != nil {
		panic(err)
	}
}

func init() {
	rootLogger := slog.New(devlog.NewHandler(os.Stdout, &devlog.Options{
		Level: &logLevel,
	}))
	slog.SetDefault(rootLogger)
	viper.SetOptions(viper.WithLogger(rootLogger))
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
