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
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo/custom"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
)

var (
	address          string
	manufacturingKey string
	deviceCACert     string
	deviceCAKey      string
	ownerPKey        string
)

// serveCmd represents the serve command
var manufacturingCmd = &cobra.Command{
	Use:   "manufacturing http_address",
	Short: "Serve an instance of the manufacturing server",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		address = args[0]
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

		// Retrieve RV info from DB
		rvInfo, err := rvinfo.FetchRvInfo()
		if err != nil {
			return err
		}

		return serveManufacturing(rvInfo, state, insecureTLS)
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

func getSingleOwnerManufacturerState() (*SingleOwnerManufacturer, error) {
	mfgKey, err := parsePrivateKey(manufacturingKey)
	if err != nil {
		return nil, err
	}
	mfgKeyType, err := getPrivateKeyType(mfgKey)
	if err != nil {
		return nil, err
	}
	deviceKey, err := parsePrivateKey(deviceCAKey)
	if err != nil {
		return nil, err
	}
	deviceCAKeyType, err := getPrivateKeyType(deviceKey)
	if err != nil {
		return nil, err
	}
	deviceCA, err := os.ReadFile(deviceCACert)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(deviceCA)
	parsedDeviceCACert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}
	ownerPublicKey, err := os.ReadFile(ownerPKey)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(ownerPublicKey))
	if block == nil {
		return nil, fmt.Errorf("unable to decode owner public key")
	}
	// in the future we need to also accept owner public keys directly
	var ownerCert *x509.Certificate
	ownerCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &SingleOwnerManufacturer{
		ownerKey:        ownerCert.PublicKey.(crypto.PublicKey),
		// TODO: chain length >1 should be supported too
		chain:           []*x509.Certificate{parsedDeviceCACert},
		mfgKey:          mfgKey,
		mfgKeyType:      mfgKeyType,
		deviceCAKey:     deviceKey,
		deviceCAKeyType: deviceCAKeyType,
	}, nil
}

func serveManufacturing(rvInfo [][]protocol.RvInstruction, db *sqlite.DB, useTLS bool) error {
	state, err := getSingleOwnerManufacturerState()
	if err != nil {
		return err
	}

	// Create FDO responder
	handler := &transport.Handler{
		Tokens: db,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               db,
			Vouchers:              db,
			SignDeviceCertificate: custom.SignDeviceCertificate(state),
			DeviceInfo: func(_ context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.KeyType, protocol.KeyEncoding, error) {
				return info.DeviceInfo, info.KeyType, info.KeyEncoding, nil
			},
			BeforeVoucherPersist: state.Extend,
			RvInfo:               func(context.Context, *fdo.Voucher) ([][]protocol.RvInstruction, error) { return rvinfo.FetchRvInfo() },
		},
	}

	// Handle messages
	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc("GET /vouchers", handlers.GetVoucherHandler)
	apiRouter.Handle("/rvinfo", handlers.RvInfoHandler(&rvInfo))
	httpHandler := api.NewHTTPHandler(handler, db).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewManufacturingServer(address, httpHandler, useTLS)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

type SingleOwnerManufacturer struct {
	ownerKey        crypto.PublicKey
	chain           []*x509.Certificate
	mfgKey          crypto.Signer
	mfgKeyType      protocol.KeyType
	deviceCAKey     crypto.Signer
	deviceCAKeyType protocol.KeyType
}

func (state *SingleOwnerManufacturer) ManufacturerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	return state.deviceCAKey, state.chain, nil
}

func (state *SingleOwnerManufacturer) Extend(ctx context.Context, ov *fdo.Voucher) error {
	mfgKey := ov.Header.Val.ManufacturerKey
	keyType, rsaBits := mfgKey.Type, mfgKey.RsaBits()
	// TODO: change this to use the mfg key instead of the device ca key
	if keyType != state.deviceCAKeyType {
		return fmt.Errorf("auto extend: invalid key type %T", state.deviceCAKey)
	}
	if state.deviceCAKeyType == protocol.Rsa2048RestrKeyType {
		if rsaBits != 2048 {
			return fmt.Errorf("auto extend: invalid rsa bits %d", rsaBits)
		}
	}
	switch state.ownerKey.(type) {
	case *ecdsa.PublicKey:
		nextOwner, ok := state.ownerKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("auto extend: owner key must be %s", keyType)
		}
		// TODO: change this to use the mfg key instead of the device ca key
		extended, err := fdo.ExtendVoucher(ov, state.deviceCAKey, nextOwner, nil)
		if err != nil {
			return err
		}
		*ov = *extended
		return nil
	case *rsa.PublicKey:
		nextOwner, ok := state.ownerKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("auto extend: owner key must be %s", keyType)
		}
		// TODO: change this to use the mfg key instead of the device ca key
		extended, err := fdo.ExtendVoucher(ov, state.deviceCAKey, nextOwner, nil)
		if err != nil {
			return err
		}
		*ov = *extended
		return nil
	default:
		return fmt.Errorf("auto extend: invalid key type %T", state.mfgKey)
	}
}

func init() {
	rootCmd.AddCommand(manufacturingCmd)

	manufacturingCmd.Flags().StringVar(&manufacturingKey, "manufacturing-key", "", "Manufacturing private key path")
	manufacturingCmd.Flags().StringVar(&deviceCACert, "device-ca-cert", "", "Device certificate path")
	manufacturingCmd.Flags().StringVar(&ownerPKey, "owner-cert", "", "Owner certificate path")
	manufacturingCmd.Flags().StringVar(&deviceCAKey, "device-ca-key", "", "Device CA private key path")
}
