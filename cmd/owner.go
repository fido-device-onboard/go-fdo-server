// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"log/slog"

	owner "github.com/fido-device-onboard/go-fdo-server/api/v2/owner"
	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/server"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ownerCmd represents the owner command
var ownerCmd = &cobra.Command{
	Use:   "owner [ip_address:port]",
	Short: "Run an FDO Owner server",
	Long: `Run an FDO Owner server that handles device onboarding.

The Owner server runs the TO2 protocol to onboard devices. It also runs the
TO0 protocol against the Rendezvous server, registering itself as a device owner.`,
	Example: `  # Run an Owner server on port 8043 using a configuration file:
  go-fdo-server owner 0.0.0.0:8043 --config /etc/go-fdo-server/owner.yaml`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Rebind only those keys needed by the owner command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		if err := viper.BindPFlag("owner.reuse_credentials", cmd.Flags().Lookup("reuse-credentials")); err != nil {
			return err
		}
		if err := viper.BindPFlag("device_ca.cert", cmd.Flags().Lookup("device-ca-cert")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.key", cmd.Flags().Lookup("owner-key")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.to0_insecure_tls", cmd.Flags().Lookup("to0-insecure-tls")); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var ownerServerConfig config.OwnerServerConfig
		if err := viper.Unmarshal(&ownerServerConfig); err != nil {
			return fmt.Errorf("failed to unmarshal owner config: %w", err)
		}
		if err := ownerServerConfig.Validate(); err != nil {
			return fmt.Errorf("failed to validate config: %w", err)
		}
		slog.Debug("Parsed configuration", "config", ownerServerConfig.String())
		ownerServer, err := server.NewOwnerServer(ownerServerConfig)
		if err != nil {
			return fmt.Errorf("failed to create new owner server: %w", err)
		}
		return ownerServer.Start()
	},
}

var ownerInitAdminCmd = &cobra.Command{
	Use:   "init-admin",
	Short: "Create initial admin user and API key",
	Long:  `Creates an admin user with full permissions and generates an API key. The API key is printed to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ownerConfig config.OwnerServerConfig
		if err := viper.Unmarshal(&ownerConfig); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}

		gormDB, err := ownerConfig.DB.GetDB()
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

		routeScopes, err := auth.ParseRouteScopes(owner.OpenAPISpecJSON)
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

// Set up the owner command line. Used by the unit tests to reset state between tests.
func ownerCmdInit() {
	rootCmd.AddCommand(ownerCmd)

	// Declare any CLI flags for overriding configuration file settings.
	// These flags are bound to Viper in the ownerCmd PreRun handler.
	ownerCmd.Flags().Bool("reuse-credentials", false, "Perform the Credential Reuse Protocol during the TO2 protocol")
	ownerCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	ownerCmd.Flags().String("owner-key", "", "Owner private key path")
	ownerCmd.Flags().Bool("to0-insecure-tls", false, "Use insecure TLS (skip Rendezvous certificate verification) for the TO0 protocol")
	ownerCmd.Flags().BoolP("help", "h", false, "Help for Owner server")

	// Add init-admin subcommand
	ownerCmd.AddCommand(ownerInitAdminCmd)
	ownerInitAdminCmd.Flags().String("name", "admin", "Admin user name")
	ownerInitAdminCmd.Flags().String("email", "admin@example.com", "Admin user email")
	ownerInitAdminCmd.Flags().Bool("force", false, "Create admin even if users exist")
}

func init() {
	ownerCmdInit()
}
