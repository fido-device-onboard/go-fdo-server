package rendezvous

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	fdo_lib "github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
)

// Rendezvous handles FDO protocol HTTP requests
type Rendezvous struct {
	DB          *gorm.DB
	State       *state.RendezvousPersistentState
	MinWaitSecs uint32
	MaxWaitSecs uint32
}

// NewRendezvous creates a new Rendezvous
func NewRendezvous(db *gorm.DB, minWaitSecs, maxWaitSecs uint32) Rendezvous {
	return Rendezvous{DB: db, MinWaitSecs: minWaitSecs, MaxWaitSecs: maxWaitSecs}
}

func (r *Rendezvous) InitDB() error {
	state, err := state.InitRendezvousDB(r.DB)
	if err != nil {
		return err
	}
	if err = state.DeviceCA.LoadTrustedDeviceCAs(context.Background()); err != nil {
		slog.Error("failed to load trusted device CA certificates", "err", err)
		return fmt.Errorf("failed to load trusted device CA certificates: %w", err)
	}
	slog.Debug("Trusted device CA certificates loaded")
	r.State = state
	return nil
}

// StartPeriodicCleanup starts background cleanup tasks for expired rendezvous blobs and sessions
// The cleanup runs at the specified interval until the context is canceled
func (r *Rendezvous) StartPeriodicCleanup(ctx context.Context, cleanupInterval, sessionMaxAge, initialDelay time.Duration) {
	if cleanupInterval == 0 {
		slog.Info("Periodic cleanup is disabled")
		return
	}

	slog.Info("Starting periodic cleanup",
		"cleanupInterval", cleanupInterval,
		"sessionMaxAge", sessionMaxAge,
		"initialDelay", initialDelay)

	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping periodic cleanup")
			return
		case <-timer.C:
			r.runCleanup(ctx, sessionMaxAge)
			timer.Reset(cleanupInterval)
		}
	}
}

// runCleanup executes all cleanup tasks
func (r *Rendezvous) runCleanup(ctx context.Context, sessionMaxAge time.Duration) {
	startTime := time.Now()
	var blobCount, sessionCount int64
	var errors []error

	// Cleanup expired rendezvous blobs
	if count, err := r.State.RVBlob.CleanupExpiredBlobs(ctx); err != nil {
		slog.Error("Failed to cleanup expired rendezvous blobs", "err", err)
		errors = append(errors, err)
	} else {
		blobCount = count
	}

	// Cleanup expired sessions (only if sessionMaxAge > 0 to prevent deleting all sessions)
	if sessionMaxAge > 0 {
		if count, err := r.State.Token.CleanupExpiredSessions(ctx, sessionMaxAge); err != nil {
			slog.Error("Failed to cleanup expired sessions", "err", err)
			errors = append(errors, err)
		} else {
			sessionCount = count
		}
	} else {
		slog.Debug("Session cleanup is disabled (session_timeout <= 0)")
	}

	duration := time.Since(startTime)
	totalDeleted := blobCount + sessionCount

	logArgs := []any{
		"duration_ms", duration.Milliseconds(),
		"blobs_deleted", blobCount,
		"sessions_deleted", sessionCount,
		"total_deleted", totalDeleted,
	}

	if len(errors) > 0 {
		errorStrings := make([]string, len(errors))
		for i, err := range errors {
			errorStrings[i] = err.Error()
		}
		logArgs = append(logArgs, "error_count", len(errors), "errors", errorStrings)
		slog.Warn("Cleanup completed with errors", logArgs...)
	} else if totalDeleted > 0 {
		slog.Info("Cleanup completed", logArgs...)
	} else {
		slog.Debug("Cleanup completed, no items to delete", logArgs...)
	}
}

func rateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func bodySizeMiddleware(limitBytes int64, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.LimitReader(r.Body, limitBytes),
			Closer: r.Body,
		}
		next.ServeHTTP(w, r)
	}
}

func (s *Rendezvous) acceptVoucher(ctx context.Context, ov fdo_lib.Voucher, requestedTTLSecs uint32) (ttlSecs uint32, err error) {
	guid := ov.Header.Val.GUID
	slog.Debug("TO0 acceptVoucher called",
		"guid", fmt.Sprintf("%x", guid[:]),
		"requestedTTLSecs", requestedTTLSecs,
		"minWaitSecs", s.MinWaitSecs,
		"maxWaitSecs", s.MaxWaitSecs)

	// Verify device certificate chain against trusted device CAs
	s.State.DeviceCA.Mutex.RLock()
	certPool := s.State.DeviceCA.TrustedDeviceCACertPool
	s.State.DeviceCA.Mutex.RUnlock()

	if err := ov.VerifyDeviceCertChain(certPool); err != nil {
		slog.Error("TO0 device certificate chain verification failed",
			"guid", fmt.Sprintf("%x", guid[:]),
			"err", err)
		return 0, err
	}
	slog.Debug("TO0 device certificate chain verified successfully", "guid", fmt.Sprintf("%x", guid[:]))

	// Reject if below minimum
	if s.MinWaitSecs > 0 && requestedTTLSecs < s.MinWaitSecs {
		slog.Warn("TO0 request rejected: requested wait time below minimum",
			"guid", fmt.Sprintf("%x", guid[:]),
			"requestedTTLSecs", requestedTTLSecs,
			"minWaitSecs", s.MinWaitSecs)
		return 0, fmt.Errorf("requested wait time %d seconds is below minimum %d seconds",
			requestedTTLSecs, s.MinWaitSecs)
	}

	// Cap if above maximum
	if requestedTTLSecs > s.MaxWaitSecs {
		slog.Debug("TO0 request capped: requested wait time above maximum",
			"guid", fmt.Sprintf("%x", guid[:]),
			"requestedTTLSecs", requestedTTLSecs,
			"maxWaitSecs", s.MaxWaitSecs,
			"acceptedTTLSecs", s.MaxWaitSecs)
		return s.MaxWaitSecs, nil
	}

	slog.Debug("TO0 request accepted",
		"guid", fmt.Sprintf("%x", guid[:]),
		"acceptedTTLSecs", requestedTTLSecs)
	return requestedTTLSecs, nil
}

func (s *Rendezvous) Handler() http.Handler {
	rendezvousServeMux := http.NewServeMux()
	// Wire FDO Handler
	fdoHandler := &fdo_http.Handler{
		Tokens: s.State.Token,
		TO0Responder: &fdo_lib.TO0Server{
			Session:       s.State.TO0Session,
			RVBlobs:       s.State.RVBlob,
			AcceptVoucher: s.acceptVoucher,
		},
		TO1Responder: &fdo_lib.TO1Server{
			Session: s.State.TO1Session,
			RVBlobs: s.State.RVBlob,
		},
	}
	rendezvousServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health Handler
	healthServer := health.NewServer(s.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rendezvousServeMux)

	// Wire management APIs
	mgmtAPIServeMux := http.NewServeMux()

	deviceCAServer := deviceca.NewServer(s.State.DeviceCA)
	deviceCAMiddlewares := []deviceca.StrictMiddlewareFunc{
		deviceca.ContentNegotiationMiddleware,
	}
	deviceCAStrictHandler := deviceca.NewStrictHandler(&deviceCAServer, deviceCAMiddlewares)
	deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtAPIServeMux)

	mgmtAPIHandler := rateLimitMiddleware(
		rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, // 1MB
			mgmtAPIServeMux,
		),
	)
	rendezvousServeMux.Handle("/api/v1/", http.StripPrefix("/api", mgmtAPIHandler))

	return rendezvousServeMux
}
