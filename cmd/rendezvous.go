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
	"sync"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/rendezvous"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// defaultMinWaitSecs Default minimum wait time in seconds for TO0 rendezvous entries (requests below this are rejected)
	// Default: 0 (no minimum)
	defaultMinWaitSecs uint32 = 0

	// defaultMaxWaitSecs Default maximum wait time in seconds for TO0 rendezvous entries (requests above this are capped)
	// Default: 86400 (24h)
	defaultMaxWaitSecs uint32 = 86400

	// defaultCleanupIntervalSecs Default interval in seconds for cleaning up expired rendezvous blobs and sessions
	defaultCleanupIntervalSecs uint32 = 3600 // 1 hour

	// defaultSessionMaxAgeSecs Default maximum age in seconds for sessions before cleanup
	defaultSessionMaxAgeSecs uint32 = 3600 // 1 hour

	// defaultInitialCleanupDelaySecs Default delay before first cleanup after startup
	defaultInitialCleanupDelaySecs uint32 = 300 // 5 minutes
)

// RendezvousConfig server configuration
type RendezvousConfig struct {
	// MinWaitSecs is the minimum time in seconds the rendezvous server will accept
	// to maintain a rendezvous blob registered in the database.
	// If an owner server requests a wait time lower than this value during TO0,
	// the request will be rejected.
	// Default: 0 (no minimum)
	MinWaitSecs uint32 `mapstructure:"to0_min_wait"`

	// MaxWaitSecs is the maximum time in seconds the rendezvous server will accept
	// to maintain a rendezvous blob registered in the database.
	// If an owner server requests a wait time higher than this value during TO0,
	// the request will be accepted but capped at this maximum value.
	// Default: 86400 (24h)
	MaxWaitSecs uint32 `mapstructure:"to0_max_wait"`

	// CleanupIntervalSecs is the interval in seconds at which the server will
	// automatically cleanup expired rendezvous blobs and old sessions from the database.
	// Set to 0 to disable automatic cleanup.
	// Default: 3600 (1 hour)
	CleanupIntervalSecs uint32 `mapstructure:"cleanup_interval"`

	// SessionMaxAgeSecs is the maximum age in seconds for sessions before they are
	// considered expired and cleaned up. Sessions older than this will be deleted
	// along with their associated TO0/TO1 session data.
	// Default: 3600 (1 hour)
	SessionMaxAgeSecs uint32 `mapstructure:"session_timeout"`

	// InitialCleanupDelaySecs is the delay in seconds before the first cleanup runs after startup.
	// This prevents startup spikes when restarting servers with large amounts of expired data.
	// Default: 300 (5 minutes)
	InitialCleanupDelaySecs uint32 `mapstructure:"initial_cleanup_delay"`
}

func (rv *RendezvousConfig) validate() error {
	if rv.MinWaitSecs > rv.MaxWaitSecs {
		return fmt.Errorf("'to0_max_wait' (%d) must be greater or equal than 'to0_min_wait' (%d)", rv.MaxWaitSecs, rv.MinWaitSecs)
	}
	return nil
}

// RendezvousServerConfig server configuration file structure
type RendezvousServerConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	Rendezvous      RendezvousConfig `mapstructure:"rendezvous"`
}

// validate checks that required configuration is present
func (rv *RendezvousServerConfig) validate() error {
	slog.Debug("Validating rendezvous server configuration")
	if err := rv.HTTP.validate(); err != nil {
		slog.Error("HTTP configuration validation failed", "err", err)
		return err
	}
	if err := rv.Rendezvous.validate(); err != nil {
		slog.Error("rendezvous configuration validation failed", "err", err)
		return err
	}
	slog.Debug("Rendezvous server configuration validated successfully")

	return nil
}

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("Binding rendezvous command flags")
		// Rebind only those keys needed by the rendezvous command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		if err := viper.BindPFlag("rendezvous.to0_min_wait", cmd.Flags().Lookup("to0-min-wait")); err != nil {
			slog.Error("Failed to bind to0-min-wait flag", "err", err)
			return err
		}
		if err := viper.BindPFlag("rendezvous.to0_max_wait", cmd.Flags().Lookup("to0-max-wait")); err != nil {
			slog.Error("Failed to bind to0-max-wait flag", "err", err)
			return err
		}
		if err := viper.BindPFlag("rendezvous.cleanup_interval", cmd.Flags().Lookup("cleanup-interval")); err != nil {
			slog.Error("Failed to bind cleanup-interval flag", "err", err)
			return err
		}
		if err := viper.BindPFlag("rendezvous.session_timeout", cmd.Flags().Lookup("session-timeout")); err != nil {
			slog.Error("Failed to bind session-timeout flag", "err", err)
			return err
		}
		if err := viper.BindPFlag("rendezvous.initial_cleanup_delay", cmd.Flags().Lookup("initial-cleanup-delay")); err != nil {
			slog.Error("Failed to bind initial-cleanup-delay flag", "err", err)
			return err
		}
		slog.Debug("Flags bound successfully")
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
	slog.Info("Initializing rendezvous server")

	db, err := config.DB.getDB()
	if err != nil {
		slog.Error("Failed to get a database connection", "err", err)
		return fmt.Errorf("failed to get a database connection: %w", err)
	}

	maxWaitSecs := config.Rendezvous.MaxWaitSecs
	minWaitSecs := config.Rendezvous.MinWaitSecs
	slog.Info("TO0 wait time limits configured", "minWaitSecs", minWaitSecs, "maxWaitSecs", maxWaitSecs)

	rendezvous := rendezvous.NewRendezvous(db, minWaitSecs, maxWaitSecs)
	if err = rendezvous.InitDB(); err != nil {
		slog.Error("failed to initialize rendezvous database", "err", err)
		return fmt.Errorf("failed to initialize rendezvous database: %w", err)
	}
	slog.Info("Database initialized successfully", "type", config.DB.Type)

	// Start background cleanup (if enabled)
	// Config values are populated by viper.Unmarshal() from config file, CLI flags, or defaults
	ctx, cancel := context.WithCancel(context.Background())

	var cleanupWg sync.WaitGroup
	cleanupWg.Add(1)
	go func() {
		defer cleanupWg.Done()
		rendezvous.StartPeriodicCleanup(ctx,
			time.Duration(config.Rendezvous.CleanupIntervalSecs)*time.Second,
			time.Duration(config.Rendezvous.SessionMaxAgeSecs)*time.Second,
			time.Duration(config.Rendezvous.InitialCleanupDelaySecs)*time.Second)
	}()

	handler := rendezvous.Handler()
	// Listen and serve
	server := NewRendezvousServer(config.HTTP, handler)

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	err = server.Start()

	// Signal shutdown and wait for cleanup to finish
	cancel()
	slog.Info("Waiting for cleanup to finish...")
	cleanupWg.Wait()
	slog.Info("Cleanup finished, server shutdown complete")

	return err
}

// Set up the rendezvous command line. Used by the unit tests to reset state between tests.
func rendezvousCmdInit() {
	rootCmd.AddCommand(rendezvousCmd)
	rendezvousCmd.Flags().Uint32("to0-min-wait", defaultMinWaitSecs, "Minimum wait time in seconds for TO0 rendezvous entries (requests below this are rejected, default: 0 = no minimum)")
	rendezvousCmd.Flags().Uint32("to0-max-wait", defaultMaxWaitSecs, fmt.Sprintf("Maximum wait time in seconds for TO0 rendezvous entries (requests above this are capped, default: %d seconds)", defaultMaxWaitSecs))
	rendezvousCmd.Flags().Uint32("cleanup-interval", defaultCleanupIntervalSecs, fmt.Sprintf("Interval in seconds for automatic cleanup of expired rendezvous blobs and sessions (set to 0 to disable, default: %d seconds)", defaultCleanupIntervalSecs))
	rendezvousCmd.Flags().Uint32("session-timeout", defaultSessionMaxAgeSecs, fmt.Sprintf("Maximum age in seconds for sessions before cleanup (default: %d seconds)", defaultSessionMaxAgeSecs))
	rendezvousCmd.Flags().Uint32("initial-cleanup-delay", defaultInitialCleanupDelaySecs, fmt.Sprintf("Delay in seconds before first cleanup after startup (default: %d seconds)", defaultInitialCleanupDelaySecs))
	viper.SetDefault("rendezvous.to0_min_wait", defaultMinWaitSecs)
	viper.SetDefault("rendezvous.to0_max_wait", defaultMaxWaitSecs)
	viper.SetDefault("rendezvous.cleanup_interval", defaultCleanupIntervalSecs)
	viper.SetDefault("rendezvous.session_timeout", defaultSessionMaxAgeSecs)
	viper.SetDefault("rendezvous.initial_cleanup_delay", defaultInitialCleanupDelaySecs)
}

func init() {
	rendezvousCmdInit()
}
