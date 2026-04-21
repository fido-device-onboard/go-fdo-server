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

// manufacturingCmd represents the manufacturing command
var manufacturingCmd = &cobra.Command{
	Use:   "manufacturing [ip_address:port]",
	Short: "Run an FDO Manufacturing server",
	Long: `Run an FDO Manufacturing server that handles device initialization (DI).

The Manufacturing server runs the DI protocol to initialize devices and
generate Ownership Vouchers.`,
	Example: `  # Run a Manufacturing server on port 8038 using a configuration file:
  go-fdo-server manufacturing 0.0.0.0:8038 --config /etc/go-fdo-server/manufacturing.yaml`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Rebind only those keys needed by the manufacturing
		// command. This is necessary because Viper cannot bind the
		// same key twice and the other sub commands use the same
		// keys.
		if err := viper.BindPFlag("manufacturing.key", cmd.Flags().Lookup("manufacturing-key")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.cert", cmd.Flags().Lookup("owner-cert")); err != nil {
			return err
		}
		if err := viper.BindPFlag("device_ca.cert", cmd.Flags().Lookup("device-ca-cert")); err != nil {
			return err
		}
		if err := viper.BindPFlag("device_ca.key", cmd.Flags().Lookup("device-ca-key")); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var mfgConfig config.ManufacturingServerConfig
		if err := viper.Unmarshal(&mfgConfig); err != nil {
			return fmt.Errorf("failed to unmarshal manufacturing config: %w", err)
		}
		slog.Debug("Configuration loaded", "config", mfgConfig)
		if err := mfgConfig.Validate(); err != nil {
			return err
		}
		srv, err := server.NewManufacturingServer(mfgConfig)
		if err != nil {
			return fmt.Errorf("failed to create manufacturing server: %w", err)
		}
		return srv.Start()
	},
}

// Set up the manufacturing command line. Used by the unit tests to reset state between tests.
func manufacturingCmdInit() {
	rootCmd.AddCommand(manufacturingCmd)

	// Declare any CLI flags for overriding configuration file settings.
	// These flags are bound to Viper in the manufacturingCmd PreRun handler.
	manufacturingCmd.Flags().String("manufacturing-key", "", "Manufacturing private key path")
	manufacturingCmd.Flags().String("owner-cert", "", "Owner certificate path")
	manufacturingCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	manufacturingCmd.Flags().String("device-ca-key", "", "Device CA private key path")
	manufacturingCmd.Flags().BoolP("help", "h", false, "Help for Manufacturing server")
}

func init() {
	manufacturingCmdInit()
}
