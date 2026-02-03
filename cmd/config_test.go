// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Configuration capture for testing
type TestFullConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	DeviceCA        DeviceCAConfig      `mapstructure:"device_ca"`
	Manufacturer    ManufacturingConfig `mapstructure:"manufacturing"`
	Owner           OwnerConfig         `mapstructure:"owner"`
	Rendezvous      RendezvousConfig    `mapstructure:"rendezvous"`
}

var capturedConfig *TestFullConfig

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

	// Reset captured config
	capturedConfig = nil
}

// Stub out the command execution. We do not want to run the actual
// command, just verify that the configuration is correct
func stubRunE(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	orig := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Capture the configuration that would be unmarshaled
		// Note: flags are already parsed by cobra before RunE is called,
		// and PersistentPreRunE has already loaded the config file into viper
		var fdoConfig TestFullConfig
		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return err
		}
		capturedConfig = &fdoConfig

		// Validate the configuration (same as in actual commands)
		if err := fdoConfig.HTTP.validate(); err != nil {
			return err
		}

		// Validate ServiceInfo to trigger UnmarshalParams()
		if err := fdoConfig.Owner.ServiceInfo.validate(); err != nil {
			return err
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
		ip              string
		port            string
		dbType          string
		dbDSN           string
		manufacturerKey string
		deviceCACert    string
		deviceCAKey     string
		ownerCert       string
		logLevel        string
	}

	tests := []struct {
		name     string
		config   string
		expected expectedConfig
	}{
		{
			name: "basic configuration",
			config: `
[log]
level = "warn"
[http]
ip = "127.0.0.1"
port = "8081"
[db]
type = "sqlite"
dsn = "file:/tmp/bar.db"
[device_ca]
cert = "/path/to/device.ca"
key = "/path/to/device.key"
[manufacturing]
key = "/path/to/mfg.key"
[owner]
cert = "/path/to/owner.crt"
`,
			expected: expectedConfig{
				ip:              "127.0.0.1",
				port:            "8081",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/bar.db",
				manufacturerKey: "/path/to/mfg.key",
				deviceCACert:    "/path/to/device.ca",
				deviceCAKey:     "/path/to/device.key",
				ownerCert:       "/path/to/owner.crt",
				logLevel:        "warn",
			},
		},
		{
			name: "toml-specific configuration",
			config: `
[log]
level = "warn"
[http]
ip = "127.0.0.1"
port = "8082"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[device_ca]
cert = "/path/to/toml-device.ca"
key = "/path/to/toml-device.key"
[manufacturing]
key = "/path/to/toml-mfg.key"
[owner]
cert = "/path/to/toml-owner.crt"
`,
			expected: expectedConfig{
				ip:              "127.0.0.1",
				port:            "8082",
				dbType:          "sqlite",
				dbDSN:           "file:/tmp/database.db",
				manufacturerKey: "/path/to/toml-mfg.key",
				deviceCACert:    "/path/to/toml-device.ca",
				deviceCAKey:     "/path/to/toml-device.key",
				ownerCert:       "/path/to/toml-owner.crt",
				logLevel:        "warn",
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

			if capturedConfig == nil {
				t.Fatalf("manufacturing config not captured")
			}

			if capturedConfig.HTTP.IP != tt.expected.ip {
				t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, tt.expected.ip)
			}
			if capturedConfig.HTTP.Port != tt.expected.port {
				t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, tt.expected.port)
			}
			if capturedConfig.DB.Type != tt.expected.dbType {
				t.Fatalf("DB.Type=%q, want %q", capturedConfig.DB.Type, tt.expected.dbType)
			}
			if capturedConfig.DB.DSN != tt.expected.dbDSN {
				t.Fatalf("DB.DSN=%q, want %q", capturedConfig.DB.DSN, tt.expected.dbDSN)
			}
			if capturedConfig.Manufacturer.ManufacturerKeyPath != tt.expected.manufacturerKey {
				t.Fatalf("Manufacturer.ManufacturerKeyPath=%q, want %q", capturedConfig.Manufacturer.ManufacturerKeyPath, tt.expected.manufacturerKey)
			}
			if capturedConfig.DeviceCA.CertPath != tt.expected.deviceCACert {
				t.Fatalf("DeviceCA.CertPath=%q, want %q", capturedConfig.DeviceCA.CertPath, tt.expected.deviceCACert)
			}
			if capturedConfig.DeviceCA.KeyPath != tt.expected.deviceCAKey {
				t.Fatalf("DeviceCA.KeyPath=%q, want %q", capturedConfig.DeviceCA.KeyPath, tt.expected.deviceCAKey)
			}
			if capturedConfig.Owner.OwnerCertificate != tt.expected.ownerCert {
				t.Fatalf("Owner.OwnerCertificate=%q, want %q", capturedConfig.Owner.OwnerCertificate, tt.expected.ownerCert)
			}
			if capturedConfig.Log.Level != tt.expected.logLevel {
				t.Fatalf("Log.Level=%q, want %q", capturedConfig.Log.Level, tt.expected.logLevel)
			}
		})
	}
}

func TestOwner_LoadsFromTOMLConfig(t *testing.T) {
	type expectedOwnerConfig struct {
		ip           string
		port         string
		dbType       string
		dbDSN        string
		deviceCACert string
		ownerKey     string
	}

	tests := []struct {
		name     string
		config   string
		expected expectedOwnerConfig
	}{
		{
			name: "basic owner configuration",
			config: `
[http]
ip = "127.0.0.1"
port = "8082"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[device_ca]
cert = "/path/to/owner.device.ca"
[owner]
reuse_credentials = true
key = "/path/to/owner.key"
to0_insecure_tls = false
`,
			expected: expectedOwnerConfig{
				ip:           "127.0.0.1",
				port:         "8082",
				dbType:       "sqlite",
				dbDSN:        "file:/tmp/database.db",
				deviceCACert: "/path/to/owner.device.ca",
				ownerKey:     "/path/to/owner.key",
			},
		},
		{
			name: "toml-specific owner configuration",
			config: `
[http]
ip = "127.0.0.1"
port = "8083"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[device_ca]
cert = "/path/to/toml-owner.device.ca"
[owner]
external-address = "0.0.0.0:8444"
reuse_credentials = true
key = "/path/to/toml-owner.key"
to0_insecure_tls = false
`,
			expected: expectedOwnerConfig{
				ip:           "127.0.0.1",
				port:         "8083",
				dbType:       "sqlite",
				dbDSN:        "file:/tmp/database.db",
				deviceCACert: "/path/to/toml-owner.device.ca",
				ownerKey:     "/path/to/toml-owner.key",
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

			if capturedConfig == nil {
				t.Fatalf("owner config not captured")
			}

			if capturedConfig.HTTP.IP != tt.expected.ip {
				t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, tt.expected.ip)
			}
			if capturedConfig.HTTP.Port != tt.expected.port {
				t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, tt.expected.port)
			}
			if capturedConfig.DB.Type != tt.expected.dbType {
				t.Fatalf("DB.Type=%q, want %q", capturedConfig.DB.Type, tt.expected.dbType)
			}
			if capturedConfig.DB.DSN != tt.expected.dbDSN {
				t.Fatalf("DB.DSN=%q, want %q", capturedConfig.DB.DSN, tt.expected.dbDSN)
			}
			if capturedConfig.DeviceCA.CertPath != tt.expected.deviceCACert {
				t.Fatalf("DeviceCA.CertPath=%q, want %q", capturedConfig.DeviceCA.CertPath, tt.expected.deviceCACert)
			}
			if capturedConfig.Owner.OwnerPrivateKey != tt.expected.ownerKey {
				t.Fatalf("Owner.OwnerPrivateKey=%q, want %q", capturedConfig.Owner.OwnerPrivateKey, tt.expected.ownerKey)
			}
		})
	}
}

func TestRendezvous_LoadsFromTOMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
[http]
ip = "127.0.0.1"
port = "8083"
[db]
type = "postgres"
dsn = "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("rendezvous config not captured")
	}

	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "8083" {
		t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, "8083")
	}
	if capturedConfig.DB.Type != "postgres" {
		t.Fatalf("DB.Type=%q, want %q", capturedConfig.DB.Type, "postgres")
	}
	if capturedConfig.DB.DSN != "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid" {
		t.Fatalf("DB.DSN=%q, want %q", capturedConfig.DB.DSN, "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid")
	}
}

func TestManufacturing_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
[http]
ip = "1.2.3.4"
port = "1111"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[manufacturing]
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path, "127.0.0.1:9090"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("manufacturing config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9090" {
		t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, "9090")
	}
}

func TestOwner_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
[http]
ip = "1.2.3.4"
port = "1111"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[owner]
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path, "127.0.0.1:9191"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9191" {
		t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, "9191")
	}
}

func TestRendezvous_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
[http]
ip = "1.2.3.4"
port = "1111"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path, "127.0.0.1:9292"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("rendezvous config not captured")
	}

	// The positional argument should override the config file value
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9292" {
		t.Fatalf("HTTP.Port=%q, want %q", capturedConfig.HTTP.Port, "9292")
	}
}

func TestManufacturing_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[manufacturing]
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestOwner_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[owner]
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
[db]
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
log:
  level: "error"
http:
  ip: "127.0.0.1"
  port: "8081"
db:
  type: "sqlite"
  dsn: "file:test-yaml.db"
device_ca:
  cert: "/path/to/yaml-device.ca"
  key: "/path/to/yaml-device.key"
manufacturing:
  key: "/path/to/yaml-mfg.key"
owner:
  cert: "/path/to/yaml-owner.crt"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("manufacturing config not captured")
	}

	if capturedConfig.Manufacturer.ManufacturerKeyPath != "/path/to/yaml-mfg.key" {
		t.Fatalf("Manufacturer.ManufacturerKeyPath=%q", capturedConfig.Manufacturer.ManufacturerKeyPath)
	}
	if capturedConfig.DeviceCA.CertPath != "/path/to/yaml-device.ca" {
		t.Fatalf("DeviceCA.CertPath=%q", capturedConfig.DeviceCA.CertPath)
	}
	if capturedConfig.DeviceCA.KeyPath != "/path/to/yaml-device.key" {
		t.Fatalf("DeviceCA.KeyPath=%q", capturedConfig.DeviceCA.KeyPath)
	}
	if capturedConfig.Owner.OwnerCertificate != "/path/to/yaml-owner.crt" {
		t.Fatalf("Owner.OwnerCertificate=%q", capturedConfig.Owner.OwnerCertificate)
	}
	if capturedConfig.Log.Level != "error" {
		t.Fatalf("Log.Level=%q, want %q", capturedConfig.Log.Level, "error")
	}
}

func TestOwner_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:test-owner-yaml.db"
device_ca:
  cert: "/path/to/yaml-owner.device.ca"
owner:
  key: "/path/to/yaml-owner.key"
  reuse_credentials: true
  to0_insecure_tls: false
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	if capturedConfig.DeviceCA.CertPath != "/path/to/yaml-owner.device.ca" {
		t.Fatalf("DeviceCA.CertPath=%q", capturedConfig.DeviceCA.CertPath)
	}
	if capturedConfig.Owner.OwnerPrivateKey != "/path/to/yaml-owner.key" {
		t.Fatalf("Owner.OwnerPrivateKey=%q", capturedConfig.Owner.OwnerPrivateKey)
	}
}

func TestRendezvous_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8083"
db:
  type: "sqlite"
  dsn: "file:test-rendezvous-yaml.db"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("rendezvous config not captured")
	}

	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q", capturedConfig.HTTP.IP)
	}
	if capturedConfig.HTTP.Port != "8083" {
		t.Fatalf("HTTP.Port=%q", capturedConfig.HTTP.Port)
	}
	if capturedConfig.DB.DSN != "file:test-rendezvous-yaml.db" {
		t.Fatalf("DB.DSN=%q", capturedConfig.DB.DSN)
	}
	if capturedConfig.DB.Type != "sqlite" {
		t.Fatalf("DB.Type=%q", capturedConfig.DB.Type)
	}
}

func TestManufacturing_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	// Create a configuration file with specific values
	cfg := `
[http]
ip = "127.0.0.1"
port = "8081"
cert = "/config/server.crt"
key = "/config/server.key"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[device_ca]
cert = "/config/device.ca"
key = "/config/device.key"
[manufacturing]
key = "/config/mfg.key"
[owner]
cert = "/config/owner.crt"
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
		"--http-cert", "/cli/server.crt",
		"--http-key", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("manufacturing config not captured")
	}

	// Verify that command-line values overrode config file values
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q (positional arg should override config)", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9090" {
		t.Fatalf("HTTP.Port=%q, want %q (positional arg should override config)", capturedConfig.HTTP.Port, "9090")
	}
	if capturedConfig.Manufacturer.ManufacturerKeyPath != "/cli/mfg.key" {
		t.Fatalf("Manufacturer.ManufacturerKeyPath=%q, want %q (CLI flag should override config)", capturedConfig.Manufacturer.ManufacturerKeyPath, "/cli/mfg.key")
	}
	if capturedConfig.Owner.OwnerCertificate != "/cli/owner.crt" {
		t.Fatalf("Owner.OwnerCertificate=%q, want %q (CLI flag should override config)", capturedConfig.Owner.OwnerCertificate, "/cli/owner.crt")
	}
	if capturedConfig.DeviceCA.CertPath != "/cli/device.ca" {
		t.Fatalf("DeviceCA.CertPath=%q, want %q (CLI flag should override config)", capturedConfig.DeviceCA.CertPath, "/cli/device.ca")
	}
	if capturedConfig.DeviceCA.KeyPath != "/cli/device.key" {
		t.Fatalf("DeviceCA.KeyPath=%q, want %q (CLI flag should override config)", capturedConfig.DeviceCA.KeyPath, "/cli/device.key")
	}
	if capturedConfig.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.DSN=%q, want %q (CLI flag should override config)", capturedConfig.DB.DSN, "file:cli.db")
	}
	if capturedConfig.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.CertPath, "/cli/server.crt")
	}
	if capturedConfig.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.KeyPath, "/cli/server.key")
	}
}

func TestOwner_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Create a configuration file with specific values
	cfg := `
[http]
ip = "127.0.0.1"
port = "8082"
cert = "/config/server.crt"
key = "/config/server.key"
[db]
type = "sqlite"
dsn = "file:/tmp/database.db"
[device_ca]
cert = "/config/owner.device.ca"
[owner]
key = "/config/owner.key"
reuse_credentials = true
to0_insecure_tls = true
`
	path := writeTOMLConfig(t, cfg)

	// Set command-line flags that should override the config file values
	rootCmd.SetArgs([]string{
		"owner",
		"--config", path,
		"127.0.0.1:9091", // positional argument for listen address
		"--device-ca-cert", "/cli/owner.device.ca",
		"--owner-key", "/cli/owner.key",
		"--reuse-credentials=false",
		"--db-dsn", "file:cli.db",
		"--to0-insecure-tls=false",
		"--http-cert", "/cli/server.crt",
		"--http-key", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	// Verify that command-line values overrode config file values
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q (positional arg should override config)", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9091" {
		t.Fatalf("HTTP.Port=%q, want %q (positional arg should override config)", capturedConfig.HTTP.Port, "9091")
	}
	if capturedConfig.DeviceCA.CertPath != "/cli/owner.device.ca" {
		t.Fatalf("DeviceCA.CertPath=%q, want %q (CLI flag should override config)", capturedConfig.DeviceCA.CertPath, "/cli/owner.device.ca")
	}
	if capturedConfig.Owner.OwnerPrivateKey != "/cli/owner.key" {
		t.Fatalf("Owner.OwnerPrivateKey=%q, want %q (CLI flag should override config)", capturedConfig.Owner.OwnerPrivateKey, "/cli/owner.key")
	}
	if capturedConfig.Owner.ReuseCred != false {
		t.Fatalf("Owner.ReuseCred=%v, want %v (CLI flag should override config)", capturedConfig.Owner.ReuseCred, false)
	}
	if capturedConfig.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.DSN=%q, want %q (CLI flag should override config)", capturedConfig.DB.DSN, "file:cli.db")
	}
	if capturedConfig.Owner.TO0InsecureTLS != false {
		t.Fatalf("Owner.TO0InsecureTLS=%v, want %v (CLI flag should override config)", capturedConfig.Owner.TO0InsecureTLS, false)
	}
	if capturedConfig.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.CertPath, "/cli/server.crt")
	}
	if capturedConfig.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.KeyPath, "/cli/server.key")
	}
}

func TestRendezvous_CommandLineFlagsOverrideConfigFile(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	// Create a configuration file with specific values
	cfg := `
[http]
ip = "127.0.0.1"
port = "8083"
cert = "/config/server.crt"
key = "/config/server.key"
[db]
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
		"--http-cert", "/cli/server.crt",
		"--http-key", "/cli/server.key",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("rendezvous config not captured")
	}

	// Verify that command-line values overrode config file values
	if capturedConfig.HTTP.IP != "127.0.0.1" {
		t.Fatalf("HTTP.IP=%q, want %q (positional arg should override config)", capturedConfig.HTTP.IP, "127.0.0.1")
	}
	if capturedConfig.HTTP.Port != "9092" {
		t.Fatalf("HTTP.Port=%q, want %q (positional arg should override config)", capturedConfig.HTTP.Port, "9092")
	}
	if capturedConfig.DB.DSN != "file:cli.db" {
		t.Fatalf("DB.DSN=%q, want %q (CLI flag should override config)", capturedConfig.DB.DSN, "file:cli.db")
	}
	if capturedConfig.HTTP.CertPath != "/cli/server.crt" {
		t.Fatalf("HTTP.CertPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.CertPath, "/cli/server.crt")
	}
	if capturedConfig.HTTP.KeyPath != "/cli/server.key" {
		t.Fatalf("HTTP.KeyPath=%q, want %q (CLI flag should override config)", capturedConfig.HTTP.KeyPath, "/cli/server.key")
	}
}

func TestOwner_FSIMConfigFromTOML(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Test checksums (SHA-384, 96 hex characters)
	checksum1 := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	checksum2 := "ef567890ef567890ef567890ef567890ef567890ef567890ef567890ef567890ef567890ef567890ef567890ef567890"

	// Create temporary files for download test.
	// These files must exist because the ServiceInfo validation code (triggered by
	// stubRunE) verifies that fdo.download source files are accessible on the filesystem.
	// See ServiceInfoConfig.validate() in cmd/config.go for the file existence check.
	dir := t.TempDir()
	// Create a subdirectory to test relative paths
	subdir := filepath.Join(dir, "files")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}
	// file1 will be accessed via relative path
	file1 := filepath.Join(subdir, "data.bin")
	if err := os.WriteFile(file1, []byte("test data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// file2 will be accessed via absolute path
	file2 := filepath.Join(dir, "optional.log")
	if err := os.WriteFile(file2, []byte("test log"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := fmt.Sprintf(`
[http]
ip = "127.0.0.1"
port = "8082"
[db]
type = "sqlite"
dsn = "file:/tmp/owner.db"
[device_ca]
cert = "/path/to/device.ca"
[owner]
key = "/path/to/owner.key"

[owner.service_info]
[[owner.service_info.fsims]]
fsim = "fdo.command"
[owner.service_info.fsims.params]
cmd = "/usr/bin/echo"
args = ["hello", "world"]
may_fail = true
return_stdout = true
return_stderr = false

[[owner.service_info.fsims]]
fsim = "fdo.upload"
[owner.service_info.fsims.params]
dir = "/upload/base"
[[owner.service_info.fsims.params.files]]
src = "/local/file1.txt"
dst = "subdir/newfile1.txt"
[[owner.service_info.fsims.params.files]]
src = "/local/file2.txt"
dst = "another/file2.txt"

[[owner.service_info.fsims]]
fsim = "fdo.download"
[owner.service_info.fsims.params]
dir = "%s"
[[owner.service_info.fsims.params.files]]
src = "files/data.bin"
dst = "/local/data.bin"
may_fail = false
[[owner.service_info.fsims.params.files]]
src = "%s"
dst = "/local/optional.log"
may_fail = true

[[owner.service_info.fsims]]
fsim = "fdo.wget"
[[owner.service_info.fsims.params.files]]
url = "https://example.com/file1.tar.gz"
dst = "/tmp/file1.tar.gz"
length = 12345
checksum = "%s"
[[owner.service_info.fsims.params.files]]
url = "http://example.org/file2.zip"
dst = "/tmp/file2.zip"
length = 67890
checksum = "%s"
`, dir, file2, checksum1, checksum2)
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	// Verify service_info has 4 operations
	if len(capturedConfig.Owner.ServiceInfo.Fsims) != 4 {
		t.Fatalf("ServiceInfo.Fsims length=%d, want 4", len(capturedConfig.Owner.ServiceInfo.Fsims))
	}

	// Verify fdo.command operation
	cmdOp := capturedConfig.Owner.ServiceInfo.Fsims[0]
	if cmdOp.FSIM != "fdo.command" {
		t.Fatalf("ServiceInfo.Fsims[0].FSIM=%q, want %q", cmdOp.FSIM, "fdo.command")
	}
	if cmdOp.CommandParams == nil {
		t.Fatal("ServiceInfo.Fsims[0].CommandParams is nil")
	}
	if cmdOp.CommandParams.Command != "/usr/bin/echo" {
		t.Fatalf("CommandParams.Command=%q, want %q", cmdOp.CommandParams.Command, "/usr/bin/echo")
	}
	if len(cmdOp.CommandParams.Args) != 2 || cmdOp.CommandParams.Args[0] != "hello" || cmdOp.CommandParams.Args[1] != "world" {
		t.Fatalf("CommandParams.Args=%v, want [hello world]", cmdOp.CommandParams.Args)
	}
	if !cmdOp.CommandParams.MayFail {
		t.Fatalf("CommandParams.MayFail=%v, want true", cmdOp.CommandParams.MayFail)
	}
	if !cmdOp.CommandParams.RetStdout {
		t.Fatalf("CommandParams.RetStdout=%v, want true", cmdOp.CommandParams.RetStdout)
	}
	if cmdOp.CommandParams.RetStderr {
		t.Fatalf("CommandParams.RetStderr=%v, want false", cmdOp.CommandParams.RetStderr)
	}

	// Verify fdo.upload operation
	uploadOp := capturedConfig.Owner.ServiceInfo.Fsims[1]
	if uploadOp.FSIM != "fdo.upload" {
		t.Fatalf("ServiceInfo.Fsims[1].FSIM=%q, want %q", uploadOp.FSIM, "fdo.upload")
	}
	if uploadOp.UploadParams == nil {
		t.Fatal("ServiceInfo.Fsims[1].UploadParams is nil")
	}
	if uploadOp.UploadParams.Dir != "/upload/base" {
		t.Fatalf("UploadParams.Dir=%q, want %q", uploadOp.UploadParams.Dir, "/upload/base")
	}
	if len(uploadOp.UploadParams.Files) != 2 {
		t.Fatalf("UploadParams.Files length=%d, want 2", len(uploadOp.UploadParams.Files))
	}
	if uploadOp.UploadParams.Files[0].Src != "/local/file1.txt" {
		t.Fatalf("UploadParams.Files[0].Src=%q, want %q", uploadOp.UploadParams.Files[0].Src, "/local/file1.txt")
	}
	if uploadOp.UploadParams.Files[0].Dst != "subdir/newfile1.txt" {
		t.Fatalf("UploadParams.Files[0].Dst=%q, want %q", uploadOp.UploadParams.Files[0].Dst, "subdir/newfile1.txt")
	}
	if uploadOp.UploadParams.Files[1].Src != "/local/file2.txt" {
		t.Fatalf("UploadParams.Files[1].Src=%q, want %q", uploadOp.UploadParams.Files[1].Src, "/local/file2.txt")
	}
	if uploadOp.UploadParams.Files[1].Dst != "another/file2.txt" {
		t.Fatalf("UploadParams.Files[1].Dst=%q, want %q", uploadOp.UploadParams.Files[1].Dst, "another/file2.txt")
	}

	// Verify fdo.download operation
	downloadOp := capturedConfig.Owner.ServiceInfo.Fsims[2]
	if downloadOp.FSIM != "fdo.download" {
		t.Fatalf("ServiceInfo.Fsims[2].FSIM=%q, want %q", downloadOp.FSIM, "fdo.download")
	}
	if downloadOp.DownloadParams == nil {
		t.Fatal("ServiceInfo.Fsims[2].DownloadParams is nil")
	}
	if downloadOp.DownloadParams.Dir != dir {
		t.Fatalf("DownloadParams.Dir=%q, want %q", downloadOp.DownloadParams.Dir, dir)
	}
	if len(downloadOp.DownloadParams.Files) != 2 {
		t.Fatalf("DownloadParams.Files length=%d, want 2", len(downloadOp.DownloadParams.Files))
	}
	if downloadOp.DownloadParams.Files[0].Src != "files/data.bin" {
		t.Fatalf("DownloadParams.Files[0].Src=%q, want %q", downloadOp.DownloadParams.Files[0].Src, "files/data.bin")
	}
	if downloadOp.DownloadParams.Files[0].Dst != "/local/data.bin" {
		t.Fatalf("DownloadParams.Files[0].Dst=%q, want %q", downloadOp.DownloadParams.Files[0].Dst, "/local/data.bin")
	}
	if downloadOp.DownloadParams.Files[0].MayFail {
		t.Fatalf("DownloadParams.Files[0].MayFail=%v, want false", downloadOp.DownloadParams.Files[0].MayFail)
	}
	if downloadOp.DownloadParams.Files[1].Src != file2 {
		t.Fatalf("DownloadParams.Files[1].Src=%q, want %q", downloadOp.DownloadParams.Files[1].Src, file2)
	}
	if downloadOp.DownloadParams.Files[1].Dst != "/local/optional.log" {
		t.Fatalf("DownloadParams.Files[1].Dst=%q, want %q", downloadOp.DownloadParams.Files[1].Dst, "/local/optional.log")
	}
	if !downloadOp.DownloadParams.Files[1].MayFail {
		t.Fatalf("DownloadParams.Files[1].MayFail=%v, want true", downloadOp.DownloadParams.Files[1].MayFail)
	}

	// Verify fdo.wget operation
	wgetOp := capturedConfig.Owner.ServiceInfo.Fsims[3]
	if wgetOp.FSIM != "fdo.wget" {
		t.Fatalf("ServiceInfo.Fsims[3].FSIM=%q, want %q", wgetOp.FSIM, "fdo.wget")
	}
	if wgetOp.WgetParams == nil {
		t.Fatal("ServiceInfo.Fsims[3].WgetParams is nil")
	}
	if len(wgetOp.WgetParams.Files) != 2 {
		t.Fatalf("WgetParams.Files length=%d, want 2", len(wgetOp.WgetParams.Files))
	}
	if wgetOp.WgetParams.Files[0].URL != "https://example.com/file1.tar.gz" {
		t.Fatalf("WgetParams.Files[0].URL=%q, want %q", wgetOp.WgetParams.Files[0].URL, "https://example.com/file1.tar.gz")
	}
	if wgetOp.WgetParams.Files[0].Dst != "/tmp/file1.tar.gz" {
		t.Fatalf("WgetParams.Files[0].Dst=%q, want %q", wgetOp.WgetParams.Files[0].Dst, "/tmp/file1.tar.gz")
	}
	if wgetOp.WgetParams.Files[0].Length != 12345 {
		t.Fatalf("WgetParams.Files[0].Length=%d, want 12345", wgetOp.WgetParams.Files[0].Length)
	}
	if wgetOp.WgetParams.Files[0].Checksum != checksum1 {
		t.Fatalf("WgetParams.Files[0].Checksum=%q, want %q", wgetOp.WgetParams.Files[0].Checksum, checksum1)
	}
	if wgetOp.WgetParams.Files[1].URL != "http://example.org/file2.zip" {
		t.Fatalf("WgetParams.Files[1].URL=%q, want %q", wgetOp.WgetParams.Files[1].URL, "http://example.org/file2.zip")
	}
	if wgetOp.WgetParams.Files[1].Dst != "/tmp/file2.zip" {
		t.Fatalf("WgetParams.Files[1].Dst=%q, want %q", wgetOp.WgetParams.Files[1].Dst, "/tmp/file2.zip")
	}
	if wgetOp.WgetParams.Files[1].Length != 67890 {
		t.Fatalf("WgetParams.Files[1].Length=%d, want 67890", wgetOp.WgetParams.Files[1].Length)
	}
	if wgetOp.WgetParams.Files[1].Checksum != checksum2 {
		t.Fatalf("WgetParams.Files[1].Checksum=%q, want %q", wgetOp.WgetParams.Files[1].Checksum, checksum2)
	}
}

func TestOwner_FSIMConfigFromYAML(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Test checksums (SHA-384, 96 hex characters)
	checksum1 := "ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEF01"
	checksum2 := "0123456789ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEFabcdef0123456789ABCDEFabcdef01234542"

	// Create temporary files for download test.
	// These files must exist because the ServiceInfo validation code (triggered by
	// stubRunE) verifies that fdo.download source files are accessible on the filesystem.
	// See ServiceInfoConfig.validate() in cmd/config.go for the file existence check.
	dir := t.TempDir()
	// Create a subdirectory to test relative paths
	subdir := filepath.Join(dir, "data")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}
	// file1 will be accessed via relative path
	file1 := filepath.Join(subdir, "critical.dat")
	if err := os.WriteFile(file1, []byte("critical data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// file2 will be accessed via absolute path
	file2 := filepath.Join(dir, "extra.txt")
	if err := os.WriteFile(file2, []byte("extra data"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := fmt.Sprintf(`
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    fsims:
      - fsim: "fdo.command"
        params:
          may_fail: false
          return_stdout: false
          return_stderr: true
          cmd: "/bin/bash"
          args:
            - "-c"
            - |
              #! /bin/bash
              set -xeuo pipefail
              echo "Current Date:"
              date
              dmidecode --quiet --dump-bin /var/lib/fdo/upload/dmidecode
      - fsim: "fdo.upload"
        params:
          dir: "/var/upload"
          files:
            - src: "/source/config.yaml"
              dst: "configs/app-config.yaml"
            - src: "/source/data.json"
              dst: "data/app-data.json"

      - fsim: "fdo.download"
        params:
          dir: "%s"
          files:
            - src: "data/critical.dat"
              dst: "/client/critical.dat"
              may_fail: false
            - src: "%s"
              dst: "/client/extra.txt"
              may_fail: true

      - fsim: "fdo.wget"
        params:
          files:
            - url: "https://cdn.example.com/package.rpm"
              dst: "/tmp/package.rpm"
              length: 98765
              checksum: "%s"
            - url: "http://repo.example.net/archive.tar.gz"
              dst: "/tmp/archive.tar.gz"
              length: 54321
              checksum: "%s"
`, dir, file2, checksum1, checksum2)
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	// Verify service_info has 4 operations
	if len(capturedConfig.Owner.ServiceInfo.Fsims) != 4 {
		t.Fatalf("ServiceInfo.Fsims length=%d, want 4", len(capturedConfig.Owner.ServiceInfo.Fsims))
	}

	// Verify fdo.command operation
	cmdOp := capturedConfig.Owner.ServiceInfo.Fsims[0]
	if cmdOp.FSIM != "fdo.command" {
		t.Fatalf("ServiceInfo.Fsims[0].FSIM=%q, want %q", cmdOp.FSIM, "fdo.command")
	}
	if cmdOp.CommandParams == nil {
		t.Fatal("ServiceInfo.Fsims[0].CommandParams is nil")
	}
	if cmdOp.CommandParams.Command != "/bin/bash" {
		t.Fatalf("CommandParams.Command=%q, want %q", cmdOp.CommandParams.Command, "/bin/bash")
	}
	for i, cmd := range cmdOp.CommandParams.Args {
		fmt.Printf("Arg [%d] = %s\n", i, cmd)
	}
	expected_args := `#! /bin/bash
set -xeuo pipefail
echo "Current Date:"
date
dmidecode --quiet --dump-bin /var/lib/fdo/upload/dmidecode
`
	if len(cmdOp.CommandParams.Args) != 2 || cmdOp.CommandParams.Args[0] != "-c" || cmdOp.CommandParams.Args[1] != expected_args {
		t.Fatalf("CommandParams.Args=%v, want [\"-c\", %s ]", cmdOp.CommandParams.Args, expected_args)
	}
	if cmdOp.CommandParams.MayFail {
		t.Fatalf("CommandParams.MayFail=%v, want false", cmdOp.CommandParams.MayFail)
	}
	if cmdOp.CommandParams.RetStdout {
		t.Fatalf("CommandParams.RetStdout=%v, want false", cmdOp.CommandParams.RetStdout)
	}
	if !cmdOp.CommandParams.RetStderr {
		t.Fatalf("CommandParams.RetStderr=%v, want true", cmdOp.CommandParams.RetStderr)
	}

	// Verify fdo.upload operation
	uploadOp := capturedConfig.Owner.ServiceInfo.Fsims[1]
	if uploadOp.FSIM != "fdo.upload" {
		t.Fatalf("ServiceInfo.Fsims[1].FSIM=%q, want %q", uploadOp.FSIM, "fdo.upload")
	}
	if uploadOp.UploadParams == nil {
		t.Fatal("ServiceInfo.Fsims[1].UploadParams is nil")
	}
	if uploadOp.UploadParams.Dir != "/var/upload" {
		t.Fatalf("UploadParams.Dir=%q, want %q", uploadOp.UploadParams.Dir, "/var/upload")
	}
	if len(uploadOp.UploadParams.Files) != 2 {
		t.Fatalf("UploadParams.Files length=%d, want 2", len(uploadOp.UploadParams.Files))
	}
	if uploadOp.UploadParams.Files[0].Src != "/source/config.yaml" {
		t.Fatalf("UploadParams.Files[0].Src=%q, want %q", uploadOp.UploadParams.Files[0].Src, "/source/config.yaml")
	}
	if uploadOp.UploadParams.Files[0].Dst != "configs/app-config.yaml" {
		t.Fatalf("UploadParams.Files[0].Dst=%q, want %q", uploadOp.UploadParams.Files[0].Dst, "configs/app-config.yaml")
	}
	if uploadOp.UploadParams.Files[1].Src != "/source/data.json" {
		t.Fatalf("UploadParams.Files[1].Src=%q, want %q", uploadOp.UploadParams.Files[1].Src, "/source/data.json")
	}
	if uploadOp.UploadParams.Files[1].Dst != "data/app-data.json" {
		t.Fatalf("UploadParams.Files[1].Dst=%q, want %q", uploadOp.UploadParams.Files[1].Dst, "data/app-data.json")
	}

	// Verify fdo.download operation
	downloadOp := capturedConfig.Owner.ServiceInfo.Fsims[2]
	if downloadOp.FSIM != "fdo.download" {
		t.Fatalf("ServiceInfo.Fsims[2].FSIM=%q, want %q", downloadOp.FSIM, "fdo.download")
	}
	if downloadOp.DownloadParams == nil {
		t.Fatal("ServiceInfo.Fsims[2].DownloadParams is nil")
	}
	if downloadOp.DownloadParams.Dir != dir {
		t.Fatalf("DownloadParams.Dir=%q, want %q", downloadOp.DownloadParams.Dir, dir)
	}
	if len(downloadOp.DownloadParams.Files) != 2 {
		t.Fatalf("DownloadParams.Files length=%d, want 2", len(downloadOp.DownloadParams.Files))
	}
	if downloadOp.DownloadParams.Files[0].Src != "data/critical.dat" {
		t.Fatalf("DownloadParams.Files[0].Src=%q, want %q", downloadOp.DownloadParams.Files[0].Src, "data/critical.dat")
	}
	if downloadOp.DownloadParams.Files[0].Dst != "/client/critical.dat" {
		t.Fatalf("DownloadParams.Files[0].Dst=%q, want %q", downloadOp.DownloadParams.Files[0].Dst, "/client/critical.dat")
	}
	if downloadOp.DownloadParams.Files[0].MayFail {
		t.Fatalf("DownloadParams.Files[0].MayFail=%v, want false", downloadOp.DownloadParams.Files[0].MayFail)
	}
	if downloadOp.DownloadParams.Files[1].Src != file2 {
		t.Fatalf("DownloadParams.Files[1].Src=%q, want %q", downloadOp.DownloadParams.Files[1].Src, file2)
	}
	if downloadOp.DownloadParams.Files[1].Dst != "/client/extra.txt" {
		t.Fatalf("DownloadParams.Files[1].Dst=%q, want %q", downloadOp.DownloadParams.Files[1].Dst, "/client/extra.txt")
	}
	if !downloadOp.DownloadParams.Files[1].MayFail {
		t.Fatalf("DownloadParams.Files[1].MayFail=%v, want true", downloadOp.DownloadParams.Files[1].MayFail)
	}

	// Verify fdo.wget operation
	wgetOp := capturedConfig.Owner.ServiceInfo.Fsims[3]
	if wgetOp.FSIM != "fdo.wget" {
		t.Fatalf("ServiceInfo.Fsims[3].FSIM=%q, want %q", wgetOp.FSIM, "fdo.wget")
	}
	if wgetOp.WgetParams == nil {
		t.Fatal("ServiceInfo.Fsims[3].WgetParams is nil")
	}
	if len(wgetOp.WgetParams.Files) != 2 {
		t.Fatalf("WgetParams.Files length=%d, want 2", len(wgetOp.WgetParams.Files))
	}
	if wgetOp.WgetParams.Files[0].URL != "https://cdn.example.com/package.rpm" {
		t.Fatalf("WgetParams.Files[0].URL=%q, want %q", wgetOp.WgetParams.Files[0].URL, "https://cdn.example.com/package.rpm")
	}
	if wgetOp.WgetParams.Files[0].Dst != "/tmp/package.rpm" {
		t.Fatalf("WgetParams.Files[0].Dst=%q, want %q", wgetOp.WgetParams.Files[0].Dst, "/tmp/package.rpm")
	}
	if wgetOp.WgetParams.Files[0].Length != 98765 {
		t.Fatalf("WgetParams.Files[0].Length=%d, want 98765", wgetOp.WgetParams.Files[0].Length)
	}
	if wgetOp.WgetParams.Files[0].Checksum != checksum1 {
		t.Fatalf("WgetParams.Files[0].Checksum=%q, want %q", wgetOp.WgetParams.Files[0].Checksum, checksum1)
	}
	if wgetOp.WgetParams.Files[1].URL != "http://repo.example.net/archive.tar.gz" {
		t.Fatalf("WgetParams.Files[1].URL=%q, want %q", wgetOp.WgetParams.Files[1].URL, "http://repo.example.net/archive.tar.gz")
	}
	if wgetOp.WgetParams.Files[1].Dst != "/tmp/archive.tar.gz" {
		t.Fatalf("WgetParams.Files[1].Dst=%q, want %q", wgetOp.WgetParams.Files[1].Dst, "/tmp/archive.tar.gz")
	}
	if wgetOp.WgetParams.Files[1].Length != 54321 {
		t.Fatalf("WgetParams.Files[1].Length=%d, want 54321", wgetOp.WgetParams.Files[1].Length)
	}
	if wgetOp.WgetParams.Files[1].Checksum != checksum2 {
		t.Fatalf("WgetParams.Files[1].Checksum=%q, want %q", wgetOp.WgetParams.Files[1].Checksum, checksum2)
	}
}

func TestOwner_ServiceInfoDefaults_Valid(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Create temporary directories for download and upload defaults
	downloadDir := t.TempDir()
	uploadDir := t.TempDir()

	// Create a test file in download directory
	testFile := filepath.Join(downloadDir, "test.bin")
	if err := os.WriteFile(testFile, []byte("test data"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := fmt.Sprintf(`
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - fsim: "fdo.download"
        dir: "%s"
      - fsim: "fdo.upload"
        dir: "%s"
      - fsim: "fdo.wget"
        dir: "/device/downloads"
    fsims:
      - fsim: "fdo.download"
        params:
          # No dir specified - should use default
          files:
            - src: "test.bin"
              dst: "/tmp/test.bin"
      - fsim: "fdo.upload"
        params:
          # Override default with custom dir
          dir: "/custom/upload"
          files:
            - src: "/device/file.txt"
              dst: "uploaded.txt"
      - fsim: "fdo.wget"
        params:
          # No dir specified - should use default
          files:
            - url: "https://example.com/file.tar.gz"
              dst: "file.tar.gz"
`, downloadDir, uploadDir)

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capturedConfig == nil {
		t.Fatalf("owner config not captured")
	}

	// Verify defaults are set
	if len(capturedConfig.Owner.ServiceInfo.Defaults) != 3 {
		t.Fatalf("ServiceInfo.Defaults length=%d, want 3", len(capturedConfig.Owner.ServiceInfo.Defaults))
	}

	// Check fdo.download default
	if capturedConfig.Owner.ServiceInfo.Defaults[0].FSIM != "fdo.download" {
		t.Fatalf("Defaults[0].FSIM=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[0].FSIM, "fdo.download")
	}
	if capturedConfig.Owner.ServiceInfo.Defaults[0].Dir != downloadDir {
		t.Fatalf("Defaults[0].Dir=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[0].Dir, downloadDir)
	}

	// Check fdo.upload default
	if capturedConfig.Owner.ServiceInfo.Defaults[1].FSIM != "fdo.upload" {
		t.Fatalf("Defaults[1].FSIM=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[1].FSIM, "fdo.upload")
	}
	if capturedConfig.Owner.ServiceInfo.Defaults[1].Dir != uploadDir {
		t.Fatalf("Defaults[1].Dir=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[1].Dir, uploadDir)
	}

	// Check fdo.wget default
	if capturedConfig.Owner.ServiceInfo.Defaults[2].FSIM != "fdo.wget" {
		t.Fatalf("Defaults[2].FSIM=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[2].FSIM, "fdo.wget")
	}
	if capturedConfig.Owner.ServiceInfo.Defaults[2].Dir != "/device/downloads" {
		t.Fatalf("Defaults[2].Dir=%q, want %q", capturedConfig.Owner.ServiceInfo.Defaults[2].Dir, "/device/downloads")
	}

	// Verify fdo.download operation uses default
	downloadOp := capturedConfig.Owner.ServiceInfo.Fsims[0]
	if downloadOp.DownloadParams.Dir != downloadDir {
		t.Fatalf("DownloadParams.Dir=%q, want default %q", downloadOp.DownloadParams.Dir, downloadDir)
	}

	// Verify fdo.upload operation overrides default
	uploadOp := capturedConfig.Owner.ServiceInfo.Fsims[1]
	if uploadOp.UploadParams.Dir != "/custom/upload" {
		t.Fatalf("UploadParams.Dir=%q, want override %q", uploadOp.UploadParams.Dir, "/custom/upload")
	}

	// Verify fdo.wget operation uses default
	wgetOp := capturedConfig.Owner.ServiceInfo.Fsims[2]
	if wgetOp.WgetParams.Dir != "/device/downloads" {
		t.Fatalf("WgetParams.Dir=%q, want default %q", wgetOp.WgetParams.Dir, "/device/downloads")
	}
}

func TestOwner_ServiceInfoDefaults_DuplicateFsim(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	dir := t.TempDir()

	cfg := fmt.Sprintf(`
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - fsim: "fdo.download"
        dir: "%s"
      - fsim: "fdo.download"
        dir: "%s"
    fsims: []
`, dir, dir)

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for duplicate fsim in defaults, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate fsim value") {
		t.Fatalf("expected error about duplicate fsim, got: %v", err)
	}
}

func TestOwner_ServiceInfoDefaults_MissingFields(t *testing.T) {
	tests := []struct {
		name   string
		config string
		errMsg string
	}{
		{
			name: "missing fsim field",
			config: `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - dir: "/some/dir"
    fsims: []
`,
			errMsg: "fsim field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			stubRunE(t, ownerCmd)

			path := writeYAMLConfig(t, tt.config)
			rootCmd.SetArgs([]string{"owner", "--config", path})

			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Fatalf("expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

func TestOwner_ServiceInfoDefaults_InvalidFsimType(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - fsim: "fdo.invalid"
        dir: "/some/dir"
    fsims: []
`

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid fsim type, got nil")
	}
	if !strings.Contains(err.Error(), "fsim must be one of") {
		t.Fatalf("expected error about invalid fsim type, got: %v", err)
	}
}

func TestOwner_ServiceInfoDefaults_RelativePath(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - fsim: "fdo.download"
        dir: "relative/path"
    fsims: []
`

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for relative path in defaults, got nil")
	}
	if !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("expected error about absolute path requirement, got: %v", err)
	}
}

func TestOwner_ServiceInfoDefaults_NonExistentDirectory(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    defaults:
      - fsim: "fdo.download"
        dir: "/this/directory/does/not/exist"
    fsims: []
`

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
	if !strings.Contains(err.Error(), "cannot access directory") {
		t.Fatalf("expected error about directory access, got: %v", err)
	}
}

func TestOwner_UploadParams_AbsoluteDstRejected(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
http:
  ip: "127.0.0.1"
  port: "8082"
db:
  type: "sqlite"
  dsn: "file:/tmp/owner.db"
device_ca:
  cert: "/path/to/device.ca"
owner:
  key: "/path/to/owner.key"
  service_info:
    fsims:
      - fsim: "fdo.upload"
        params:
          dir: "/var/upload"
          files:
            - src: "/device/file.txt"
              dst: "/absolute/path/file.txt"
`

	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for absolute path in upload dst, got nil")
	}
	if !strings.Contains(err.Error(), "dst must be a relative path") {
		t.Fatalf("expected error about relative path requirement, got: %v", err)
	}
}
