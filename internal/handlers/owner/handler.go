package owner

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"golang.org/x/time/rate"
)

// Owner handles FDO protocol HTTP requests
type Owner struct {
	State                   *db.State
	ReuseCred               bool
	TO0InsecureTLS          bool
	OwnerModuleStateMachine map[string]*ownerModule
	date                    bool
	wgets                   []string
	wgetURLs                []*url.URL // Parsed wget URLs
	uploads                 []string
	uploadDir               string
	downloads               []string
	downloadPaths           []string // Cleaned download file paths
	DefaultTo0TTL           uint32
	// current module state machine state for all sessions (indexed by token)
}

// NewOwner creates a new Owner instance
func NewOwner(
	state *db.State,
	reuseCreds bool,
	to0InsecureTLS bool,
	date bool,
	wgets []string,
	wgetURLs []*url.URL,
	uploads []string,
	uploadDir string,
	downloads []string,
	downloadPaths []string,
	defaultTo0TTL uint32,
) Owner {
	return Owner{
		State:                   state,
		ReuseCred:               reuseCreds,
		TO0InsecureTLS:          to0InsecureTLS,
		OwnerModuleStateMachine: make(map[string]*ownerModule),
		date:                    date,
		wgets:                   wgets,
		wgetURLs:                wgetURLs,
		uploads:                 uploads,
		uploadDir:               uploadDir,
		downloads:               downloads,
		downloadPaths:           downloadPaths,
		DefaultTo0TTL:           defaultTo0TTL,
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

func (o *Owner) Handler() http.Handler {
	ownerServeMux := http.NewServeMux()

	to2Server := &fdo.TO2Server{
		Session:              o.State,
		Vouchers:             o.State,
		VouchersForExtension: o.State,
		OwnerKeys:            o.State,
		RvInfo: func(_ context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		Modules:         o,
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return o.ReuseCred, nil },
		VerifyVoucher: func(_ context.Context, voucher fdo.Voucher) error {
			return VerifyVoucher(&voucher, o.State)
		},
	}

	// Wire FDO owner handler
	fdoHandler := &fdo_http.Handler{
		Tokens:       o.State,
		TO2Responder: to2Server,
	}
	ownerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health API
	healthServer := health.NewServer(o.State)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, ownerServeMux)

	// Wire mgmt APIs
	mgmtServeMux := http.NewServeMux()
	//
	// rvto2addrServer := rvto2addr.NewServer(s.State)
	// rvto2addrStrictHandler := rvto2addr.NewStrictHandler(&rvto2addrServer, nil)
	// rvto2addr.HandlerFromMux(rvto2addrStrictHandler, mgmtServeMux)
	//
	// voucherServer := voucher.NewServer(s.State)
	// voucherStrictHandler := voucher.NewStrictHandler(&voucherServer, nil)
	// voucher.HandlerFromMux(voucherStrictHandler, mgmtServeMux)
	//
	// Old APIs to be migrated to the code commented above
	mgmtServeMux.Handle("POST /owner/vouchers", InsertVoucherHandler(o.State))
	mgmtServeMux.Handle("POST /owner/resell/{guid}", ResellHandler(to2Server))
	mgmtServeMux.Handle("GET /owner/devices", http.HandlerFunc(OwnerDevicesHandler))
	mgmtServeMux.HandleFunc("/owner/redirect", OwnerInfoHandler)

	deviceCAServer := deviceca.NewServer(o.State)
	deviceCAStrictHandler := deviceca.NewStrictHandler(&deviceCAServer, nil)
	deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtServeMux)

	mgmtHandler := rateLimitMiddleware(
		rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, /* 1MB */
			mgmtServeMux,
		),
	)

	ownerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtHandler))

	return ownerServeMux
}
