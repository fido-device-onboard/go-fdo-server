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

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeJSONConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManufacturing_LoadsFromConfigOnly(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `
address: "127.0.0.1:8081"
db: "test.db"
db-pass: "Abcdef1!"
debug: true
insecure-tls: true
manufacturing-key: "/path/to/mfg.key"
device-ca-cert: "/path/to/device.ca"
device-ca-key: "/path/to/device.key"
owner-cert: "/path/to/owner.crt"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8081" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test.db" || dbPass != "Abcdef1!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
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
}

func TestManufacturing_LoadsFromJSONConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	cfg := `{
  "address": "127.0.0.1:8082",
  "db": "test-json.db",
  "db-pass": "JsonPass123!",
  "debug": true,
  "insecure-tls": true,
  "manufacturing-key": "/path/to/json-mfg.key",
  "device-ca-cert": "/path/to/json-device.ca",
  "device-ca-key": "/path/to/json-device.key",
  "owner-cert": "/path/to/json-owner.crt"
}`
	path := writeJSONConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8082" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test-json.db" || dbPass != "JsonPass123!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v", insecureTLS, debug)
	}
	if manufacturerKeyPath != "/path/to/json-mfg.key" {
		t.Fatalf("manufacturerKeyPath=%q", manufacturerKeyPath)
	}
	if deviceCACertPath != "/path/to/json-device.ca" {
		t.Fatalf("deviceCACertPath=%q", deviceCACertPath)
	}
	if deviceCAKeyPath != "/path/to/json-device.key" {
		t.Fatalf("deviceCAKeyPath=%q", deviceCAKeyPath)
	}
	if ownerPublicKeyPath != "/path/to/json-owner.crt" {
		t.Fatalf("ownerPublicKeyPath=%q", ownerPublicKeyPath)
	}
}

func TestOwner_LoadsFromConfigOnly(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
address: "127.0.0.1:8082"
db: "test.db"
db-pass: "Abcdef1!"
debug: true
insecure-tls: true
external-address: "0.0.0.0:8443"
command-date: true
command-wget: ["https://a/x", "https://b/y"]
command-upload: ["a.txt", "b.txt"]
upload-directory: "/tmp/uploads"
command-download: ["c.txt"]
reuse-credentials: true
device-ca-cert: "/path/to/owner.device.ca"
owner-key: "/path/to/owner.key"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8082" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test.db" || dbPass != "Abcdef1!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug || !date || !reuseCred {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v date=%v reuseCred=%v", insecureTLS, debug, date, reuseCred)
	}
	if externalAddress != "0.0.0.0:8443" {
		t.Fatalf("externalAddress=%q", externalAddress)
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
	if ownerDeviceCACert != "/path/to/owner.device.ca" {
		t.Fatalf("ownerDeviceCACert=%q", ownerDeviceCACert)
	}
	if ownerPrivateKey != "/path/to/owner.key" {
		t.Fatalf("ownerPrivateKey=%q", ownerPrivateKey)
	}
}

func TestOwner_LoadsFromJSONConfig(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `{
  "address": "127.0.0.1:8083",
  "db": "test-owner-json.db",
  "db-pass": "OwnerJson123!",
  "debug": true,
  "insecure-tls": true,
  "external-address": "0.0.0.0:8444",
  "command-date": true,
  "command-wget": ["https://json.example.com/file1", "https://json.example.com/file2"],
  "command-upload": ["json-upload1.txt", "json-upload2.txt"],
  "upload-directory": "/tmp/json-uploads",
  "command-download": ["json-download1.txt"],
  "reuse-credentials": true,
  "device-ca-cert": "/path/to/json-owner.device.ca",
  "owner-key": "/path/to/json-owner.key"
}`
	path := writeJSONConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8083" {
		t.Fatalf("address=%q", address)
	}
	if dbPath != "test-owner-json.db" || dbPass != "OwnerJson123!" {
		t.Fatalf("db not loaded: path=%q pass=%q", dbPath, dbPass)
	}
	if !insecureTLS || !debug || !date || !reuseCred {
		t.Fatalf("expected booleans true: insecureTLS=%v debug=%v date=%v reuseCred=%v", insecureTLS, debug, date, reuseCred)
	}
	if externalAddress != "0.0.0.0:8444" {
		t.Fatalf("externalAddress=%q", externalAddress)
	}
	if got := wgets; !reflect.DeepEqual(got, []string{"https://json.example.com/file1", "https://json.example.com/file2"}) {
		t.Fatalf("wgets=%v", got)
	}
	if got := uploads; !reflect.DeepEqual(got, []string{"json-upload1.txt", "json-upload2.txt"}) {
		t.Fatalf("uploads=%v", got)
	}
	if uploadDir != "/tmp/json-uploads" {
		t.Fatalf("uploadDir=%q", uploadDir)
	}
	if got := downloads; !reflect.DeepEqual(got, []string{"json-download1.txt"}) {
		t.Fatalf("downloads=%v", got)
	}
	if ownerDeviceCACert != "/path/to/json-owner.device.ca" {
		t.Fatalf("ownerDeviceCACert=%q", ownerDeviceCACert)
	}
	if ownerPrivateKey != "/path/to/json-owner.key" {
		t.Fatalf("ownerPrivateKey=%q", ownerPrivateKey)
	}
}

func TestRendezvous_LoadsFromConfigOnly(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
address: "127.0.0.1:8083"
db: "test.db"
db-pass: "Abcdef1!"
debug: true
insecure-tls: true
`
	path := writeConfig(t, cfg)
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
address: "1.2.3.4:1111"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
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
address: "1.2.3.4:1111"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
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
address: "1.2.3.4:1111"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
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
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"manufacturing", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestOwner_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	cfg := `
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"owner", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestRendezvous_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	cfg := `
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"rendezvous", "--config", path})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestManufacturing_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, manufacturingCmd)

	rootCmd.SetArgs([]string{"manufacturing", "--config", "/no/such/file.yaml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestOwner_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, ownerCmd)

	rootCmd.SetArgs([]string{"owner", "--config", "/no/such/file.yaml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestRendezvous_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t, rendezvousCmd)

	rootCmd.SetArgs([]string{"rendezvous", "--config", "/no/such/file.yaml"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}
