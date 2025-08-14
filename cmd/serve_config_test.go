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
	_ = viper.BindPFlags(serveCmd.Flags())

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
	configFilePath = ""

	dbPath = ""
	dbPass = ""
	debug = false

	rootCmd.SetArgs(nil)
}

func stubRunE(t *testing.T) {
	t.Helper()
	orig := serveCmd.RunE
	serveCmd.RunE = func(*cobra.Command, []string) error { return nil }
	t.Cleanup(func() { serveCmd.RunE = orig })
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

func TestServe_LoadsFromConfigOnly(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
address: "127.0.0.1:8081"
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
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"serve", "--config", path})

	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:8081" {
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
}

func TestServe_PositionalArgOverridesAddressInConfig(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
address: "1.2.3.4:1111"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"serve", "--config", path, "127.0.0.1:9090"})

	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if address != "127.0.0.1:9090" {
		t.Fatalf("expected positional address override, got %q", address)
	}
	if externalAddress != address {
		t.Fatalf("externalAddress default mismatch: got %q want %q", externalAddress, address)
	}
}

func TestServe_FlagOverridesConfigBoolAndArray(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
address: "127.0.0.1:8081"
db: "test.db"
db-pass: "Abcdef1!"
insecure-tls: false
command-wget: ["https://cfg/a", "https://cfg/b"]
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{
		"serve",
		"--config", path,
		"--insecure-tls", "true",
		"--command-wget", "https://cli/x",
		"--command-wget", "https://cli/y",
	})

	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !insecureTLS {
		t.Fatalf("flag override for insecure-tls failed")
	}
	if want := []string{"https://cli/x", "https://cli/y"}; !reflect.DeepEqual(wgets, want) {
		t.Fatalf("wgets override failed: %v", wgets)
	}
}

func TestServe_ErrorWhenNoAddress(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"serve", "--config", path})

	if _, err := rootCmd.ExecuteC(); err == nil {
		t.Fatalf("expected error for missing address")
	}
}

func TestServe_ErrorForInvalidConfigPath(t *testing.T) {
	resetState(t)
	stubRunE(t)

	rootCmd.SetArgs([]string{"serve", "--config", "/no/such/file.yaml"})

	if _, err := rootCmd.ExecuteC(); err == nil {
		t.Fatalf("expected error reading config file")
	}
}

func TestServe_ExternalAddress_DefaultsToAddress_FromConfig(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
address: "127.0.0.1:8081"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{"serve", "--config", path})

	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if externalAddress != address {
		t.Fatalf("externalAddress default mismatch: got %q want %q", externalAddress, address)
	}
}

func TestServe_ExternalAddress_FlagOverridesConfig(t *testing.T) {
	resetState(t)
	stubRunE(t)

	cfg := `
address: "127.0.0.1:8081"
external-address: "0.0.0.0:8443"
db: "test.db"
db-pass: "Abcdef1!"
`
	path := writeConfig(t, cfg)
	rootCmd.SetArgs([]string{
		"serve",
		"--config", path,
		"--external-address", "10.0.0.1:9443",
	})

	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if externalAddress != "10.0.0.1:9443" {
		t.Fatalf("externalAddress flag override failed: %q", externalAddress)
	}
}
