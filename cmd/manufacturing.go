// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/manufacturing"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The manufacturer server configuration
type ManufacturingConfig struct {
	ManufacturerKeyPath string `mapstructure:"key"`
}

// Manufacturer server configuration file structure
type ManufacturingServerConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	DeviceCA        DeviceCAConfig      `mapstructure:"device_ca"`
	Manufacturer    ManufacturingConfig `mapstructure:"manufacturing"`
	Owner           OwnerConfig         `mapstructure:"owner"`
}

// validateCertFile checks that a certificate file exists and returns a helpful error if not
func validateCertFile(path, name, contextLine string) error {
	if path == "" || func() bool { _, err := os.Stat(path); return err != nil }() {
		detail := ""
		if path != "" {
			detail = fmt.Sprintf(" (configured: %s)", path)
		}
		context := ""
		if contextLine != "" {
			context = contextLine + "\n"
		}
		return fmt.Errorf("%s is required%s\n%s"+
			"run 'generate-go-fdo-server-certs.sh' for single-host setup\n"+
			"see docs/user-guide/certificates.md for multi-host deployment", name, detail, context)
	}
	return nil
}

// validate checks that required configuration is present
func (m *ManufacturingServerConfig) validate() error {
	if err := m.HTTP.validate(); err != nil {
		return err
	}
	// Validate manufacturing key exists
	if err := validateCertFile(m.Manufacturer.ManufacturerKeyPath, "manufacturing key", ""); err != nil {
		return err
	}
	// Validate device CA key exists
	if err := validateCertFile(m.DeviceCA.KeyPath, "device CA key", "this key must be shared between Manufacturing and Owner servers"); err != nil {
		return err
	}
	// Validate device CA certificate exists
	if err := validateCertFile(m.DeviceCA.CertPath, "device CA certificate", "this certificate must be shared between Manufacturing and Owner servers"); err != nil {
		return err
	}
	// Validate owner certificate exists
	if err := validateCertFile(m.Owner.OwnerCertificate, "owner certificate", "this certificate must come from the Owner server deployment"); err != nil {
		return err
	}
	return nil
}

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
		var mfgConfig ManufacturingServerConfig
		if err := viper.Unmarshal(&mfgConfig); err != nil {
			return fmt.Errorf("failed to unmarshal manufacturing config: %w", err)
		}
		if err := mfgConfig.validate(); err != nil {
			return err
		}
		return serveManufacturing(&mfgConfig)
	},
}

// Server represents the HTTP server
type ManufacturingServer struct {
	handler http.Handler
	config  HTTPConfig
}

// NewServer creates a new Server
func NewManufacturingServer(config HTTPConfig, handler http.Handler) *ManufacturingServer {
	return &ManufacturingServer{handler: handler, config: config}
}

// Start starts the HTTP server
func (s *ManufacturingServer) Start() error {
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 3 * time.Second,
	}

	// Channel to listen for interrupt or terminate signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to listen for signals and gracefully shut down the server
	go func() {
		<-stop
		slog.Debug("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Debug("Server forced to shutdown:", "err", err)
		}
	}()

	// Listen and serve
	lis, err := net.Listen("tcp", s.config.ListenAddress())
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String())

	if s.config.UseTLS() {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}
		srv.TLSConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			CipherSuites: preferredCipherSuites,
		}
		err := srv.ServeTLS(lis, s.config.CertPath, s.config.KeyPath)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	err = srv.Serve(lis)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func serveManufacturing(config *ManufacturingServerConfig) error {
	// Database
	dbState, err := config.DB.getState()
	if err != nil {
		return err
	}

	// Load Certs
	mfgKey, err := parsePrivateKey(config.Manufacturer.ManufacturerKeyPath)
	if err != nil {
		return err
	}
	deviceKey, err := parsePrivateKey(config.DeviceCA.KeyPath)
	if err != nil {
		return err
	}
	deviceCA, err := os.ReadFile(config.DeviceCA.CertPath)
	if err != nil {
		return err
	}
	blk, _ := pem.Decode(deviceCA)
	parsedDeviceCACert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return err
	}
	// TODO: chain length >1 should be supported too
	deviceCAChain := []*x509.Certificate{parsedDeviceCACert}

	// Parse
	ownerPublicKey, err := os.ReadFile(config.Owner.OwnerCertificate)
	if err != nil {
		return err
	}
	block, _ := pem.Decode([]byte(ownerPublicKey))
	if block == nil {
		return fmt.Errorf("unable to decode owner public key")
	}

	// TODO: Support PKIX public keys
	// TODO: Support certificate chains > 1
	var ownerCert *x509.Certificate
	ownerCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	// Create manufacturing handler with all dependencies
	mfgHandler := manufacturing.NewManufacturing(
		dbState,
		deviceKey,
		deviceCAChain,
		mfgKey,
		ownerCert,
		encodePublicKey,
	)
	if err = mfgHandler.InitDB(); err != nil {
		slog.Error("failed to initialize Manufacturing database", "err", err)
		return fmt.Errorf("failed to initialize Manufacturing database: %w", err)
	}
	slog.Info("Database initialized successfully", "type", config.DB.Type)
	handler := mfgHandler.Handler()

	// Listen and serve
	server := NewManufacturingServer(config.HTTP, handler)

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	return server.Start()
}

func encodePublicKey(keyType protocol.KeyType, keyEncoding protocol.KeyEncoding, pub crypto.PublicKey, chain []*x509.Certificate) (*protocol.PublicKey, error) {
	if pub == nil && len(chain) > 0 {
		pub = chain[0].PublicKey
	}
	if pub == nil {
		return nil, fmt.Errorf("no key to encode")
	}

	switch keyEncoding {
	case protocol.X509KeyEnc, protocol.CoseKeyEnc:
		// Intentionally panic if pub is not the correct key type
		switch keyType {
		case protocol.Secp256r1KeyType, protocol.Secp384r1KeyType:
			return protocol.NewPublicKey(keyType, pub.(*ecdsa.PublicKey), keyEncoding == protocol.CoseKeyEnc)
		case protocol.Rsa2048RestrKeyType, protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
			return protocol.NewPublicKey(keyType, pub.(*rsa.PublicKey), keyEncoding == protocol.CoseKeyEnc)
		default:
			return nil, fmt.Errorf("unsupported key type: %s", keyType)
		}
	case protocol.X5ChainKeyEnc:
		return protocol.NewPublicKey(keyType, chain, false)
	default:
		return nil, fmt.Errorf("unsupported key encoding: %s", keyEncoding)
	}
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
