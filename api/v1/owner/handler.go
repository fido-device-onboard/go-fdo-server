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
	device_v1 "github.com/fido-device-onboard/go-fdo-server/api/v1/device"
	health_v1 "github.com/fido-device-onboard/go-fdo-server/api/v1/health"
	ownerinfo_v1 "github.com/fido-device-onboard/go-fdo-server/api/v1/ownerinfo"
	resell_v1 "github.com/fido-device-onboard/go-fdo-server/api/v1/resell"
	voucher_v1 "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/serviceinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var ownerSpecJSON []byte

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

	// Serve OpenAPI specification
	ownerServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // For Swagger UI
		w.Write(ownerSpecJSON)
	})

	// Serve Swagger UI documentation
	ownerServeMux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>FDO Owner API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/api/openapi.json',
            dom_id: '#swagger-ui',
        });
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	// Create ModuleStateMachine for ServiceInfo
	// This properly handles per-session state tracking using token-based indexing
	modules := serviceinfo.NewModuleStateMachines(o.State, o.ServiceInfoConfig)

	to2Server := &fdo.TO2Server{
		Session:              o.State.TO2Session,
		Modules:              modules,
		Vouchers:             o.State.Voucher,
		VouchersForExtension: o.State.Voucher,
		OwnerKeys:            o.State.OwnerKey,
		RvInfo: func(_ context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return o.ReuseCred, nil },
		VerifyVoucher: func(ctx context.Context, voucher fdo.Voucher) error {
			return VerifyVoucher(ctx, voucher, o.State.OwnerKey.Signer(), o.State, o.ReuseCred)
		},
	}

	// Wire FDO owner handler
	fdoHandler := &fdo_http.Handler{
		Tokens:       o.State.Token,
		TO2Responder: to2Server,
	}
	ownerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health API
	healthServer := health_v1.NewServer(o.State.Health)
	healthStrictHandler := health_v1.NewStrictHandler(&healthServer, nil)
	health_v1.HandlerFromMux(healthStrictHandler, ownerServeMux)

	// Wire mgmt APIs
	mgmtServeMuxV1 := http.NewServeMux()

	// Wire RVTO2 Address API
	ownerinfoServerV1 := ownerinfo_v1.NewServer(o.State.RVTO2Addr)
	ownerinfoStrictHandlerV1 := ownerinfo_v1.NewStrictHandler(&ownerinfoServerV1, nil)
	ownerinfo_v1.HandlerFromMux(ownerinfoStrictHandlerV1, mgmtServeMuxV1)

	// Wire Voucher API
	voucherServerV1 := voucher_v1.NewServer(o.State.Voucher, []crypto.PublicKey{o.State.OwnerKey.Signer().Public()})
	voucherStrictHandlerV1 := voucher_v1.NewStrictHandler(&voucherServerV1, nil)
	voucher_v1.HandlerWithOptions(voucherStrictHandlerV1, voucher_v1.StdHTTPServerOptions{BaseRouter: mgmtServeMuxV1, BaseURL: "/owner"})

	// Wire Resell API
	resellServerV1 := resell_v1.NewServer(o.State.Voucher, o.State.OwnerKey)
	resellStrictHandlerV1 := resell_v1.NewStrictHandler(&resellServerV1, nil)
	resell_v1.HandlerWithOptions(resellStrictHandlerV1, resell_v1.StdHTTPServerOptions{BaseRouter: mgmtServeMuxV1, BaseURL: "/owner"})

	// Wire Device API
	deviceServerV1 := device_v1.NewServer(o.State.Voucher)
	deviceStrictHandlerV1 := device_v1.NewStrictHandler(&deviceServerV1, nil)
	device_v1.HandlerFromMux(deviceStrictHandlerV1, mgmtServeMuxV1)

	mgmtHandlerV1 := rateLimitMiddleware(
		rate.NewLimiter(10, 10), // 10 req/s, burst of 10
		bodySizeMiddleware(10<<20, /* 10MB */
			mgmtServeMuxV1,
		),
	)

	ownerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtHandlerV1))

	return ownerServeMux
}
