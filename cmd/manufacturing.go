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
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	address             string
	manufacturerKeyPath string
	deviceCACertPath    string
	deviceCAKeyPath     string
	ownerPublicKeyPath  string
)

// manufacturingCmd represents the manufacturing command
var manufacturingCmd = &cobra.Command{
	Use:   "manufacturing http_address",
	Short: "Serve an instance of the manufacturing server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration first
		if err := manufacturingCmdLoadConfig(cmd, args); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := getState()
		if err != nil {
			return err
		}

		err = db.InitDb(state)
		if err != nil {
			return err
		}

		return serveManufacturing(state, insecureTLS)
	},
}

// Server represents the HTTP server
type ManufacturingServer struct {
	addr    string
	handler http.Handler
	useTLS  bool
}

// NewServer creates a new Server
func NewManufacturingServer(addr string, handler http.Handler, useTLS bool) *ManufacturingServer {
	return &ManufacturingServer{addr: addr, handler: handler, useTLS: useTLS}
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
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String())

	if s.useTLS {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}

		if serverCertPath != "" && serverKeyPath != "" {
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, serverCertPath, serverKeyPath)
		} else {
			return fmt.Errorf("no TLS cert or key provided")
		}
	}
	return srv.Serve(lis)
}

func serveManufacturing(dbState *sqlite.DB, useTLS bool) error {
	mfgKey, err := parsePrivateKey(manufacturerKeyPath)
	if err != nil {
		return err
	}
	deviceKey, err := parsePrivateKey(deviceCAKeyPath)
	if err != nil {
		return err
	}
	deviceCA, err := os.ReadFile(deviceCACertPath)
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
	ownerPublicKey, err := os.ReadFile(ownerPublicKeyPath)
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
	httpHandler := api.NewHTTPHandler(handler, dbState).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewManufacturingServer(address, httpHandler, useTLS)

	slog.Debug("Starting server on:", "addr", address)
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

func init() {
	rootCmd.AddCommand(manufacturingCmd)

	manufacturingCmd.Flags().String("manufacturing-key", "", "Manufacturing private key path")
	manufacturingCmd.Flags().String("device-ca-cert", "", "Device certificate path")
	manufacturingCmd.Flags().String("owner-cert", "", "Owner certificate path")
	manufacturingCmd.Flags().String("device-ca-key", "", "Device CA private key path")
	manufacturingCmd.Flags().String("config", "", "Pathname of the configuration file")
}

// Load configuration from viper
func manufacturingCmdLoadConfig(cmd *cobra.Command, args []string) error {
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
	}

	// If the http_address has been provided on the command line it
	// will take precedence over the content of the configuration
	// file. Yet viper has no visibility of the command line so we
	// force it here:
	if len(args) > 0 {
		viper.Set("address", args[0])
	}

	// Get the config flag directly from the CLI
	configFilePath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("failed to get config flag: %w", err)
	}

	if configFilePath != "" {
		slog.Debug("Loading manufacturing server configuration file", "path", configFilePath)
		viper.SetConfigFile(configFilePath)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("configuration file read failed: %w", err)
		}
	}

	// We can now load the root configuration after reading config
	// file
	if err := rootCmdLoadConfig(); err != nil {
		return err
	}

	manufacturerKeyPath = viper.GetString("manufacturing-key")
	deviceCACertPath = viper.GetString("device-ca-cert")
	ownerPublicKeyPath = viper.GetString("owner-cert")
	deviceCAKeyPath = viper.GetString("device-ca-key")
	address = viper.GetString("address")

	if address == "" {
		return fmt.Errorf("the manufacturing command requires the 'http_address' argument")
	}

	return nil
}
