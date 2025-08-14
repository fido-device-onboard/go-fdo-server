// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"

	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hermannm.dev/devlog"
)

var (
	dbPath   string
	dbPass   string
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
		return rootCmdLoadConfig()
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

	rootCmd.PersistentFlags().Bool("debug", false, "Print debug contents")
	rootCmd.PersistentFlags().String("db", "", "SQLite database file path")
	rootCmd.PersistentFlags().String("db-pass", "", "SQLite database encryption-at-rest passphrase")
	// Note: viper does not enforce cobra's "MarkFlagRequired"
	// functionality when the configuration is read from a
	// file. Manually check for required flags when loading the
	// configuration.
	viper.BindPFlags(rootCmd.PersistentFlags())
}

// Initialize configuration flags from viper's configuration. Enforce
// required flags are present.
func rootCmdLoadConfig() error {
	if !viper.IsSet("db") {
		return errors.New("missing required path to the database (--db)")
	}
	if !viper.IsSet("db-pass") {
		return errors.New("missing database password (--db-pass)")
	}
	dbPath = viper.GetString("db")
	dbPass = viper.GetString("db-pass")
	err := validatePassword(dbPass)
	if err != nil {
		return err
	}
	debug = viper.GetBool("debug")
	if debug {
		logLevel.Set(slog.LevelDebug)
	}
	return nil
}

const (
	minPasswordLength = 8
)

func getState() (*sqlite.DB, error) {
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
