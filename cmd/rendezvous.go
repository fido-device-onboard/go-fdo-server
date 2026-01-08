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
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Rendezvous server configuration (TBD)
type RendezvousConfig struct {
}

// Rendezvous server configuration file structure
type RendezvousServerConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	Rendezvous      RendezvousConfig `mapstructure:"rendezvous"`
}

// validate checks that required configuration is present
func (rv *RendezvousServerConfig) validate() error {
	if err := rv.HTTP.validate(); err != nil {
		return err
	}
	return nil
}

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	RunE: func(cmd *cobra.Command, args []string) error {
		var rvConfig RendezvousServerConfig
		if err := viper.Unmarshal(&rvConfig); err != nil {
			return fmt.Errorf("failed to unmarshal rendezvous config: %w", err)
		}
		if err := rvConfig.validate(); err != nil {
			return err
		}
		return serveRendezvous(&rvConfig)
	},
}

// Server represents the HTTP server
type RendezvousServer struct {
	handler http.Handler
	config  HTTPConfig
}

// NewServer creates a new Server
func NewRendezvousServer(config HTTPConfig, handler http.Handler) *RendezvousServer {
	return &RendezvousServer{handler: handler, config: config}
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

type RendezvousServerState struct {
	DB *db.State
}

func serveRendezvous(config *RendezvousServerConfig) error {
	dbState, err := config.DB.getState()
	if err != nil {
		return err
	}

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

	// Add health endpoint directly to the main handler (not under /api/v1/)
	healthServer := health.NewServer()
	healthHandlerFunc := health.Handler(healthServer)
	httpHandler.Handle("/health", healthHandlerFunc)

	// Listen and serve
	server := NewRendezvousServer(config.HTTP, httpHandler)

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	return server.Start()
}

// Set up the rendezvous command line. Used by the unit tests to reset state between tests.
func rendezvousCmdInit() {
	rootCmd.AddCommand(rendezvousCmd)

}

func init() {
	rendezvousCmdInit()
}
