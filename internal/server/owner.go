package server

import (
	"crypto"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/resell"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/rvto2addr"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/voucher"
)

// OwnerServer represents the owner FDO server with modular handlers
type OwnerServer struct {
	// Domain-specific handler servers (Miguel's pattern)
	health    *health.Server
	rvto2addr *rvto2addr.Server
	voucher   *voucher.Server
	resell    *resell.Server

	// Dependencies
	ownerPKeys []crypto.PublicKey
	to2Server  *fdo.TO2Server
	db         *db.State

	// HTTP handler
	handler http.Handler
}

// NewOwnerServer creates a new owner server with modular architecture
func NewOwnerServer(ownerPKeys []crypto.PublicKey, to2Server *fdo.TO2Server, database *db.State) *OwnerServer {
	// Create domain-specific handlers
	healthServer := health.NewServer()
	rvto2addrServer := rvto2addr.NewServer(database)
	voucherServer := voucher.NewServer(database)
	resellServer := resell.NewServer(database, to2Server)

	// Create HTTP multiplexer (standard Go HTTP, no chi)
	mux := http.NewServeMux()

	// Register health endpoints
	healthHttpHandler := health.Handler(healthServer)
	mux.Handle("/health", healthHttpHandler)

	// Register RVTO2Addr endpoints
	rvto2addrHttpHandler := rvto2addr.Handler(rvto2addrServer)
	mux.Handle("/owner/redirect", rvto2addrHttpHandler)

	// Register voucher endpoints
	voucherHttpHandler := voucher.Handler(voucherServer)
	mux.Handle("/owner/vouchers", voucherHttpHandler)
	mux.Handle("/vouchers/", voucherHttpHandler)

	// Register resell endpoints
	resellHttpHandler := resell.Handler(resellServer)
	mux.Handle("/owner/resell/", resellHttpHandler)

	return &OwnerServer{
		health:     healthServer,
		rvto2addr:  rvto2addrServer,
		voucher:    voucherServer,
		resell:     resellServer,
		ownerPKeys: ownerPKeys,
		to2Server:  to2Server,
		db:         database,
		handler:    mux,
	}
}

// Handler returns the HTTP handler for the owner server
func (s *OwnerServer) Handler() http.Handler {
	return s.handler
}

// ServeHTTP implements http.Handler interface
func (s *OwnerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}
