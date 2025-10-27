// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/tls"
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
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		address = args[0]
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		dbType, dsn, err := getDBConfig()
		if err != nil {
			return err
		}

		state, err := db.InitDb(dbType, dsn)
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
}

// NewServer creates a new Server
func NewRendezvousServer(addr string, extAddr string, handler http.Handler, useTLS bool) *RendezvousServer {
	return &RendezvousServer{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS}
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
			return fmt.Errorf("no TLS cert or key provided")
		}
	}
	return srv.Serve(lis)
}

type RendezvousServerState struct {
	DB *db.State
}

func serveRendezvous(dbState *db.State, useTLS bool) error {
	state := &RendezvousServerState{
		DB: dbState,
	}
	// Create FDO responder
	handler := &transport.Handler{
		Tokens: state.DB,
		TO0Responder: &fdo.TO0Server{
			Session: state.DB,
			RVBlobs: state.DB,
		},
		TO1Responder: &fdo.TO1Server{
			Session: state.DB,
			RVBlobs: state.DB,
		}}

	httpHandler := api.NewHTTPHandler(handler, state.DB.DB).RegisterRoutes(nil)

	// Listen and serve
	server := NewRendezvousServer(address, externalAddress, httpHandler, useTLS)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

func init() {
	rootCmd.AddCommand(rendezvousCmd)
}
