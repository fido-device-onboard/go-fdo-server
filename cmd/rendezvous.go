// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rendezvousFlagConfig defines a single flag's metadata for the rendezvous command.
type rendezvousFlagConfig struct {
	name         string // CLI flag name (e.g., "cleanup-interval")
	viperKey     string // Viper config key (e.g., "rendezvous.cleanup_interval")
	defaultValue uint32 // Default value
	description  string // Help text; may contain a single %d for the default value
}

// rendezvousFlags is the single source of truth for all rendezvous command flags.
var rendezvousFlags = []rendezvousFlagConfig{
	{
		name:         "to0-min-wait",
		viperKey:     "rendezvous.to0_min_wait",
		defaultValue: config.DefaultMinWaitSecs,
		description:  "Minimum wait time the Rendezvous server will accept for an entry registered by the Owner server during TO0 protocol. If the Owner server requests a shorter wait time, it is rejected (default: 0 = no minimum)",
	},
	{
		name:         "to0-max-wait",
		viperKey:     "rendezvous.to0_max_wait",
		defaultValue: config.DefaultMaxWaitSecs,
		description:  "Maximum wait time the Rendezvous server will keep an entry registered by the Owner server during TO0 protocol before it expires. If the Owner server requests a longer wait time, it is capped to this value (default: %d seconds)",
	},
	{
		name:         "cleanup-interval",
		viperKey:     "rendezvous.cleanup_interval",
		defaultValue: config.DefaultCleanupIntervalSecs,
		description:  "Interval in seconds for automatic cleanup of expired rendezvous blobs and sessions (set to 0 to disable, default: %d seconds)",
	},
	{
		name:         "session-timeout",
		viperKey:     "rendezvous.session_timeout",
		defaultValue: config.DefaultSessionMaxAgeSecs,
		description:  "Maximum age in seconds for sessions before cleanup (default: %d seconds)",
	},
	{
		name:         "initial-cleanup-delay",
		viperKey:     "rendezvous.initial_cleanup_delay",
		defaultValue: config.DefaultInitialCleanupDelaySecs,
		description:  "Delay in seconds before first cleanup after startup (default: %d seconds)",
	},
}

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous [ip_address:port]",
	Short: "Run an FDO Rendezvous server",
	Long: `Run an FDO Rendezvous server that mediates device onboarding.

The Rendezvous server acts as an intermediary between devices and Owner servers.
It accepts registration requests from Owner servers via the TO0 protocol and
directs devices to their Owner server during the TO1 protocol.`,
	Example: `  # Run a Rendezvous server on port 8041 using a configuration file:
  go-fdo-server rendezvous 0.0.0.0:8041 --config /etc/go-fdo-server/rendezvous.yaml`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("Binding rendezvous command flags")
		// Rebind only those keys needed by the rendezvous command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		for _, flag := range rendezvousFlags {
			if err := viper.BindPFlag(flag.viperKey, cmd.Flags().Lookup(flag.name)); err != nil {
				slog.Error("Failed to bind flag", "flag", flag.name, "error", err)
				return fmt.Errorf("failed to bind %s flag: %w", flag.name, err)
			}
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
	rendezvousCmd.Flags().BoolP("help", "h", false, "Help for Rendezvous server")
	for _, flag := range rendezvousFlags {
		description := flag.description
		if strings.Contains(description, "%d") {
			description = fmt.Sprintf(description, flag.defaultValue)
		}
		rendezvousCmd.Flags().Uint32(flag.name, flag.defaultValue, description)
		viper.SetDefault(flag.viperKey, flag.defaultValue)
	}
}

func init() {
	rendezvousCmdInit()
}
