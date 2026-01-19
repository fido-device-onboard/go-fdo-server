// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/rendezvous"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RendezvousConfig server configuration
type RendezvousConfig struct {
	// MinTTL is the minimum time-to-live in seconds for rendezvous entries.
	// If an owner server requests a TTL lower than this value, this minimum will be used instead.
	// Default: 4294967295 (maximum uint32, effectively no minimum)
	MinTTL uint32 `mapstructure:"min_ttl"`
}

// RendezvousServerConfig server configuration file structure
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
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Bind the min-ttl flag
		if err := viper.BindPFlag("rendezvous.min_ttl", cmd.Flags().Lookup("min-ttl")); err != nil {
			return err
		}
		return nil
	},
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

// RendezvousServer represents the HTTP server
type RendezvousServer struct {
	handler http.Handler
	config  HTTPConfig
}

// NewRendezvousServer creates a new Server
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
		err = srv.ServeTLS(lis, s.config.CertPath, s.config.KeyPath)
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

func serveRendezvous(config *RendezvousServerConfig) error {
	dbState, err := config.DB.getState()
	if err != nil {
		return err
	}

	// Set default MinTTL if not configured
	minTTL := config.Rendezvous.MinTTL
	if minTTL == 0 {
		minTTL = math.MaxUint32
	}

	rendezvous := rendezvous.NewRendezvous(dbState, minTTL)
	handler := rendezvous.Handler()

	// Listen and serve
	server := NewRendezvousServer(config.HTTP, handler)

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	return server.Start()
}

// Set up the rendezvous command line. Used by the unit tests to reset state between tests.
func rendezvousCmdInit() {
	rootCmd.AddCommand(rendezvousCmd)
	rendezvousCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	rendezvousCmd.Flags().Uint32("min-ttl", math.MaxUint32, "Minimum TTL in seconds for rendezvous entries (default: no minimum)")
}

func init() {
	rendezvousCmdInit()
}
