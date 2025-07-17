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
