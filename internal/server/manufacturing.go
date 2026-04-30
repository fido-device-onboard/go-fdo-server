// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package server

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

	"github.com/fido-device-onboard/go-fdo-server/api/v2/manufacturer"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"gorm.io/gorm"
)

// ManufacturingServer represents the HTTP server
type ManufacturingServer struct {
	handler http.Handler
	config  *config.ManufacturingServerConfig
	db      *gorm.DB
}

// NewManufacturingServer creates a new manufacturing server
func NewManufacturingServer(config config.ManufacturingServerConfig) (*ManufacturingServer, error) {
	slog.Info("Initializing manufacturing server")

	// Initialize database
	gormDB, err := config.DB.GetDB()
	if err != nil {
		return nil, err
	}

	// Load keys and certificates from config
	mfgKey, err := config.GetManufacturerKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load manufacturer key: %w", err)
	}

	deviceKey, err := config.GetDeviceCAKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA key: %w", err)
	}

	deviceCACerts, err := config.GetDeviceCACerts()
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA certificates: %w", err)
	}

	ownerCert, err := config.GetOwnerCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to load owner certificate: %w", err)
	}

	// Create manufacturer handler
	mfg := manufacturer.NewManufacturer(gormDB, mfgKey, deviceKey, deviceCACerts, ownerCert)
	if err := mfg.InitDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize manufacturing state: %w", err)
	}

	httpHandler := mfg.Handler()

	slog.Info("Manufacturing server initialized successfully")
	return &ManufacturingServer{handler: httpHandler, config: &config, db: gormDB}, nil
}

// Start starts the HTTP server
func (s *ManufacturingServer) Start() error {
	srv := &http.Server{
		Handler:           s.handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Channel to listen for interrupt or terminate signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	// Goroutine to listen for signals and gracefully shut down the server
	go func() {
		<-stop
		slog.Info("Shutdown signal received, initiating graceful shutdown...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
		}

		// Close database connection
		if sqlDB, err := s.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				slog.Error("Failed to close database connection", "error", err)
			} else {
				slog.Debug("Database connection closed")
			}
		}
	}()

	slog.Debug("Starting server on:", "addr", s.config.HTTP.ListenAddress())
	lis, err := net.Listen("tcp", s.config.HTTP.ListenAddress())
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String())

	if s.config.ServerConfig.HTTP.UseTLS() {
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
		err = srv.ServeTLS(lis, s.config.HTTP.CertPath, s.config.HTTP.KeyPath)
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
