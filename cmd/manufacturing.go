// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"log/slog"

	manufacturer "github.com/fido-device-onboard/go-fdo-server/api/v2/manufacturer"
	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/server"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
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

var manufacturingInitAdminCmd = &cobra.Command{
	Use:   "init-admin",
	Short: "Create initial admin user and API key",
	Long:  `Creates an admin user with full permissions and generates an API key. The API key is printed to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var mfgConfig config.ManufacturingServerConfig
		if err := viper.Unmarshal(&mfgConfig); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}

		gormDB, err := mfgConfig.DB.GetDB()
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		ctx := cmd.Context()

		if err := state.InitAuthDB(ctx, gormDB); err != nil {
			return fmt.Errorf("failed to initialize auth database: %w", err)
		}

		name, _ := cmd.Flags().GetString("name")
		email, _ := cmd.Flags().GetString("email")
		force, _ := cmd.Flags().GetBool("force")

		routeScopes, err := auth.ParseRouteScopes(manufacturer.OpenAPISpecJSON)
		if err != nil {
			return fmt.Errorf("failed to parse route scopes: %w", err)
		}
		serverScopes := auth.CollectAllScopes(routeScopes)

		admin := &state.SeedAdminConfig{Name: name, Email: email}
		apiKey, err := state.SeedAuth(ctx, gormDB, admin, serverScopes, force)
		if err != nil {
			return err
		}

		if apiKey == "" {
			return fmt.Errorf("auth database already seeded; use --force to create an additional admin user")
		}

		fmt.Println(apiKey)
		return nil
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

	// Add init-admin subcommand
	manufacturingCmd.AddCommand(manufacturingInitAdminCmd)
	manufacturingInitAdminCmd.Flags().String("name", "admin", "Admin user name")
	manufacturingInitAdminCmd.Flags().String("email", "admin@example.com", "Admin user email")
	manufacturingInitAdminCmd.Flags().Bool("force", false, "Create admin even if users exist")
}

func init() {
	manufacturingCmdInit()
}
