package health

import (
	"encoding/json"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

// Server implements health check handlers following Miguel's pattern
type Server struct{}

// NewServer creates a new health server instance
func NewServer() *Server {
	return &Server{}
}

// Handler creates an HTTP handler for health endpoints following Miguel's pattern
func Handler(s *Server) http.Handler {
	mux := http.NewServeMux()

	// Manual HTTP handler registration following Miguel's pattern
	mux.HandleFunc("GET /health", s.handleGetHealth)

	return mux
}

// handleGetHealth handles health check requests
func (s *Server) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	version := version.VERSION
	response := HealthResponse{
		Status:  "healthy",
		Version: &version,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
