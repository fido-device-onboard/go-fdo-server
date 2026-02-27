package server

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/owner"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo/cbor"
)

const defaultTo0TTL uint32 = 300

// OwnerServer represents the HTTP server
type OwnerServer struct {
	handler *owner.Owner
	config  *config.OwnerServerConfig
}

// NewOwnerServer creates a new Server
func NewOwnerServer(config config.OwnerServerConfig) (*OwnerServer, error) {
	db, err := config.DB.GetDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Load owner signing key
	ownerKey, err := config.GetOwnerSigner()
	if err != nil {
		return nil, fmt.Errorf("failed to get owner signer: %w", err)
	}
	ownerKeyType, err := config.GetPrivateKeyType()
	if err != nil {
		return nil, fmt.Errorf("failed to get owner key type: %w", err)
	}

	// Create owner handler
	owner := owner.NewOwner(
		db,
		config.OwnerConfig.ReuseCred,
		config.OwnerConfig.TO0InsecureTLS,
		defaultTo0TTL,
		&config.OwnerConfig.ServiceInfo,
	)

	// Initialize database and state
	err = owner.InitDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize owner db: %w", err)
	}

	// Initialize owner key state with loaded key
	owner.State.OwnerKey = state.NewOwnerKeyPersistentState(ownerKey, ownerKeyType, nil)

	return &OwnerServer{handler: &owner, config: &config}, nil
}

func TO0(ctx context.Context, config *config.OwnerServerConfig, ownerState *state.OwnerState, ownerKeyState fdo.OwnerKeyPersistentState) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// nextTry holds per-GUID backoff based on TO0 refresh or fallback
	nextTry := make(map[string]time.Time)

	// Periodic cleanup to prevent unbounded memory growth
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("TO0 scheduler shutting down")
			return

		case <-cleanupTicker.C:
			// Clean up stale entries from nextTry map to prevent memory leaks
			now := time.Now()
			for guid, nextTime := range nextTry {
				// Remove entries that haven't been retried in over 24 hours
				if now.Sub(nextTime) > 24*time.Hour {
					delete(nextTry, guid)
					slog.Debug("to0 scheduler: cleaned up stale entry", "guid", guid)
				}
			}

		case <-ticker.C:
			// Create context with timeout for database operations
			dbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

			// Fetch vouchers that still need TO2
			vouchers, err := ownerState.Voucher.ListPendingTO0Vouchers(dbCtx)
			cancel()
			if err != nil {
				slog.Warn("to0 scheduler: list pending vouchers failed", "err", err)
				continue
			}

			now := time.Now()
			for _, v := range vouchers {
				// Check if context was canceled
				if ctx.Err() != nil {
					return
				}

				// Parse voucher to get GUID and RVInfo
				var ov fdo.Voucher
				if err := cbor.Unmarshal(v.CBOR, &ov); err != nil {
					slog.Warn("to0 scheduler: unmarshal voucher failed", "err", err)
					continue
				}
				guidHex := hex.EncodeToString(ov.Header.Val.GUID[:])

				// Skip if already completed
				dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				completed, err := ownerState.Voucher.IsTO2Completed(dbCtx, ov.Header.Val.GUID)
				cancel()
				if err != nil {
					slog.Warn("to0 scheduler: to2 completion check failed", "guid", guidHex, "err", err)
					continue
				}
				if completed {
					delete(nextTry, guidHex)
					continue
				}

				// Respect backoff schedule
				if t, ok := nextTry[guidHex]; ok && now.Before(t) {
					continue
				}

				// Attempt TO0 once for this GUID
				refresh, err := to0.RegisterRvBlob(ov.Header.Val.RvInfo, guidHex, ownerState.Voucher, ownerKeyState, ownerState.RVTO2Addr, config.OwnerConfig.TO0InsecureTLS, defaultTo0TTL)
				if err != nil {
					// On failure, retry after 10s
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
		}
	}
}

// Start starts the HTTP server
func (s *OwnerServer) Start() error {
	srv := &http.Server{
		Handler:           s.handler.Handler(),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Create context for coordinating graceful shutdown of background tasks
	to0Ctx, to0Cancel := context.WithCancel(context.Background())
	defer to0Cancel()

	// Channel to listen for interrupt or terminate signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	// Goroutine to listen for signals and gracefully shut down the server
	go func() {
		<-stop
		slog.Info("Shutdown signal received, initiating graceful shutdown...")

		// Cancel TO0 background task first
		to0Cancel()

		// Shutdown HTTP server with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Server forced to shutdown", "err", err)
		}

		// Close database connection
		if sqlDB, err := s.handler.DB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				slog.Error("Failed to close database connection", "err", err)
			} else {
				slog.Debug("Database connection closed")
			}
		}
	}()

	slog.Info("Starting TO0 background task")
	// Background TO0 scheduler: after restarts, continue attempting TO0 for any
	// devices without completed TO2 as recorded in the database.
	go TO0(to0Ctx, s.config, s.handler.State, s.handler.State.OwnerKey)

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
