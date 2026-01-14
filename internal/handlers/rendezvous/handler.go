package rendezvous

import (
	"context"
	"io"
	"net/http"

	"golang.org/x/time/rate"

	fdo_lib "github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/deviceca"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
)

// Rendezvous handles FDO protocol HTTP requests
type Rendezvous struct {
	State  *db.State
	MinTTL uint32
}

// NewRendezvous creates a new Rendezvous
func NewRendezvous(state *db.State) Rendezvous {
	return Rendezvous{State: state}
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

func (s *Rendezvous) Handler() http.Handler {
	rendezvousServeMux := http.NewServeMux()
	// Wire FDO Handler
	fdoHandler := &fdo_http.Handler{
		Tokens: s.State,
		TO0Responder: &fdo_lib.TO0Server{
			Session:       s.State,
			RVBlobs:       s.State,
		},
		TO1Responder: &fdo_lib.TO1Server{
			Session: s.State,
			RVBlobs: s.State,
		},
	}
	rendezvousServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health Handler
	healthServer := health.NewServer(s.State)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rendezvousServeMux)

	// Wire management APIs
	mgmtAPIServeMux := http.NewServeMux()

	deviceCAServer := deviceca.NewServer(s.State)
	deviceCAStrictHandler := deviceca.NewStrictHandler(&deviceCAServer, nil)
	deviceca.HandlerFromMux(deviceCAStrictHandler, mgmtAPIServeMux)

	mgmtAPIHandler := rateLimitMiddleware(
		rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, /* 1MB */
			mgmtAPIServeMux,
		),
	)
	rendezvousServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandler))

	return rendezvousServeMux
}
