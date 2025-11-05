// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"os"
	"path/filepath"
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

	// Zero globals populated by load functions
	date = false
	wgets = nil
	uploads = nil
	uploadDir = ""
	downloads = nil

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
	}

	tests := []struct {
		name     string
		config   string
		expected expectedConfig
	}{
		{
			name: "basic configuration",
			config: `
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
			},
		},
		{
			name: "toml-specific configuration",
			config: `
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

			// Note: wgets, uploads, downloads, uploadDir are command-line only arguments
			// and are not loaded from configuration files, so we don't validate them here
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

	// Note: command-line only options (wgets, uploads, downloads, uploadDir, date) are not loaded from configuration files
	// They are only available as command-line flags
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
