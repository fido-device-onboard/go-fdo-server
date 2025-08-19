/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the HTTP server for the role",
	Long: `Serve runs the HTTP server for the FDO protocol. It can act as all the three
	main servers in the FDO spec.`,
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

		return serveRendezvous(state, insecureTLS)
	},
}

// Server represents the HTTP server
type RendezvousServer struct {
	addr    string
	extAddr string
	handler http.Handler
	useTLS  bool
	state   *sqlite.DB
}

// NewServer creates a new Server
func NewRendezvousServer(addr string, extAddr string, handler http.Handler, useTLS bool, state *sqlite.DB) *RendezvousServer {
	return &RendezvousServer{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS, state: state}
}

// Start starts the HTTP server
func (s *RendezvousServer) Start() error {
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
	slog.Info("Listening", "local", lis.Addr().String(), "external", s.extAddr)

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
			cert, err := tlsCert(s.state.DB())
			if err != nil {
				return err
			}
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{*cert},
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, "", "")

		}
	}
	return srv.Serve(lis)
}

type RendezvousServerState struct {
	RvInfo [][]protocol.RvInstruction
	DB     *sqlite.DB
}

func serveRendezvous(db *sqlite.DB, useTLS bool) error {
	state := &RendezvousServerState{
		DB: db,
	}
	// Create FDO responder
	handler, err := newRendezvousHandler(state)
	if err != nil {
		return err
	}

	httpHandler := api.NewHTTPHandler(handler, nil, state.DB).RegisterRoutes(nil)

	// Listen and serve
	server := NewRendezvousServer(address, externalAddress, httpHandler, useTLS, state.DB)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

//nolint:gocyclo
func newRendezvousHandler(state *RendezvousServerState) (*transport.Handler, error) {
	// Generate manufacturing component keys
	rsa2048MfgKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	rsa3072MfgKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	ec256MfgKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ec384MfgKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	generateCA := func(key crypto.Signer) ([]*x509.Certificate, error) {
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(30 * 365 * 24 * time.Hour),
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		return []*x509.Certificate{cert}, nil
	}
	rsa2048Chain, err := generateCA(rsa2048MfgKey)
	if err != nil {
		return nil, err
	}
	rsa3072Chain, err := generateCA(rsa3072MfgKey)
	if err != nil {
		return nil, err
	}
	ec256Chain, err := generateCA(ec256MfgKey)
	if err != nil {
		return nil, err
	}
	ec384Chain, err := generateCA(ec384MfgKey)
	if err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Rsa2048RestrKeyType, rsa2048MfgKey, rsa2048Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.RsaPkcsKeyType, rsa3072MfgKey, rsa3072Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.RsaPssKeyType, rsa3072MfgKey, rsa3072Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Secp256r1KeyType, ec256MfgKey, ec256Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Secp384r1KeyType, ec384MfgKey, ec384Chain); err != nil {
		return nil, err
	}

	// Generate owner keys
	rsa2048OwnerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	rsa3072OwnerKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	ec256OwnerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ec384OwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Rsa2048RestrKeyType, rsa2048OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.RsaPkcsKeyType, rsa3072OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.RsaPssKeyType, rsa3072OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Secp256r1KeyType, ec256OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Secp384r1KeyType, ec384OwnerKey, nil); err != nil {
		return nil, err
	}

	return &transport.Handler{
		Tokens: state.DB,
		TO0Responder: &fdo.TO0Server{
			Session: state.DB,
			RVBlobs: state.DB,
		},
		TO1Responder: &fdo.TO1Server{
			Session: state.DB,
			RVBlobs: state.DB,
		}}, nil
}

func init() {
	serveCmd.AddCommand(rendezvousCmd)

	//serveCmd.Flags().StringVar(&externalAddress, "external-address", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	// serveCmd.Flags().BoolVar(&date, "command-date", false, "Use fdo.command FSIM to have device run \"date --utc\"")
	// serveCmd.Flags().StringArrayVar(&wgets, "command-wget", nil, "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")
	// serveCmd.Flags().StringArrayVar(&uploads, "command-upload", nil, "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	// serveCmd.Flags().StringVar(&uploadDir, "upload-directory", "", "The directory `path` to put file uploads")
	// serveCmd.Flags().StringArrayVar(&downloads, "command-download", nil, "Use fdo.download FSIM for each `file` (flag may be used multiple times)")
	//serveCmd.Flags().BoolVar(&reuseCred, "reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
}
