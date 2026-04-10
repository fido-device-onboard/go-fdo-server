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

	"github.com/fido-device-onboard/go-fdo-server/api/v1/rendezvous"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"gorm.io/gorm"
)

// RendezvousServer represents the HTTP server
type RendezvousServer struct {
	handler http.Handler
	config  *config.RendezvousServerConfig
	db      *gorm.DB
}

// NewRendezvousServer creates a new rendezvous server
func NewRendezvousServer(config config.RendezvousServerConfig) (*RendezvousServer, error) {
	slog.Info("Initializing rendezvous server")

	db, err := config.DB.GetDB()
	if err != nil {
		slog.Error("Failed to get a database connection", "err", err)
		return nil, fmt.Errorf("failed to get a database connection: %w", err)
	}

	maxWaitSecs := config.Rendezvous.MaxWaitSecs
	minWaitSecs := config.Rendezvous.MinWaitSecs
	slog.Info("TO0 wait time limits configured", "minWaitSecs", minWaitSecs, "maxWaitSecs", maxWaitSecs)

	rendezvous := rendezvous.NewRendezvous(db, minWaitSecs, maxWaitSecs)
	if err = rendezvous.InitDB(); err != nil {
		slog.Error("failed to initialize rendezvous database", "err", err)
		return nil, fmt.Errorf("failed to initialize rendezvous database: %w", err)
	}
	slog.Info("Database initialized successfully", "type", config.DB.Type)

	handler := rendezvous.Handler()

	return &RendezvousServer{handler: handler, config: &config, db: db}, nil
}

// Start starts the HTTP server
func (s *RendezvousServer) Start() error {
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
			slog.Error("Server forced to shutdown", "err", err)
		}

		// Close database connection
		if sqlDB, err := s.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				slog.Error("Failed to close database connection", "err", err)
			} else {
				slog.Debug("Database connection closed")
			}
		}
	}()

	slog.Debug("Starting server on:", "addr", s.config.ServerConfig.HTTP.ListenAddress())
	lis, err := net.Listen("tcp", s.config.ServerConfig.HTTP.ListenAddress())
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
		err = srv.ServeTLS(lis, s.config.ServerConfig.HTTP.CertPath, s.config.ServerConfig.HTTP.KeyPath)
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
