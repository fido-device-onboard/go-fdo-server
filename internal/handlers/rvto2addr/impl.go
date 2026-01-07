package rvto2addr

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
)

// Server implements RVTO2Addr HTTP handlers
type Server struct {
	db *db.State
}

// NewServer creates a new rvto2addr server instance
func NewServer(database *db.State) *Server {
	return &Server{
		db: database,
	}
}

// Handler creates an HTTP handler for rvto2addr endpoints following Miguel's pattern
func Handler(s *Server) http.Handler {
	mux := http.NewServeMux()

	// Manual HTTP handler registration following Miguel's pattern
	mux.HandleFunc("GET /owner/redirect", s.handleGetOwnerRedirect)
	mux.HandleFunc("PUT /owner/redirect", s.handleSetOwnerRedirect)

	return mux
}

func (s *Server) handleGetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	// Retrieve owner redirect info from database
	ownerInfoBytes, err := s.db.FetchOwnerInfo()
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Owner redirect information not found")
		return
	}

	var response components.RVTO2Addr
	if err := json.Unmarshal(ownerInfoBytes, &response); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to parse owner redirect information")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var request components.RVTO2Addr
	if err := json.Unmarshal(body, &request); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON in request body")
		return
	}

	// Marshal the request data to store in database
	requestBytes, err := json.Marshal(request)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process request data")
		return
	}

	// Update owner redirect info in database
	if err := s.db.SetOwnerInfo(requestBytes); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to update owner redirect information")
		return
	}

	// Return updated info
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(request)
}

func (s *Server) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	errorResp := components.Error{
		Error:   "rvto2addr_error",
		Message: message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}
