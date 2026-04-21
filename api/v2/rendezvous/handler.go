package rendezvous

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/elnormous/contenttype"
	fdo_lib "github.com/fido-device-onboard/go-fdo"
	v1deviceca "github.com/fido-device-onboard/go-fdo-server/api/v1/deviceca"
	v2deviceca "github.com/fido-device-onboard/go-fdo-server/api/v2/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/api/v2/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	swaggerui "github.com/swaggest/swgui/v5emb"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var openAPISpecJSON []byte

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
	r.State = state
	return nil
}

func (r *Rendezvous) acceptVoucher(ctx context.Context, ov fdo_lib.Voucher, requestedTTLSecs uint32) (ttlSecs uint32, err error) {
	guid := ov.Header.Val.GUID
	slog.Debug("TO0 acceptVoucher called",
		"guid", fmt.Sprintf("%x", guid[:]),
		"requestedTTLSecs", requestedTTLSecs,
		"minWaitSecs", r.MinWaitSecs,
		"maxWaitSecs", r.MaxWaitSecs)

	// Verify device certificate chain against trusted device CAs
	certPool := r.State.DeviceCA.CertPool()

	if err := ov.VerifyDeviceCertChain(certPool); err != nil {
		slog.Error("TO0 device certificate chain verification failed",
			"guid", fmt.Sprintf("%x", guid[:]),
			"error", err)
		return 0, err
	}
	slog.Debug("TO0 device certificate chain verified successfully", "guid", fmt.Sprintf("%x", guid[:]))

	// Reject if below minimum
	if r.MinWaitSecs > 0 && requestedTTLSecs < r.MinWaitSecs {
		slog.Warn("TO0 request rejected: requested wait time below minimum",
			"guid", fmt.Sprintf("%x", guid[:]),
			"requestedTTLSecs", requestedTTLSecs,
			"minWaitSecs", r.MinWaitSecs)
		return 0, fmt.Errorf("requested wait time %d seconds is below minimum %d seconds",
			requestedTTLSecs, r.MinWaitSecs)
	}

	// Cap if above maximum
	if requestedTTLSecs > r.MaxWaitSecs {
		slog.Debug("TO0 request capped: requested wait time above maximum",
			"guid", fmt.Sprintf("%x", guid[:]),
			"requestedTTLSecs", requestedTTLSecs,
			"maxWaitSecs", r.MaxWaitSecs,
			"acceptedTTLSecs", r.MaxWaitSecs)
		return r.MaxWaitSecs, nil
	}

	slog.Debug("TO0 request accepted",
		"guid", fmt.Sprintf("%x", guid[:]),
		"acceptedTTLSecs", requestedTTLSecs)
	return requestedTTLSecs, nil
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
		slog.Error("Failed to cleanup expired rendezvous blobs", "error", err)
		errors = append(errors, err)
	} else {
		blobCount = count
	}

	// Cleanup expired sessions (only if sessionMaxAge > 0 to prevent deleting all sessions)
	if sessionMaxAge > 0 {
		if count, err := r.State.Token.CleanupExpiredSessions(ctx, sessionMaxAge); err != nil {
			slog.Error("Failed to cleanup expired sessions", "error", err)
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

func (r *Rendezvous) Handler() http.Handler {
	rendezvousServeMux := http.NewServeMux()
	// Wire FDO Handler
	fdoHandler := &fdo_http.Handler{
		Tokens: r.State.Token,
		TO0Responder: &fdo_lib.TO0Server{
			Session:       r.State.TO0Session,
			RVBlobs:       r.State.RVBlob,
			AcceptVoucher: r.acceptVoucher,
		},
		TO1Responder: &fdo_lib.TO1Server{
			Session: r.State.TO1Session,
			RVBlobs: r.State.RVBlob,
		},
	}
	rendezvousServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health Handler
	healthServer := health.NewServer(r.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rendezvousServeMux)

	deviceCACertContentTypes := []contenttype.MediaType{
		contenttype.NewMediaType("application/json"),
		contenttype.NewMediaType("application/x-pem-file"),
	}
	deviceCACertPreferredContentType := "application/json"

	// Wire deprecated V1 management APIs
	mgmtAPIServeMuxV1 := http.NewServeMux()
	deviceCAServerV1 := v1deviceca.NewServer(r.State.DeviceCA)
	deviceCAMiddlewaresV1 := []v1deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware(deviceCACertContentTypes, deviceCACertPreferredContentType),
	}
	deviceCAStrictHandlerV1 := v1deviceca.NewStrictHandler(&deviceCAServerV1, deviceCAMiddlewaresV1)
	v1deviceca.HandlerFromMux(deviceCAStrictHandlerV1, mgmtAPIServeMuxV1)

	mgmtAPIHandlerV1 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(1<<20, // 1MB
			mgmtAPIServeMuxV1,
		),
	)

	// Wire management APIs
	mgmtAPIServeMuxV2 := http.NewServeMux()

	deviceCAServerV2 := v2deviceca.NewServer(r.State.DeviceCA)
	deviceCAMiddlewaresV2 := []v2deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware(deviceCACertContentTypes, deviceCACertPreferredContentType),
	}
	deviceCAStrictHandler := v2deviceca.NewStrictHandler(&deviceCAServerV2, deviceCAMiddlewaresV2)
	v2deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtAPIServeMuxV2)

	mgmtAPIHandlerV2 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(1<<20, // 1MB
			mgmtAPIServeMuxV2,
		),
	)

	// Serve OpenAPI specification
	rendezvousServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if _, err := w.Write(openAPISpecJSON); err != nil {
			slog.Error("Failed to write OpenAPI spec response", "error", err)
		}
	})

	// Serve Swagger UI documentation
	rendezvousServeMux.Handle("GET /api/docs/", swaggerui.New(
		"Rendezvous",
		"/api/openapi.json",
		"/api/docs/"))

	rendezvousServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))
	rendezvousServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return rendezvousServeMux
}
