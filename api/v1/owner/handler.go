package owner

import (
	"context"
	"crypto"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/device"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/health"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/ownerinfo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/resell"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/serviceinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdohttp "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	swaggerui "github.com/swaggest/swgui/v5emb"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var openAPISpecJSON []byte

// Owner handles FDO protocol HTTP requests
type Owner struct {
	DB                *gorm.DB
	State             *state.OwnerState
	ReuseCred         bool
	TO0InsecureTLS    bool
	DefaultTo0TTL     uint32
	ServiceInfoConfig *config.ServiceInfoConfig
}

// NewOwner creates a new Owner instance
func NewOwner(
	db *gorm.DB,
	reuseCreds bool,
	to0InsecureTLS bool,
	defaultTo0TTL uint32,
	serviceInfoConfig *config.ServiceInfoConfig,
) Owner {
	return Owner{
		DB:                db,
		ReuseCred:         reuseCreds,
		TO0InsecureTLS:    to0InsecureTLS,
		DefaultTo0TTL:     defaultTo0TTL,
		ServiceInfoConfig: serviceInfoConfig,
	}
}

func (r *Owner) InitDB() error {
	state, err := state.InitOwnerDB(r.DB)
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

func (o *Owner) Handler() http.Handler {
	ownerServeMux := http.NewServeMux()

	// Create ModuleStateMachine for ServiceInfo
	// This properly handles per-session state tracking using token-based indexing
	modules := serviceinfo.NewModuleStateMachines(o.State, o.ServiceInfoConfig)

	to2Server := &fdo.TO2Server{
		Session:              o.State.TO2Session,
		Modules:              modules,
		Vouchers:             o.State.Voucher,
		VouchersForExtension: o.State.Voucher,
		OwnerKeys:            o.State.OwnerKey,
		RvInfo: func(ctx context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return o.ReuseCred, nil },
		VerifyVoucher: func(ctx context.Context, voucher fdo.Voucher) error {
			return VerifyVoucher(ctx, voucher, o.State.OwnerKey.Signer(), o.State, o.ReuseCred)
		},
	}

	// Wire FDO owner handler
	fdoHandler := &fdohttp.Handler{
		Tokens:       o.State.Token,
		TO2Responder: to2Server,
	}
	ownerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health API
	healthServer := health.NewServer(o.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, ownerServeMux)

	// Wire mgmt APIs
	mgmtServeMuxV1 := http.NewServeMux()

	// Wire RVTO2 Address API
	ownerinfoServerV1 := ownerinfo.NewServer(o.State.RVTO2Addr)
	ownerinfoStrictHandlerV1 := ownerinfo.NewStrictHandler(&ownerinfoServerV1, nil)
	ownerinfo.HandlerFromMux(ownerinfoStrictHandlerV1, mgmtServeMuxV1)

	// Wire Voucher API
	voucherServerV1 := voucher.NewServer(o.State.Voucher, []crypto.PublicKey{o.State.OwnerKey.Signer().Public()})
	voucherStrictHandlerV1 := voucher.NewStrictHandler(&voucherServerV1, nil)
	voucher.HandlerWithOptions(voucherStrictHandlerV1, voucher.StdHTTPServerOptions{BaseRouter: mgmtServeMuxV1, BaseURL: "/owner"})

	// Wire Resell API
	resellServerV1 := resell.NewServer(o.State.Voucher, o.State.OwnerKey)
	resellStrictHandlerV1 := resell.NewStrictHandler(&resellServerV1, nil)
	resell.HandlerWithOptions(resellStrictHandlerV1, resell.StdHTTPServerOptions{BaseRouter: mgmtServeMuxV1, BaseURL: "/owner"})

	// Wire Device API
	deviceServerV1 := device.NewServer(o.State.Voucher)
	deviceStrictHandlerV1 := device.NewStrictHandler(&deviceServerV1, nil)
	device.HandlerFromMux(deviceStrictHandlerV1, mgmtServeMuxV1)

	mgmtHandlerV1 := rateLimitMiddleware(
		rate.NewLimiter(10, 10), // 10 req/s, burst of 10
		bodySizeMiddleware(10<<20, /* 10MB */
			mgmtServeMuxV1,
		),
	)

	// Serve OpenAPI specification
	ownerServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // For Swagger UI
		w.Write(openAPISpecJSON)
	})

	// Serve Swagger UI documentation
	ownerServeMux.Handle("GET /api/docs", swaggerui.New(
		"Rendezvous",
		"/api/openapi.json",
		"/api/docs/"))

	ownerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtHandlerV1))

	return ownerServeMux
}
