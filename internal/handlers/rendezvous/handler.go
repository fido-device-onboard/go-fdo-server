package rendezvous

import (
	"net/http"

	fdo_lib "github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
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

func (s *Rendezvous) Handler() http.Handler {
	rendezvousServeMux := http.NewServeMux()
	// Wire FDO Handler
	fdoHandler := &fdo_http.Handler{
		Tokens: s.State,
		TO0Responder: &fdo_lib.TO0Server{
			Session: s.State,
			RVBlobs: s.State,
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

	return rendezvousServeMux
}
