// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("Binding rendezvous command flags")
		// Rebind only those keys needed by the rendezvous command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		if err := viper.BindPFlag("rendezvous.to0_min_wait", cmd.Flags().Lookup("to0-min-wait")); err != nil {
			slog.Error("Failed to bind to0-min-wait flag", "err", err)
			return err
		}
		if err := viper.BindPFlag("rendezvous.to0_max_wait", cmd.Flags().Lookup("to0-max-wait")); err != nil {
			slog.Error("Failed to bind to0-max-wait flag", "err", err)
			return err
		}
		slog.Debug("Flags bound successfully")
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var rvConfig config.RendezvousServerConfig
		if err := viper.Unmarshal(&rvConfig); err != nil {
			return fmt.Errorf("failed to unmarshal rendezvous config: %w", err)
		}
		slog.Debug("Configuration loaded", "config", rvConfig)
		if err := rvConfig.Validate(); err != nil {
			return err
		}
		srv, err := server.NewRendezvousServer(rvConfig)
		if err != nil {
			return fmt.Errorf("failed to create rendezvous server: %w", err)
		}
		return srv.Start()
	},
}

// Set up the rendezvous command line. Used by the unit tests to reset state between tests.
func rendezvousCmdInit() {
	rootCmd.AddCommand(rendezvousCmd)
	rendezvousCmd.Flags().Uint32("to0-min-wait", config.DefaultMinWaitSecs, "Minimum wait time in seconds for TO0 rendezvous entries (requests below this are rejected, default: 0 = no minimum)")
	rendezvousCmd.Flags().Uint32("to0-max-wait", config.DefaultMaxWaitSecs, fmt.Sprintf("Maximum wait time in seconds for TO0 rendezvous entries (requests above this are capped, default: %d seconds)", config.DefaultMaxWaitSecs))
	viper.SetDefault("rendezvous.to0_min_wait", config.DefaultMinWaitSecs)
	viper.SetDefault("rendezvous.to0_max_wait", config.DefaultMaxWaitSecs)
}

func init() {
	rendezvousCmdInit()
}
