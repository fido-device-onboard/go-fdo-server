// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

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
	Use:   "go-fdo-server {manufacturing|rendezvous|owner}",
	Short: "Run a FIDO Device Onboard (FDO) server",
	Long: `Run an FDO Manufacturing, Rendezvous, or Owner server.

Use one of the subcommands to run a Manufacturing, Rendezvous, or Owner
server instance. Each subcommand accepts an ip_address:port argument specifying the listen address.`,
	Example: `  # Run a Manufacturing server on port 8038 using a configuration file:
  go-fdo-server manufacturing 0.0.0.0:8038 --config /etc/go-fdo-server/manufacturing.yaml`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// bootstrap debug logging early to include configuration loading
		level, _ := cmd.Flags().GetString("log-level")
		if strings.ToLower(level) == "debug" {
			logLevel.Set(slog.LevelDebug)
		}

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
			filename := cmd.Name() // base filename, no suffix e.g. "manufacturing"
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

		// Parse ip_address:port from positional argument if provided
		if len(args) > 0 {
			ip, port, err := parseHTTPAddress(args[0])
			if err != nil {
				return fmt.Errorf("invalid ip_address:port: %w", err)
			}
			viper.Set("http.ip", ip)
			viper.Set("http.port", port)
		}

		return nil
	},
}

// Root returns the root cobra command for use by documentation generators.
func Root() *cobra.Command { return rootCmd }

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
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().String("config", "", "Pathname of the configuration file")
	rootCmd.PersistentFlags().String("log-level", "info", "Set logging level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("db-type", "sqlite", "Database type (sqlite or postgres)")
	rootCmd.PersistentFlags().String("db-dsn", "", "Database DSN (connection string)")
	rootCmd.PersistentFlags().String("http-cert", "", "Path to server certificate")
	rootCmd.PersistentFlags().String("http-key", "", "Path to server private key")
	for _, binding := range []struct{ key, flag string }{
		{"log.level", "log-level"},
		{"db.type", "db-type"},
		{"db.dsn", "db-dsn"},
		{"http.cert", "http-cert"},
		{"http.key", "http-key"},
	} {
		if err := viper.BindPFlag(binding.key, rootCmd.PersistentFlags().Lookup(binding.flag)); err != nil {
			slog.Error("Failed to bind flag", "key", binding.key, "flag", binding.flag, "error", err)
			os.Exit(1)
		}
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
