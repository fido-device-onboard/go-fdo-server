// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/owner"
	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// FSIM command line flags
	date          bool
	wgets         []string
	wgetURLs      []*url.URL // Parsed wget URLs
	uploads       []string
	uploadDir     string
	downloads     []string
	downloadPaths []string // Cleaned download file paths
	defaultTo0TTL uint32   = 300
)

// ownerCmd represents the owner command
var ownerCmd = &cobra.Command{
	Use:   "owner http_address",
	Short: "Serve an instance of the owner server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Rebind only those keys needed by the owner command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		if err := viper.BindPFlag("owner.reuse_credentials", cmd.Flags().Lookup("reuse-credentials")); err != nil {
			return err
		}
		if err := viper.BindPFlag("device_ca.cert", cmd.Flags().Lookup("device-ca-cert")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.key", cmd.Flags().Lookup("owner-key")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.to0_insecure_tls", cmd.Flags().Lookup("to0-insecure-tls")); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var ownerConfig OwnerServerConfig
		if err := viper.Unmarshal(&ownerConfig); err != nil {
			return fmt.Errorf("failed to unmarshal owner config: %w", err)
		}
		if err := ownerConfig.validate(); err != nil {
			return fmt.Errorf("failed to validate config: %w", err)
		}
		slog.Info("Parsed Config:", "ownerConfig", fmt.Sprintf("%+v", ownerConfig))
		return serveOwner(&ownerConfig)
	},
}

// OwnerServer represents the HTTP server
type OwnerServer struct {
	handler http.Handler
	config  HTTPConfig
}

// NewOwnerServer creates a new Server
func NewOwnerServer(config HTTPConfig, handler http.Handler) *OwnerServer {
	return &OwnerServer{handler: handler, config: config}
}

func serveOwner(config *OwnerServerConfig) error {
	state, err := config.getState()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	ownerKey, err := config.getOwnerSigner()
	if err != nil {
		return fmt.Errorf("failed to get owner signer: %w", err)
	}
	ownerKeyType, err := config.getPrivateKeyType()
	if err != nil {
		return fmt.Errorf("failed to get owner key type: %w", err)
	}
	// Add owner keys to the database
	if err = state.AddOwnerKey(ownerKeyType, ownerKey, nil); err != nil {
		return fmt.Errorf("failed to add owner key to database: %w", err)
	}
	slog.Debug("Loading device CA certificate from configuration")
	deviceCACerts, err := config.DeviceCAConfig.getDeviceCACertsAsPEM()
	if err != nil {
		slog.Error("Failed to get device CA cert as PEM", "err", err)
		return fmt.Errorf("failed to get device CA cert as PEM: %w", err)
	}

	slog.Debug("Importing device CA certificate to database")
	stats, err := state.ImportDeviceCACertificates(context.Background(), deviceCACerts)
	if err != nil {
		slog.Error("Failed to import device CA certificate", "err", err)
		return fmt.Errorf("failed to import device CA certificate: %w", err)
	}
	slog.Info("Device CA certificate import completed",
		"detected", stats.Detected,
		"imported", stats.Imported,
		"skipped", stats.Skipped,
		"malformed", stats.Malformed)

	mux := owner.NewOwner(
		state,
		config.OwnerConfig.ReuseCred,
		config.OwnerConfig.TO0InsecureTLS,
		date,
		wgets,
		wgetURLs,
		uploads,
		uploadDir,
		downloads,
		downloadPaths,
		defaultTo0TTL,
	)
	handler := mux.Handler()
	server := NewOwnerServer(config.HTTP, handler)

	slog.Info("Starting TO0 background task")
	// Background TO0 scheduler: after restarts, continue attempting TO0 for any
	// devices without completed TO2 as recorded in the database.
	go TO0(config, state)

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	return server.Start()
}

func TO0(config *OwnerServerConfig, state *db.State) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	// nextTry holds per-GUID backoff based on TO0 refresh or fallback
	nextTry := make(map[string]time.Time)
	for {
		// Fetch vouchers that still need TO2
		vouchers, err := db.ListPendingTO0Vouchers(true)
		if err != nil {
			slog.Warn("to0 scheduler: list pending vouchers failed", "err", err)
			<-ticker.C
			continue
		}
		now := time.Now()
		for _, v := range vouchers {
			// Parse voucher to get GUID and RVInfo
			var ov fdo.Voucher
			if err := cbor.Unmarshal(v.CBOR, &ov); err != nil {
				slog.Warn("to0 scheduler: unmarshal voucher failed", "err", err)
				continue
			}
			guidHex := hex.EncodeToString(ov.Header.Val.GUID[:])
			// Skip if already completed
			completed, err := db.IsTO2Completed(ov.Header.Val.GUID[:])
			if err != nil {
				slog.Warn("to0 scheduler: to2 completion check failed", "guid", guidHex, "err", err)
				continue
			}
			if completed {
				delete(nextTry, guidHex)
				continue
			} // Respect backoff schedule
			if t, ok := nextTry[guidHex]; ok && now.Before(t) {
				continue
			}
			// Attempt TO0 once for this GUID
			refresh, err := to0.RegisterRvBlob(ov.Header.Val.RvInfo, guidHex, state, state, config.OwnerConfig.TO0InsecureTLS, defaultTo0TTL)
			if err != nil {
				// On failure, retry after 60s
				nextTry[guidHex] = now.Add(10 * time.Second)
				slog.Warn("to0 scheduler: register 'RV2TO0Addr' failed", "guid", guidHex, "err", err)
				continue
			}
			if refresh == 0 {
				refresh = defaultTo0TTL
			}
			slog.Debug("to0 scheduler: register 'RV2TO0Addr' completed", "guid", guidHex, "refresh", refresh)
			nextTry[guidHex] = now.Add(time.Duration(refresh) * time.Second)
		}
		<-ticker.C
	}
}

// Start starts the HTTP server
func (s *OwnerServer) Start() error {
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

// Set up the owner command line. Used by the unit tests to reset state between tests.
func ownerCmdInit() {
	rootCmd.AddCommand(ownerCmd)

	// TODO: add FSIM to configuration file TBD
	ownerCmd.Flags().BoolVar(&date, "command-date", false, "Use fdo.command FSIM to have device run \"date --utc\"")
	ownerCmd.Flags().StringArrayVar(&wgets, "command-wget", nil, "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")
	ownerCmd.Flags().StringArrayVar(&uploads, "command-upload", nil, "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	ownerCmd.Flags().StringVar(&uploadDir, "upload-directory", "", "The directory `path` to put file uploads")
	ownerCmd.Flags().StringArrayVar(&downloads, "command-download", nil, "Use fdo.download FSIM for each `file` (flag may be used multiple times)")

	// Declare any CLI flags for overriding configuration file settings.
	// These flags are bound to Viper in the ownerCmd PreRun handler.
	ownerCmd.Flags().Bool("reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
	ownerCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	ownerCmd.Flags().String("owner-key", "", "Owner private key path")
	ownerCmd.Flags().Bool("to0-insecure-tls", false, "Use insecure TLS (skip rendezvous certificate verification) for TO0")
}

func init() {
	ownerCmdInit()
}
