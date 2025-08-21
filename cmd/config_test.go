// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Configuration capture for testing
var capturedConfig *FIDOServerConfig

func resetState(t *testing.T) {
	t.Helper()

	// reinitialize the CLI/Config logic
	viper.Reset()
	rootCmd.ResetFlags()
	rootCmd.ResetCommands()
	rootCmd.SetArgs(nil)

	manufacturingCmd.ResetFlags()
	manufacturingCmd.ResetCommands()
	manufacturingCmd.SetArgs(nil)

	ownerCmd.ResetFlags()
	ownerCmd.ResetCommands()
	ownerCmd.SetArgs(nil)

	rendezvousCmd.ResetFlags()
	rendezvousCmd.ResetCommands()
	rendezvousCmd.SetArgs(nil)

	rootCmdInit()
	ownerCmdInit()
	manufacturingCmdInit()
	rendezvousCmdInit()

	// Zero globals populated by load functions
	date = false
	wgets = nil
	uploads = nil
	uploadDir = ""
	downloads = nil
	debug = false

	// Reset captured config
	capturedConfig = nil
}

// Stub out the command execution. We do not want to run the actual
// command, just verify that the configuration is correct
func stubRunE(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	orig := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {

		// Parse flags to ensure viper gets the command-line values
		if err := cmd.ParseFlags(args); err != nil {
			return err
		}

		// Handle positional argument override (same as in actual commands)
		if len(args) > 0 {
			switch cmd {
			case manufacturingCmd:
				viper.Set("manufacturing.http.listen", args[0])
			case ownerCmd:
				viper.Set("owner.http.listen", args[0])
			case rendezvousCmd:
				viper.Set("rendezvous.http.listen", args[0])
			}
		}

		// Capture the configuration that would be unmarshaled
		var fdoConfig FIDOServerConfig
		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return err
		}
		capturedConfig = &fdoConfig

		// Validate the configuration (same as in actual commands)
		switch cmd {
		case manufacturingCmd:
			if fdoConfig.Manufacturing == nil {
				return fmt.Errorf("failed to find manufacturing config")
			}
			if err := fdoConfig.Manufacturing.HTTP.validate(); err != nil {
				return err
			}
		case ownerCmd:
			if fdoConfig.Owner == nil {
				return fmt.Errorf("failed to find Owner config")
			}
			if err := fdoConfig.Owner.HTTP.validate(); err != nil {
				return err
			}
		case rendezvousCmd:
			if fdoConfig.Rendezvous == nil {
				return fmt.Errorf("failed to find rendezvous config")
			}
			if err := fdoConfig.Rendezvous.HTTP.validate(); err != nil {
				return err
			}
		}

		return nil
	}
	t.Cleanup(func() { cmd.RunE = orig })
}

func writeTOMLConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeYAMLConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManufacturing_LoadsFromTOMLConfig(t *testing.T) {
	type expectedConfig struct {
		address         string
		dbType          string
		dbDSN           string
		manufacturerKey string
		deviceCACert    string
		deviceCAKey     string
		ownerCert       string
	}

	tests := []struct {
		name     string
		config   string
		expected expectedConfig
	}{
		{
			name: "basic configuration",
			config: `
[manufacturing]
private-key = "/path/to/mfg.key"
owner-cert = "/path/to/owner.crt"
[manufacturing.http]
listen = "127.0.0.1:8081"
ssl = false
insecure-tls = true
[manufacturing.database]
type = "sqlite"
dsn = "file:/tmp/bar.db"
[manufacturing.device-ca]
cert = "/path/to/device.ca"
key = "/path/to/device.key"
`,
			expected: expectedConfig{
				address:         "127.0.0.1:8081",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/bar.db",
				manufacturerKey: "/path/to/mfg.key",
				deviceCACert:    "/path/to/device.ca",
				deviceCAKey:     "/path/to/device.key",
				ownerCert:       "/path/to/owner.crt",
			},
		},
		{
			name: "toml-specific configuration",
			config: `
[manufacturing]
private-key = "/path/to/toml-mfg.key"
owner-cert = "/path/to/toml-owner.crt"
[manufacturing.http]
listen = "127.0.0.1:8082"
ssl = false
insecure-tls = true
[manufacturing.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
[manufacturing.device-ca]
cert = "/path/to/toml-device.ca"
key = "/path/to/toml-device.key"
`,
			expected: expectedConfig{
				address:         "127.0.0.1:8082",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/database.db",
				manufacturerKey: "/path/to/toml-mfg.key",
				deviceCACert:    "/path/to/toml-device.ca",
				deviceCAKey:     "/path/to/toml-device.key",
				ownerCert:       "/path/to/toml-owner.crt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			stubRunE(t, manufacturingCmd)

			path := writeTOMLConfig(t, tt.config)
			rootCmd.SetArgs([]string{"manufacturing", "--config", path})

			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("execute failed: %v", err)
			}

			if capturedConfig == nil || capturedConfig.Manufacturing == nil {
				t.Fatalf("manufacturing config not captured")
			}

			cfg := capturedConfig.Manufacturing
			if cfg.HTTP.Listen != tt.expected.address {
				t.Fatalf("HTTP.Listen=%q, want %q", cfg.HTTP.Listen, tt.expected.address)
			}
			if cfg.DB.Type != tt.expected.dbType {
				t.Fatalf("DB.Type=%q, want %q", cfg.DB.DSN, tt.expected.dbType)
			}
			if cfg.DB.DSN != tt.expected.dbDSN {
				t.Fatalf("DB.Type=%q, want %q", cfg.DB.DSN, tt.expected.dbDSN)
			}
			if cfg.ManufacturerKeyPath != tt.expected.manufacturerKey {
				t.Fatalf("ManufacturerKeyPath=%q, want %q", cfg.ManufacturerKeyPath, tt.expected.manufacturerKey)
			}
			if cfg.DeviceCACert.CertPath != tt.expected.deviceCACert {
				t.Fatalf("DeviceCACert.CertPath=%q, want %q", cfg.DeviceCACert.CertPath, tt.expected.deviceCACert)
			}
			if cfg.DeviceCACert.KeyPath != tt.expected.deviceCAKey {
				t.Fatalf("DeviceCACert.KeyPath=%q, want %q", cfg.DeviceCACert.KeyPath, tt.expected.deviceCAKey)
			}
			if cfg.OwnerPublicKeyPath != tt.expected.ownerCert {
				t.Fatalf("OwnerPublicKeyPath=%q, want %q", cfg.OwnerPublicKeyPath, tt.expected.ownerCert)
			}
		})
	}
}

func TestOwner_LoadsFromTOMLConfig(t *testing.T) {
	type expectedOwnerConfig struct {
		address         string
		dbType          string
		dbDSN           string
		externalAddress string
		deviceCACert    string
		ownerKey        string
	}

	tests := []struct {
		name     string
		config   string
		expected expectedOwnerConfig
	}{
		{
			name: "basic owner configuration",
			config: `
[owner]
external-address = "0.0.0.0:8443"
reuse-credentials = true
device-ca-cert = "/path/to/owner.device.ca"
owner-key = "/path/to/owner.key"
[owner.http]
listen = "127.0.0.1:8082"
ssl = false
insecure-tls = true
[owner.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`,
			expected: expectedOwnerConfig{
				address:         "127.0.0.1:8082",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/database.db",
				externalAddress: "0.0.0.0:8443",
				deviceCACert:    "/path/to/owner.device.ca",
				ownerKey:        "/path/to/owner.key",
			},
		},
		{
			name: "toml-specific owner configuration",
			config: `
[owner]
external-address = "0.0.0.0:8444"
reuse-credentials = true
device-ca-cert = "/path/to/toml-owner.device.ca"
owner-key = "/path/to/toml-owner.key"
[owner.http]
listen = "127.0.0.1:8083"
ssl = false
insecure-tls = true
[owner.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`,
			expected: expectedOwnerConfig{
				address:         "127.0.0.1:8083",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/database.db",
				externalAddress: "0.0.0.0:8444",
				deviceCACert:    "/path/to/toml-owner.device.ca",
				ownerKey:        "/path/to/toml-owner.key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			stubRunE(t, ownerCmd)

			path := writeTOMLConfig(t, tt.config)
			rootCmd.SetArgs([]string{"owner", "--config", path})

			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("execute failed: %v", err)
			}

			if capturedConfig == nil || capturedConfig.Owner == nil {
				t.Fatalf("owner config not captured")
			}

			cfg := capturedConfig.Owner
			if cfg.HTTP.Listen != tt.expected.address {
				t.Fatalf("HTTP.Listen=%q, want %q", cfg.HTTP.Listen, tt.expected.address)
			}
			if cfg.DB.Type != tt.expected.dbType {
				t.Fatalf("DB.Type=%q, want %q", cfg.DB.DSN, tt.expected.dbType)
			}
			if cfg.DB.DSN != tt.expected.dbDSN {
				t.Fatalf("DB.Type=%q, want %q", cfg.DB.DSN, tt.expected.dbDSN)
			}
			if cfg.ExternalAddress != tt.expected.externalAddress {
				t.Fatalf("ExternalAddress=%q, want %q", cfg.ExternalAddress, tt.expected.externalAddress)
			}
			if cfg.OwnerDeviceCACert != tt.expected.deviceCACert {
				t.Fatalf("OwnerDeviceCACert=%q, want %q", cfg.OwnerDeviceCACert, tt.expected.deviceCACert)
			}
			if cfg.OwnerPrivateKey != tt.expected.ownerKey {
				t.Fatalf("OwnerPrivateKey=%q, want %q", cfg.OwnerPrivateKey, tt.expected.ownerKey)
			}

			// Note: wgets, uploads, downloads, uploadDir are command-line only arguments
			// and are not loaded from configuration files, so we don't validate them here
		})
	}
}

func TestRendezvous_LoadsFromTOMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
[rendezvous]
[rendezvous.http]
listen = "127.0.0.1:8083"
ssl = false
insecure-tls = true
[rendezvous.database]
type = "postgres"
dsn = "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Rendezvous == nil {
		t.Fatalf("rendezvous config not captured")
	}

	cfgObj := capturedConfig.Rendezvous
	if cfgObj.HTTP.Listen != "127.0.0.1:8083" {
		t.Fatalf("HTTP.Listen=%q, want %q", cfgObj.HTTP.Listen, "127.0.0.1:8083")
	}
	if cfgObj.DB.Type != "postgres" {
		t.Fatalf("DB.Type=%q, want %q", cfgObj.DB.Type, "postgres")
	}
	if cfgObj.DB.DSN != "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid" {
		t.Fatalf("DB.DSN=%q, want %q", cfgObj.DB.DSN, "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid")
	}
}

func TestManufacturing_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
[manufacturing]
[manufacturing.http]
listen = "1.2.3.4:1111"
ssl = false
[manufacturing.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path, "127.0.0.1:9090"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Manufacturing == nil {
		t.Fatalf("manufacturing config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.Manufacturing.HTTP.Listen != "127.0.0.1:9090" {
		t.Fatalf("HTTP.Listen=%q, want %q", capturedConfig.Manufacturing.HTTP.Listen, "127.0.0.1:9090")
	}
}

func TestOwner_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
[owner]
[owner.http]
listen = "1.2.3.4:1111"
ssl = false
[owner.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path, "127.0.0.1:9191"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Owner == nil {
		t.Fatalf("owner config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.Owner.HTTP.Listen != "127.0.0.1:9191" {
		t.Fatalf("HTTP.Listen=%q, want %q", capturedConfig.Owner.HTTP.Listen, "127.0.0.1:9191")
	}
}

func TestRendezvous_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
[rendezvous]
[rendezvous.http]
listen = "1.2.3.4:1111"
ssl = false
[rendezvous.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path, "127.0.0.1:9292"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Rendezvous == nil {
		t.Fatalf("rendezvous config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.Rendezvous.HTTP.Listen != "127.0.0.1:9292" {
		t.Fatalf("HTTP.Listen=%q, want %q", capturedConfig.Rendezvous.HTTP.Listen, "127.0.0.1:9292")
	}
}

func TestManufacturing_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
[manufacturing]
[manufacturing.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		fmt.Printf("VIPER: %s\n", viper.GetString("manufacturing.http.listen"))
		t.Fatalf("expected error for missing address")
	}
}

func TestOwner_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
[owner]
[owner.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestRendezvous_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
[rendezvous]
[rendezvous.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestManufacturing_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	rootCmd.SetArgs([]string{"manufacturing", "--config", "/no/such/file.toml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestOwner_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	rootCmd.SetArgs([]string{"owner", "--config", "/no/such/file.toml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestRendezvous_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	rootCmd.SetArgs([]string{"rendezvous", "--config", "/no/such/file.toml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestManufacturing_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
manufacturing:
  http:
    listen: "127.0.0.1:8081"
    ssl: false
    insecure-tls: true
  database:
    path: "test-yaml.db"
    password: "YamlPass123!"
  private-key: "/path/to/yaml-mfg.key"
  owner-cert: "/path/to/yaml-owner.crt"
  device-ca:
    cert: "/path/to/yaml-device.ca"
    key: "/path/to/yaml-device.key"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Manufacturing == nil {
		t.Fatalf("manufacturing config not captured")
	}

	cfgObj := capturedConfig.Manufacturing
	if cfgObj.ManufacturerKeyPath != "/path/to/yaml-mfg.key" {
		t.Fatalf("ManufacturerKeyPath=%q", cfgObj.ManufacturerKeyPath)
	}
	if cfgObj.DeviceCACert.CertPath != "/path/to/yaml-device.ca" {
		t.Fatalf("DeviceCACert.CertPath=%q", cfgObj.DeviceCACert.CertPath)
	}
	if cfgObj.DeviceCACert.KeyPath != "/path/to/yaml-device.key" {
		t.Fatalf("DeviceCACert.KeyPath=%q", cfgObj.DeviceCACert.KeyPath)
	}
	if cfgObj.OwnerPublicKeyPath != "/path/to/yaml-owner.crt" {
		t.Fatalf("OwnerPublicKeyPath=%q", cfgObj.OwnerPublicKeyPath)
	}
}

func TestOwner_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
owner:
  http:
    listen: "127.0.0.1:8082"
    ssl: false
    insecure-tls: true
  database:
    path: "test-owner-yaml.db"
    password: "OwnerYaml123!"
  external-address: "0.0.0.0:8443"
  device-ca-cert: "/path/to/yaml-owner.device.ca"
  owner-key: "/path/to/yaml-owner.key"
  reuse-credentials: true
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Owner == nil {
		t.Fatalf("owner config not captured")
	}

	cfgObj := capturedConfig.Owner
	if cfgObj.ExternalAddress != "0.0.0.0:8443" {
		t.Fatalf("ExternalAddress=%q", cfgObj.ExternalAddress)
	}
	if cfgObj.OwnerDeviceCACert != "/path/to/yaml-owner.device.ca" {
		t.Fatalf("OwnerDeviceCACert=%q", cfgObj.OwnerDeviceCACert)
	}
	if cfgObj.OwnerPrivateKey != "/path/to/yaml-owner.key" {
		t.Fatalf("OwnerPrivateKey=%q", cfgObj.OwnerPrivateKey)
	}

	// Note: command-line only options (wgets, uploads, downloads, uploadDir, date) are not loaded from configuration files
	// They are only available as command-line flags
}

func TestRendezvous_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
rendezvous:
  http:
    listen: "127.0.0.1:8083"
    ssl: false
    insecure-tls: true
  database:
    type: "sqlite"
    dsn: "file:test-rendezvous-yaml.db"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Rendezvous == nil {
		t.Fatalf("rendezvous config not captured")
	}

	cfgObj := capturedConfig.Rendezvous
	if cfgObj.HTTP.Listen != "127.0.0.1:8083" {
		t.Fatalf("HTTP.Listen=%q", cfgObj.HTTP.Listen)
	}
	if cfgObj.DB.DSN != "file:test-rendezvous-yaml.db" {
		t.Fatalf("DB.DSN=%q", cfgObj.DB.DSN)
	}
	if cfgObj.DB.Type != "sqlite" {
		t.Fatalf("DB.Type=%q", cfgObj.DB.Type)
	}
}

func TestManufacturing_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	// Create a configuration file with specific values
	cfg := `
[manufacturing]
private-key = "/config/mfg.key"
owner-cert = "/config/owner.crt"
[manufacturing.http]
listen = "127.0.0.1:8081"
ssl = false
insecure-tls = true
cert = "/config/server.crt"
key = "/config/server.key"
[manufacturing.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
[manufacturing.device-ca]
cert = "/config/device.ca"
key = "/config/device.key"
`
	path := writeTOMLConfig(t, cfg)

	// Set command-line flags that should override the config file values
	rootCmd.SetArgs([]string{
		"manufacturing",
		"--config", path,
		"127.0.0.1:9090", // positional argument for listen address
		"--manufacturing-key", "/cli/mfg.key",
		"--owner-cert", "/cli/owner.crt",
		"--device-ca-cert", "/cli/device.ca",
		"--device-ca-key", "/cli/device.key",
		"--db-dsn", "file:cli.db",
		"--insecure-tls=false",
		"--server-cert-path", "/cli/server.crt",
		"--server-key-path", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Manufacturing == nil {
		t.Fatalf("manufacturing config not captured")
	}

	cfgObj := capturedConfig.Manufacturing

	// Verify that command-line values overrode config file values
	if cfgObj.HTTP.Listen != "127.0.0.1:9090" {
		t.Fatalf("HTTP.Listen=%q, want %q (positional arg should override config)", cfgObj.HTTP.Listen, "127.0.0.1:9090")
	}
	if cfgObj.ManufacturerKeyPath != "/cli/mfg.key" {
		t.Fatalf("ManufacturerKeyPath=%q, want %q (CLI flag should override config)", cfgObj.ManufacturerKeyPath, "/cli/mfg.key")
	}
	if cfgObj.OwnerPublicKeyPath != "/cli/owner.crt" {
		t.Fatalf("OwnerPublicKeyPath=%q, want %q (CLI flag should override config)", cfgObj.OwnerPublicKeyPath, "/cli/owner.crt")
	}
	if cfgObj.DeviceCACert.CertPath != "/cli/device.ca" {
		t.Fatalf("DeviceCACert.CertPath=%q, want %q (CLI flag should override config)", cfgObj.DeviceCACert.CertPath, "/cli/device.ca")
	}
	if cfgObj.DeviceCACert.KeyPath != "/cli/device.key" {
		t.Fatalf("DeviceCACert.KeyPath=%q, want %q (CLI flag should override config)", cfgObj.DeviceCACert.KeyPath, "/cli/device.key")
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.HTTP.InsecureTLS != false {
		t.Fatalf("HTTP.InsecureTLS=%v, want %v (CLI flag should override config)", cfgObj.HTTP.InsecureTLS, false)
	}
	if cfgObj.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.CertPath, "/cli/server.crt")
	}
	if cfgObj.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.KeyPath, "/cli/server.key")
	}
}

func TestOwner_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Create a configuration file with specific values
	cfg := `
[owner]
external-address = "0.0.0.0:8443"
device-ca-cert = "/config/owner.device.ca"
owner-key = "/config/owner.key"
reuse-credentials = true
[owner.http]
listen = "127.0.0.1:8082"
ssl = false
insecure-tls = true
cert = "/config/server.crt"
key = "/config/server.key"
[owner.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)

	// Set command-line flags that should override the config file values
	rootCmd.SetArgs([]string{
		"owner",
		"--config", path,
		"127.0.0.1:9091", // positional argument for listen address
		"--external-address", "0.0.0.0:9443",
		"--device-ca-cert", "/cli/owner.device.ca",
		"--owner-key", "/cli/owner.key",
		"--reuse-credentials=false",
		"--db-dsn", "file:cli.db",
		"--insecure-tls=false",
		"--server-cert-path", "/cli/server.crt",
		"--server-key-path", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Owner == nil {
		t.Fatalf("owner config not captured")
	}

	cfgObj := capturedConfig.Owner

	// Verify that command-line values overrode config file values
	if cfgObj.HTTP.Listen != "127.0.0.1:9091" {
		t.Fatalf("HTTP.Listen=%q, want %q (positional arg should override config)", cfgObj.HTTP.Listen, "127.0.0.1:9091")
	}
	if cfgObj.ExternalAddress != "0.0.0.0:9443" {
		t.Fatalf("ExternalAddress=%q, want %q (CLI flag should override config)", cfgObj.ExternalAddress, "0.0.0.0:9443")
	}
	if cfgObj.OwnerDeviceCACert != "/cli/owner.device.ca" {
		t.Fatalf("OwnerDeviceCACert=%q, want %q (CLI flag should override config)", cfgObj.OwnerDeviceCACert, "/cli/owner.device.ca")
	}
	if cfgObj.OwnerPrivateKey != "/cli/owner.key" {
		t.Fatalf("OwnerPrivateKey=%q, want %q (CLI flag should override config)", cfgObj.OwnerPrivateKey, "/cli/owner.key")
	}
	if cfgObj.ReuseCred != false {
		t.Fatalf("ReuseCred=%v, want %v (CLI flag should override config)", cfgObj.ReuseCred, false)
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.HTTP.InsecureTLS != false {
		t.Fatalf("HTTP.InsecureTLS=%v, want %v (CLI flag should override config)", cfgObj.HTTP.InsecureTLS, false)
	}
	if cfgObj.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.CertPath, "/cli/server.crt")
	}
	if cfgObj.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.KeyPath, "/cli/server.key")
	}
}

func TestRendezvous_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	// Create a configuration file with specific values
	cfg := `
[rendezvous]
[rendezvous.http]
listen = "127.0.0.1:8083"
ssl = false
insecure-tls = true
cert = "/config/server.crt"
key = "/config/server.key"
[rendezvous.database]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)

	// Set command-line flags that should override the config file values
	rootCmd.SetArgs([]string{
		"rendezvous",
		"--config", path,
		"127.0.0.1:9092", // positional argument for listen address
		"--db-dsn", "file:cli.db",
		"--insecure-tls=false",
		"--server-cert-path", "/cli/server.crt",
		"--server-key-path", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil || capturedConfig.Rendezvous == nil {
		t.Fatalf("rendezvous config not captured")
	}

	cfgObj := capturedConfig.Rendezvous

	// Verify that command-line values overrode config file values
	if cfgObj.HTTP.Listen != "127.0.0.1:9092" {
		t.Fatalf("HTTP.Listen=%q, want %q (positional arg should override config)", cfgObj.HTTP.Listen, "127.0.0.1:9092")
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.Type=%q, want %q (CLI flag should override config)", cfgObj.DB.DSN, "file:cli.db")
	}
	if cfgObj.HTTP.InsecureTLS != false {
		t.Fatalf("HTTP.InsecureTLS=%v, want %v (CLI flag should override config)", cfgObj.HTTP.InsecureTLS, false)
	}
	if cfgObj.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.CertPath, "/cli/server.crt")
	}
	if cfgObj.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", cfgObj.HTTP.KeyPath, "/cli/server.key")
	}
}
