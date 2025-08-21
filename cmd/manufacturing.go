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

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/custom"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type DeviceCACertConfig struct {
	CertPath string `mapstructure:"cert"` // path to certificate file
	KeyPath  string `mapstructure:"key"`  // path to key file
}

// The manufacturer server configuration
type ManufacturingConfig struct {
	ManufacturerKeyPath string             `mapstructure:"private-key"`
	OwnerPublicKeyPath  string             `mapstructure:"owner-cert"`
	HTTP                HTTPConfig         `mapstructure:"http"`
	DB                  DatabaseConfig     `mapstructure:"database"`
	DeviceCACert        DeviceCACertConfig `mapstructure:"device-ca"`
}

// manufacturingCmd represents the manufacturing command
var manufacturingCmd = &cobra.Command{
	Use:   "manufacturing http_address",
	Short: "Serve an instance of the manufacturing server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			viper.Set("manufacturing.http.listen", args[0])
		}

		var fdoConfig FIDOServerConfig
		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return fmt.Errorf("failed to unmarshal manufacturing config: %w", err)
		}

		return serveManufacturing(fdoConfig.Manufacturing)
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
	err := s.config.validate()
	if err != nil {
		return err
	}
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
	lis, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String())

	if s.config.UseTLS {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}

		if s.config.CertPath != "" && s.config.KeyPath != "" {
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, s.config.CertPath, s.config.KeyPath)
		} else {
			return fmt.Errorf("no TLS cert or key provided")
		}
	}
	return srv.Serve(lis)
}

func serveManufacturing(config *ManufacturingConfig) error {
	// Database
	dbState, err := config.DB.getState()
	if err != nil {
		return err
	}

	// Load Certs
	mfgKey, err := parsePrivateKey(config.ManufacturerKeyPath)
	if err != nil {
		return err
	}
	deviceKey, err := parsePrivateKey(config.DeviceCACert.KeyPath)
	if err != nil {
		return err
	}
	deviceCA, err := os.ReadFile(config.DeviceCACert.CertPath)
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
	ownerPublicKey, err := os.ReadFile(config.OwnerPublicKeyPath)
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

	// Create FDO responder
	handler := &transport.Handler{
		Tokens: dbState,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               dbState,
			Vouchers:              dbState,
			SignDeviceCertificate: custom.SignDeviceCertificate(deviceKey, deviceCAChain),
			DeviceInfo: func(ctx context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.PublicKey, error) {
				// TODO: Parse manufacturer key chain (different than device CA chain)
				mfgPubKey, err := encodePublicKey(info.KeyType, info.KeyEncoding, mfgKey.Public(), nil)
				if err != nil {
					return "", protocol.PublicKey{}, err
				}
				return info.DeviceInfo, *mfgPubKey, nil
			},
			BeforeVoucherPersist: func(ctx context.Context, ov *fdo.Voucher) error {
				extended, err := fdo.ExtendVoucher(ov, mfgKey, []*x509.Certificate{ownerCert}, nil)
				if err != nil {
					return err
				}
				*ov = *extended
				return nil
			},
			RvInfo: func(context.Context, *fdo.Voucher) ([][]protocol.RvInstruction, error) {
				return db.FetchRvInfo()
			},
		},
	}

	// Handle messages
	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc("GET /vouchers", handlers.GetVoucherHandler)
	apiRouter.HandleFunc("GET /vouchers/{guid}", handlers.GetVoucherByGUIDHandler)
	apiRouter.Handle("/rvinfo", handlers.RvInfoHandler())
	httpHandler := api.NewHTTPHandler(handler, dbState.DB).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewManufacturingServer(config.HTTP, httpHandler)

	slog.Debug("Starting server on:", "addr", config.HTTP.Listen)
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

	// Declare any CLI flags for overriding configuration file settings and bind
	// them into the proper fields of the configuration structure

	manufacturingCmd.Flags().String("manufacturing-key", "", "Manufacturing private key path")
	manufacturingCmd.Flags().String("owner-cert", "", "Owner certificate path")
	manufacturingCmd.Flags().String("device-ca-cert", "", "Device certificate path")
	manufacturingCmd.Flags().String("device-ca-key", "", "Device CA private key path")
	if err := viper.BindPFlag("manufacturing.private-key", manufacturingCmd.Flags().Lookup("manufacturing-key")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("manufacturing.owner-cert", manufacturingCmd.Flags().Lookup("owner-cert")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("manufacturing.device-ca.cert", manufacturingCmd.Flags().Lookup("device-ca-cert")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("manufacturing.device-ca.key", manufacturingCmd.Flags().Lookup("device-ca-key")); err != nil {
		panic(err)
	}
	if err := addDatabaseConfig(manufacturingCmd, "manufacturing.database"); err != nil {
		panic(err)
	}
	if err := addHTTPConfig(manufacturingCmd, "manufacturing.http"); err != nil {
		panic(err)
	}
}

func init() {
	manufacturingCmdInit()
}
