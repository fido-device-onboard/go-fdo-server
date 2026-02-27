package owner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/rvto2addr"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/serviceinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

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

// TODO Define the device API with OpenAPI
// handleOwnerDevices returns devices with their replacement GUIDs
// GET /api/v1/owner/devices?old_guid={guid}
func (o *Owner) handleOwnerDevices(w http.ResponseWriter, r *http.Request) {
	type Device struct {
		GUID    string `json:"guid"`
		OldGUID string `json:"old_guid,omitempty"`
	}

	// Get old_guid filter from query params
	oldGuidHex := r.URL.Query().Get("old_guid")
	if oldGuidHex == "" {
		http.Error(w, "old_guid query parameter required", http.StatusBadRequest)
		return
	}

	// Decode the old GUID
	oldGuidBytes, err := hex.DecodeString(oldGuidHex)
	if err != nil || len(oldGuidBytes) != 16 {
		http.Error(w, "Invalid GUID format", http.StatusBadRequest)
		return
	}

	var oldGuid protocol.GUID
	copy(oldGuid[:], oldGuidBytes)

	// Query the replacement voucher to get the new GUID
	newGuid, err := o.State.Voucher.GetReplacementGUID(r.Context(), oldGuid)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return empty array if no replacement found
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Device{})
			return
		}
		slog.Error("Error getting replacement GUID", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return the device with both old and new GUIDs
	devices := []Device{
		{
			GUID:    hex.EncodeToString(newGuid[:]),
			OldGUID: oldGuidHex,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
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
	healthServer := health.NewServer(o.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, ownerServeMux)

	// Wire mgmt APIs
	mgmtServeMux := http.NewServeMux()

	// Wire RVTO2 Address API
	rvto2addrServer := rvto2addr.NewServer(o.State.RVTO2Addr)
	rvto2addrStrictHandler := rvto2addr.NewStrictHandler(&rvto2addrServer, nil)
	rvto2addr.HandlerFromMux(rvto2addrStrictHandler, mgmtServeMux)

	// Wire Voucher API with content negotiation middleware
	voucherServer := voucher.NewServer(o.State.Voucher, o.State.DeviceCA)
	voucherMiddlewares := []voucher.StrictMiddlewareFunc{
		voucher.ContentNegotiationMiddleware,
	}
	voucherStrictHandler := voucher.NewStrictHandler(&voucherServer, voucherMiddlewares)
	voucher.HandlerFromMux(voucherStrictHandler, mgmtServeMux)

	// Wire Device CA API with content negotiation middleware
	deviceCAServer := deviceca.NewServer(o.State.DeviceCA)
	deviceCAMiddlewares := []deviceca.StrictMiddlewareFunc{
		deviceca.ContentNegotiationMiddleware,
	}
	deviceCAStrictHandler := deviceca.NewStrictHandler(&deviceCAServer, deviceCAMiddlewares)
	deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtServeMux)

	mgmtHandler := rateLimitMiddleware(
		rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, /* 1MB */
			mgmtServeMux,
		),
	)

	ownerServeMux.Handle("/api/", http.StripPrefix("/api", mgmtHandler))

	// Wire legacy devices endpoint (not in OpenAPI spec)
	// Register directly on ownerServeMux with full path
	ownerServeMux.HandleFunc("GET /api/v1/owner/devices", o.handleOwnerDevices)

	return ownerServeMux
}
