// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func resetState(t *testing.T) {
	t.Helper()
	// Reset viper state and rebind flags so precedence works
	viper.Reset()
	_ = viper.BindPFlags(rootCmd.PersistentFlags())
	_ = viper.BindPFlags(manufacturingCmd.Flags())
	_ = viper.BindPFlags(ownerCmd.Flags())
	_ = viper.BindPFlags(rendezvousCmd.Flags())

	// Zero globals populated by load functions
	address = ""
	insecureTLS = false
	serverCertPath = ""
	serverKeyPath = ""
	externalAddress = ""
	date = false
	wgets = nil
	uploads = nil
	uploadDir = ""
	downloads = nil
	reuseCred = false

	dbPath = ""
	dbPass = ""
	debug = false

	// Manufacturing specific
	manufacturerKeyPath = ""
	deviceCACertPath = ""
	deviceCAKeyPath = ""
	ownerPublicKeyPath = ""

	// Owner specific
	ownerDeviceCACert = ""
	ownerPrivateKey = ""

	rootCmd.SetArgs(nil)
	manufacturingCmd.SetArgs(nil)
	ownerCmd.SetArgs(nil)
	rendezvousCmd.SetArgs(nil)
}

// Stub out the command execution. We do not want to run the actual
// command, just verify that the configuration is correct
func stubRunE(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	orig := cmd.RunE
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
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
		dbPath          string
		dbPass          string
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
address = "127.0.0.1:8081"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
manufacturing-key = "/path/to/mfg.key"
device-ca-cert = "/path/to/device.ca"
device-ca-key = "/path/to/device.key"
owner-cert = "/path/to/owner.crt"
`,
			expected: expectedConfig{
				address:         "127.0.0.1:8081",
				dbPath:          "test.db",
				dbPass:          "Abcdef1!",
				manufacturerKey: "/path/to/mfg.key",
				deviceCACert:    "/path/to/device.ca",
				deviceCAKey:     "/path/to/device.key",
				ownerCert:       "/path/to/owner.crt",
			},
		},
		{
			name: "toml-specific configuration",
			config: `
address = "127.0.0.1:8082"
db = "test-toml.db"
db-pass = "TomlPass123!"
debug = true
insecure-tls = true
manufacturing-key = "/path/to/toml-mfg.key"
device-ca-cert = "/path/to/toml-device.ca"
device-ca-key = "/path/to/toml-device.key"
owner-cert = "/path/to/toml-owner.crt"
`,
			expected: expectedConfig{
				address:         "127.0.0.1:8082",
				dbPath:          "test-toml.db",
				dbPass:          "TomlPass123!",
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

			if address != tt.expected.address {
				t.Fatalf("address=%q, want %q", address, tt.expected.address)
			}
			if dbPath != tt.expected.dbPath || dbPass != tt.expected.dbPass {
				t.Fatalf("db not loaded: path=%q pass=%q, want path=%q pass=%q", dbPath, dbPass, tt.expected.dbPath, tt.expected.dbPass)
			}
			if !insecureTLS || !debug {
				t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
			}
			if manufacturerKeyPath != tt.expected.manufacturerKey {
				t.Fatalf("manufacturerKeyPath=%q, want %q", manufacturerKeyPath, tt.expected.manufacturerKey)
			}
			if deviceCACertPath != tt.expected.deviceCACert {
				t.Fatalf("deviceCACertPath=%q, want %q", deviceCACertPath, tt.expected.deviceCACert)
			}
			if deviceCAKeyPath != tt.expected.deviceCAKey {
				t.Fatalf("deviceCAKeyPath=%q, want %q", deviceCAKeyPath, tt.expected.deviceCAKey)
			}
			if ownerPublicKeyPath != tt.expected.ownerCert {
				t.Fatalf("ownerPublicKeyPath=%q, want %q", ownerPublicKeyPath, tt.expected.ownerCert)
			}
		})
	}
}

func TestOwner_LoadsFromTOMLConfig(t *testing.T) {
	type expectedOwnerConfig struct {
		address         string
		dbPath          string
		dbPass          string
		externalAddress string
		wgets           []string
		uploads         []string
		uploadDir       string
		downloads       []string
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
address = "127.0.0.1:8082"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
external-address = "0.0.0.0:8443"
command-date = true
command-wget = ["https://a/x", "https://b/y"]
command-upload = ["a.txt", "b.txt"]
upload-directory = "/tmp/uploads"
command-download = ["c.txt"]
reuse-credentials = true
device-ca-cert = "/path/to/owner.device.ca"
owner-key = "/path/to/owner.key"
`,
			expected: expectedOwnerConfig{
				address:         "127.0.0.1:8082",
				dbPath:          "test.db",
				dbPass:          "Abcdef1!",
				externalAddress: "0.0.0.0:8443",
				wgets:           []string{"https://a/x", "https://b/y"},
				uploads:         []string{"a.txt", "b.txt"},
				uploadDir:       "/tmp/uploads",
				downloads:       []string{"c.txt"},
				deviceCACert:    "/path/to/owner.device.ca",
				ownerKey:        "/path/to/owner.key",
			},
		},
		{
			name: "toml-specific owner configuration",
			config: `
address = "127.0.0.1:8083"
db = "test-owner-toml.db"
db-pass = "OwnerToml123!"
debug = true
insecure-tls = true
external-address = "0.0.0.0:8444"
command-date = true
command-wget = ["https://toml.example.com/file1", "https://toml.example.com/file2"]
command-upload = ["toml-upload1.txt", "toml-upload2.txt"]
upload-directory = "/tmp/toml-uploads"
command-download = ["toml-download1.txt"]
reuse-credentials = true
device-ca-cert = "/path/to/toml-owner.device.ca"
owner-key = "/path/to/toml-owner.key"
`,
			expected: expectedOwnerConfig{
				address:         "127.0.0.1:8083",
				dbPath:          "test-owner-toml.db",
				dbPass:          "OwnerToml123!",
				externalAddress: "0.0.0.0:8444",
				wgets:           []string{"https://toml.example.com/file1", "https://toml.example.com/file2"},
				uploads:         []string{"toml-upload1.txt", "toml-upload2.txt"},
				uploadDir:       "/tmp/toml-uploads",
				downloads:       []string{"toml-download1.txt"},
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

			if address != tt.expected.address {
				t.Fatalf("address=%q, want %q", address, tt.expected.address)
			}
			if dbPath != tt.expected.dbPath || dbPass != tt.expected.dbPass {
				t.Fatalf("db not loaded: path=%q pass=%q, want path=%q pass=%q", dbPath, dbPass, tt.expected.dbPath, tt.expected.dbPass)
			}
			if !insecureTLS || !debug || !date || !reuseCred {
				t.Fatalf("expected booleans true: insecureTLS=%v debug=%v date=%v reuseCred=%v", insecureTLS, debug, date, reuseCred)
			}
			if externalAddress != tt.expected.externalAddress {
				t.Fatalf("externalAddress=%q, want %q", externalAddress, tt.expected.externalAddress)
			}
			if got := wgets; !reflect.DeepEqual(got, tt.expected.wgets) {
				t.Fatalf("wgets=%v, want %v", got, tt.expected.wgets)
			}
			if got := uploads; !reflect.DeepEqual(got, tt.expected.uploads) {
				t.Fatalf("uploads=%v, want %v", got, tt.expected.uploads)
			}
			if uploadDir != tt.expected.uploadDir {
				t.Fatalf("uploadDir=%q, want %q", uploadDir, tt.expected.uploadDir)
			}
			if got := downloads; !reflect.DeepEqual(got, tt.expected.downloads) {
				t.Fatalf("downloads=%v, want %v", got, tt.expected.downloads)
			}
			if ownerDeviceCACert != tt.expected.deviceCACert {
				t.Fatalf("ownerDeviceCACert=%q, want %q", ownerDeviceCACert, tt.expected.deviceCACert)
			}
			if ownerPrivateKey != tt.expected.ownerKey {
				t.Fatalf("ownerPrivateKey=%q, want %q", ownerPrivateKey, tt.expected.ownerKey)
			}
		})
	}
}

func TestRendezvous_LoadsFromTOMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
address = "127.0.0.1:8083"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8083" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test.db" || dbPass != "Abcdef1!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
	}
}

func TestManufacturing_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
address = "1.2.3.4:1111"
db = "test.db"
db-pass = "Abcdef1!"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path, "127.0.0.1:9090"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:9090" {
		t.Fatalf("expected positional address override, got %q", address)
	}
}

func TestOwner_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
address = "1.2.3.4:1111"
db = "test.db"
db-pass = "Abcdef1!"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path, "127.0.0.1:9090"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:9090" {
		t.Fatalf("expected positional address override, got %q", address)
	}
	if externalAddress != address {
		t.Fatalf("externalAddress default mismatch: got %q want %q", externalAddress, address)
	}
}

func TestRendezvous_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
address = "1.2.3.4:1111"
db = "test.db"
db-pass = "Abcdef1!"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path, "127.0.0.1:9090"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:9090" {
		t.Fatalf("expected positional address override, got %q", address)
	}
}

func TestManufacturing_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
db = "test.db"
db-pass = "Abcdef1!"
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
db = "test.db"
db-pass = "Abcdef1!"
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
db = "test.db"
db-pass = "Abcdef1!"
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
address: "127.0.0.1:8081"
db: "test-yaml.db"
db-pass: "YamlPass123!"
debug: true
insecure-tls: true
manufacturing-key: "/path/to/yaml-mfg.key"
device-ca-cert: "/path/to/yaml-device.ca"
device-ca-key: "/path/to/yaml-device.key"
owner-cert: "/path/to/yaml-owner.crt"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8081" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test-yaml.db" || dbPass != "YamlPass123!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
	}
	if manufacturerKeyPath != "/path/to/yaml-mfg.key" {
		t.Fatalf("manufacturerKeyPath=%q", manufacturerKeyPath)
	}
	if deviceCACertPath != "/path/to/yaml-device.ca" {
		t.Fatalf("deviceCACertPath=%q", deviceCACertPath)
	}
	if deviceCAKeyPath != "/path/to/yaml-device.key" {
		t.Fatalf("deviceCAKeyPath=%q", deviceCAKeyPath)
	}
	if ownerPublicKeyPath != "/path/to/yaml-owner.crt" {
		t.Fatalf("ownerPublicKeyPath=%q", ownerPublicKeyPath)
	}
}

func TestOwner_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
address: "127.0.0.1:8082"
db: "test-owner-yaml.db"
db-pass: "OwnerYaml123!"
debug: true
insecure-tls: true
external-address: "0.0.0.0:8443"
command-date: true
command-wget: ["https://yaml.example.com/file1", "https://yaml.example.com/file2"]
command-upload: ["yaml-upload1.txt", "yaml-upload2.txt"]
upload-directory: "/tmp/yaml-uploads"
command-download: ["yaml-download1.txt"]
reuse-credentials: true
device-ca-cert: "/path/to/yaml-owner.device.ca"
owner-key: "/path/to/yaml-owner.key"
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8082" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test-owner-yaml.db" || dbPass != "OwnerYaml123!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug || !date || !reuseCred {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v date=%v reuseCred=%v", insecureTLS, debug, date, reuseCred)
	}
	if externalAddress != "0.0.0.0:8443" {
		t.Fatalf("externalAddress=%q", externalAddress)
	}
	if got := wgets; !reflect.DeepEqual(got, []string{"https://yaml.example.com/file1", "https://yaml.example.com/file2"}) {
		t.Fatalf("wgets=%v", got)
	}
	if got := uploads; !reflect.DeepEqual(got, []string{"yaml-upload1.txt", "yaml-upload2.txt"}) {
		t.Fatalf("uploads=%v", got)
	}
	if uploadDir != "/tmp/yaml-uploads" {
		t.Fatalf("uploadDir=%q", uploadDir)
	}
	if got := downloads; !reflect.DeepEqual(got, []string{"yaml-download1.txt"}) {
		t.Fatalf("downloads=%v", got)
	}
	if ownerDeviceCACert != "/path/to/yaml-owner.device.ca" {
		t.Fatalf("ownerDeviceCACert=%q", ownerDeviceCACert)
	}
	if ownerPrivateKey != "/path/to/yaml-owner.key" {
		t.Fatalf("ownerPrivateKey=%q", ownerPrivateKey)
	}
}

func TestRendezvous_LoadsFromYAMLConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
address: "127.0.0.1:8083"
db: "test-rendezvous-yaml.db"
db-pass: "RendezvousYaml123!"
debug: true
insecure-tls: true
`
	path := writeYAMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8083" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test-rendezvous-yaml.db" || dbPass != "RendezvousYaml123!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
	}
}

func TestManufacturing_IgnoresInvalidConfigOptions(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	// Include owner-specific options that should be ignored by manufacturing server
	cfg := `
address = "127.0.0.1:8081"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
manufacturing-key = "/path/to/mfg.key"
device-ca-cert = "/path/to/device.ca"
device-ca-key = "/path/to/device.key"
owner-cert = "/path/to/owner.crt"
# These should be ignored by manufacturing server
command-wget = ["https://example.com/file"]
command-upload = ["upload.txt"]
command-download = ["download.txt"]
upload-directory = "/tmp/uploads"
reuse-credentials = true
owner-key = "/path/to/owner.key"
external-address = "0.0.0.0:8443"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Verify that manufacturing-specific options are loaded correctly
	if address != "127.0.0.1:8081" {
		t.Fatalf("address=%q", address)
	}
	if manufacturerKeyPath != "/path/to/mfg.key" {
		t.Fatalf("manufacturerKeyPath=%q", manufacturerKeyPath)
	}
	if deviceCACertPath != "/path/to/device.ca" {
		t.Fatalf("deviceCACertPath=%q", deviceCACertPath)
	}
	if deviceCAKeyPath != "/path/to/device.key" {
		t.Fatalf("deviceCAKeyPath=%q", deviceCAKeyPath)
	}
	if ownerPublicKeyPath != "/path/to/owner.crt" {
		t.Fatalf("ownerPublicKeyPath=%q", ownerPublicKeyPath)
	}

	// Verify that owner-specific options are NOT loaded (should remain at default values)
	if len(wgets) != 0 {
		t.Fatalf("wgets should be empty for manufacturing server, got %v", wgets)
	}
	if len(uploads) != 0 {
		t.Fatalf("uploads should be empty for manufacturing server, got %v", uploads)
	}
	if len(downloads) != 0 {
		t.Fatalf("downloads should be empty for manufacturing server, got %v", downloads)
	}
	if uploadDir != "" {
		t.Fatalf("uploadDir should be empty for manufacturing server, got %q", uploadDir)
	}
	if reuseCred {
		t.Fatalf("reuseCred should be false for manufacturing server, got %v", reuseCred)
	}
	if ownerPrivateKey != "" {
		t.Fatalf("ownerPrivateKey should be empty for manufacturing server, got %q", ownerPrivateKey)
	}
	if externalAddress != "" {
		t.Fatalf("externalAddress should be empty for manufacturing server, got %q", externalAddress)
	}
}

func TestOwner_IgnoresInvalidConfigOptions(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	// Include manufacturing-specific options that should be ignored by owner server
	cfg := `
address = "127.0.0.1:8082"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
external-address = "0.0.0.0:8443"
command-date = true
command-wget = ["https://a/x", "https://b/y"]
command-upload = ["a.txt", "b.txt"]
upload-directory = "/tmp/uploads"
command-download = ["c.txt"]
reuse-credentials = true
device-ca-cert = "/path/to/owner.device.ca"
owner-key = "/path/to/owner.key"
# These should be ignored by owner server
manufacturing-key = "/path/to/mfg.key"
device-ca-key = "/path/to/device.key"
owner-cert = "/path/to/owner.crt"
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Verify that owner-specific options are loaded correctly
	if address != "127.0.0.1:8082" {
		t.Fatalf("address=%q", address)
	}
	if externalAddress != "0.0.0.0:8443" {
		t.Fatalf("externalAddress=%q", externalAddress)
	}
	if !date {
		t.Fatalf("date should be true, got %v", date)
	}
	if got := wgets; !reflect.DeepEqual(got, []string{"https://a/x", "https://b/y"}) {
		t.Fatalf("wgets=%v", got)
	}
	if got := uploads; !reflect.DeepEqual(got, []string{"a.txt", "b.txt"}) {
		t.Fatalf("uploads=%v", got)
	}
	if uploadDir != "/tmp/uploads" {
		t.Fatalf("uploadDir=%q", uploadDir)
	}
	if got := downloads; !reflect.DeepEqual(got, []string{"c.txt"}) {
		t.Fatalf("downloads=%v", got)
	}
	if !reuseCred {
		t.Fatalf("reuseCred should be true, got %v", reuseCred)
	}
	if ownerDeviceCACert != "/path/to/owner.device.ca" {
		t.Fatalf("ownerDeviceCACert=%q", ownerDeviceCACert)
	}
	if ownerPrivateKey != "/path/to/owner.key" {
		t.Fatalf("ownerPrivateKey=%q", ownerPrivateKey)
	}

	// Verify that manufacturing-specific options are NOT loaded (should remain at default values)
	if manufacturerKeyPath != "" {
		t.Fatalf("manufacturerKeyPath should be empty for owner server, got %q", manufacturerKeyPath)
	}
	if deviceCAKeyPath != "" {
		t.Fatalf("deviceCAKeyPath should be empty for owner server, got %q", deviceCAKeyPath)
	}
	if ownerPublicKeyPath != "" {
		t.Fatalf("ownerPublicKeyPath should be empty for owner server, got %q", ownerPublicKeyPath)
	}
}

func TestRendezvous_IgnoresInvalidConfigOptions(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	// Include manufacturing and owner-specific options that should be ignored by rendezvous server
	cfg := `
address = "127.0.0.1:8083"
db = "test.db"
db-pass = "Abcdef1!"
debug = true
insecure-tls = true
# These should be ignored by rendezvous server
manufacturing-key = "/path/to/mfg.key"
device-ca-cert = "/path/to/device.ca"
device-ca-key = "/path/to/device.key"
owner-cert = "/path/to/owner.crt"
command-wget = ["https://example.com/file"]
command-upload = ["upload.txt"]
command-download = ["download.txt"]
upload-directory = "/tmp/uploads"
reuse-credentials = true
owner-key = "/path/to/owner.key"
external-address = "0.0.0.0:8443"
command-date = true
`
	path := writeTOMLConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Verify that rendezvous-specific options are loaded correctly
	if address != "127.0.0.1:8083" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test.db" || dbPass != "Abcdef1!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
	}

	// Verify that manufacturing-specific options are NOT loaded (should remain at default values)
	if manufacturerKeyPath != "" {
		t.Fatalf("manufacturerKeyPath should be empty for rendezvous server, got %q", manufacturerKeyPath)
	}
	if deviceCACertPath != "" {
		t.Fatalf("deviceCACertPath should be empty for rendezvous server, got %q", deviceCACertPath)
	}
	if deviceCAKeyPath != "" {
		t.Fatalf("deviceCAKeyPath should be empty for rendezvous server, got %q", deviceCAKeyPath)
	}
	if ownerPublicKeyPath != "" {
		t.Fatalf("ownerPublicKeyPath should be empty for rendezvous server, got %q", ownerPublicKeyPath)
	}

	// Verify that owner-specific options are NOT loaded (should remain at default values)
	if len(wgets) != 0 {
		t.Fatalf("wgets should be empty for rendezvous server, got %v", wgets)
	}
	if len(uploads) != 0 {
		t.Fatalf("uploads should be empty for rendezvous server, got %v", uploads)
	}
	if len(downloads) != 0 {
		t.Fatalf("downloads should be empty for rendezvous server, got %v", downloads)
	}
	if uploadDir != "" {
		t.Fatalf("uploadDir should be empty for rendezvous server, got %q", uploadDir)
	}
	if reuseCred {
		t.Fatalf("reuseCred should be false for rendezvous server, got %v", reuseCred)
	}
	if ownerPrivateKey != "" {
		t.Fatalf("ownerPrivateKey should be empty for rendezvous server, got %q", ownerPrivateKey)
	}
	if externalAddress != "" {
		t.Fatalf("externalAddress should be empty for rendezvous server, got %q", externalAddress)
	}
	if date {
		t.Fatalf("date should be false for rendezvous server, got %v", date)
	}
}
