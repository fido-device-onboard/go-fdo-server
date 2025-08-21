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
	"github.com/spf13/viper"
)

// The rendezvous server configuration
type RendezvousConfig struct {
	HTTP HTTPConfig     `mapstructure:"http"`
	DB   DatabaseConfig `mapstructure:"database"`
}

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			viper.Set("rendezvous.http.listen", args[0])
		}

		var fdoConfig FIDOServerConfig
		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return fmt.Errorf("failed to unmarshal rendezvous config: %w", err)
		}

		return serveRendezvous(fdoConfig.Rendezvous)
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

type RendezvousServerState struct {
	DB *db.State
}

func serveRendezvous(config *RendezvousConfig) error {
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

	// Listen and serve
	server := NewRendezvousServer(config.HTTP, httpHandler)

	slog.Debug("Starting server on:", "addr", config.HTTP.Listen)
	return server.Start()
}

// Set up the rendezvous command line. Used by the unit tests to reset state between tests.
func rendezvousCmdInit() {
	rootCmd.AddCommand(rendezvousCmd)

	if err := addDatabaseConfig(rendezvousCmd, "rendezvous.database"); err != nil {
		panic(err)
	}
	if err := addHTTPConfig(rendezvousCmd, "rendezvous.http"); err != nil {
		panic(err)
	}
}

func init() {
	rendezvousCmdInit()
}
