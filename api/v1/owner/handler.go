package owner

import (
	"context"
	"crypto"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/device"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/health"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/ownerinfo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/resell"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
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
	DB                 *gorm.DB
	State              *state.OwnerState
	ReuseCred          bool
	ServiceInfoModules *serviceinfo.ModuleStateMachines
}

// NewOwner creates a new Owner instance
func NewOwner(
	db *gorm.DB,
	reuseCreds bool,
) Owner {
	return Owner{
		DB:        db,
		ReuseCred: reuseCreds,
	}
}

func (o *Owner) InitDB() error {
	state, err := state.InitOwnerDB(o.DB)
	if err != nil {
		return err
	}
	if err = state.DeviceCA.LoadTrustedDeviceCAs(context.Background()); err != nil {
		slog.Error("failed to load trusted device CA certificates", "error", err)
		return fmt.Errorf("failed to load trusted device CA certificates: %w", err)
	}
	slog.Debug("Trusted device CA certificates loaded")
	o.State = state

	ownerInfoServer := ownerinfo.NewServer(state.RVTO2Addr)
	if err := ownerInfoServer.MigrateOwnerInfo(context.Background()); err != nil {
		return fmt.Errorf("failed to migrate owner_info: %w", err)
	}

	return nil
}

func (o *Owner) Handler() http.Handler {
	ownerServeMux := http.NewServeMux()

	to2Server := &fdo.TO2Server{
		Session:              o.State.TO2Session,
		Modules:              o.ServiceInfoModules,
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
	mgmtAPIServeMuxV1 := http.NewServeMux()

	// Wire Device API
	deviceServerV1 := device.NewServer(o.State.Voucher)
	deviceStrictHandlerV1 := device.NewStrictHandler(&deviceServerV1, nil)
	device.HandlerFromMux(deviceStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire the Device CA API
	deviceCAServerV1 := deviceca.NewServer(o.State.DeviceCA)
	deviceCAMiddlewaresV1 := []deviceca.StrictMiddlewareFunc{
		deviceca.ContentNegotiationMiddleware,
	}
	deviceCAStrictHandlerV1 := deviceca.NewStrictHandler(&deviceCAServerV1, deviceCAMiddlewaresV1)
	deviceca.HandlerFromMux(deviceCAStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire RVTO2 Address API
	ownerinfoServerV1 := ownerinfo.NewServer(o.State.RVTO2Addr)
	ownerinfoStrictHandlerV1 := ownerinfo.NewStrictHandler(&ownerinfoServerV1, nil)
	ownerinfo.HandlerFromMux(ownerinfoStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire Resell API
	resellServerV1 := resell.NewServer(o.State.Voucher, o.State.OwnerKey)
	resellStrictHandlerV1 := resell.NewStrictHandler(&resellServerV1, nil)
	resell.HandlerWithOptions(resellStrictHandlerV1, resell.StdHTTPServerOptions{BaseRouter: mgmtAPIServeMuxV1, BaseURL: "/owner"})

	// Wire Voucher API
	voucherServerV1 := voucher.NewServer(o.State.Voucher, []crypto.PublicKey{o.State.OwnerKey.Signer().Public()})
	voucherStrictHandlerV1 := voucher.NewStrictHandler(&voucherServerV1, nil)
	voucher.HandlerWithOptions(voucherStrictHandlerV1, voucher.StdHTTPServerOptions{BaseRouter: mgmtAPIServeMuxV1, BaseURL: "/owner"})

	mgmtHandlerV1 := middleware.RateLimitMiddleware(
		rate.NewLimiter(10, 10), // 10 req/s, burst of 10
		middleware.BodySizeMiddleware(10<<20, /* 10MB */
			mgmtAPIServeMuxV1,
		),
	)

	// Serve OpenAPI specification
	ownerServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // For Swagger UI
		if _, err := w.Write(openAPISpecJSON); err != nil {
			slog.Error("Failed to write OpenAPI spec response", "error", err)
		}
	})

	// Serve Swagger UI documentation
	ownerServeMux.Handle("GET /api/docs/", swaggerui.New(
		"Owner",
		"/api/openapi.json",
		"/api/docs/"))

	ownerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtHandlerV1))

	return ownerServeMux
}
