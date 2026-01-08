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
	db          *db.State
	isOwnerMode bool // true for owner redirect info, false for rendezvous info
}

// NewServer creates a new rvto2addr server instance for rendezvous info (manufacturing server)
func NewServer(database *db.State) *Server {
	return &Server{
		db:          database,
		isOwnerMode: false, // Handle rvinfo
	}
}

// NewOwnerServer creates a new rvto2addr server instance for owner redirect info (owner server)
func NewOwnerServer(database *db.State) *Server {
	return &Server{
		db:          database,
		isOwnerMode: true, // Handle owner redirect info
	}
}

// Handler creates an HTTP handler for rvto2addr endpoints following Miguel's pattern
func Handler(s *Server) http.Handler {
	mux := http.NewServeMux()

	// Manual HTTP handler registration following Miguel's pattern
	// Note: This will be mounted at /api/v1/rvinfo by the calling code
	mux.HandleFunc("/", s.handleGetOwnerRedirect)      // Handle GET /api/v1/rvinfo
	mux.HandleFunc("PUT /", s.handleSetOwnerRedirect)  // Handle PUT /api/v1/rvinfo
	mux.HandleFunc("POST /", s.handleSetOwnerRedirect) // Handle POST /api/v1/rvinfo

	return mux
}

func (s *Server) handleGetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	if s.isOwnerMode {
		// Retrieve owner redirect info from database (RVTO2Addr)
		ownerInfoBytes, err := s.db.FetchOwnerInfo()
		if err != nil {
			// Match original behavior: return 404 with plain text error
			http.Error(w, "No ownerInfo found", http.StatusNotFound)
			return
		}

		// Return the raw JSON data as stored in the database
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(ownerInfoBytes)
	} else {
		// Retrieve rendezvous info from database (rvinfo)
		rvInfoBytes, err := s.db.FetchRvInfoJSON()
		if err != nil {
			// Match original behavior: return 404 with plain text error
			http.Error(w, "No rvInfo found", http.StatusNotFound)
			return
		}

		// Return the raw JSON data as stored in the database
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(rvInfoBytes)
	}
}

func (s *Server) handleSetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if s.isOwnerMode {
		// Handle owner redirect info (RVTO2Addr)
		// First check if owner info already exists
		_, err = s.db.FetchOwnerInfo()
		if err != nil {
			// Record doesn't exist, try to insert it (POST behavior)
			if err := s.db.SetOwnerInfo(body); err != nil {
				http.Error(w, "Failed to create owner info", http.StatusInternalServerError)
				return
			}
		} else {
			// Record exists, update it (PUT behavior)
			if err := s.db.SetOwnerInfo(body); err != nil {
				http.Error(w, "Failed to update owner info", http.StatusInternalServerError)
				return
			}
		}
	} else {
		// Handle rendezvous info (rvinfo)
		// First check if rvinfo already exists
		_, err = s.db.FetchRvInfoJSON()
		if err != nil {
			// Record doesn't exist, try to insert it (POST behavior)
			if err := s.db.InsertRvInfo(body); err != nil {
				http.Error(w, "Failed to create RV info", http.StatusInternalServerError)
				return
			}
		} else {
			// Record exists, update it (PUT behavior)
			if err := s.db.UpdateRvInfo(body); err != nil {
				http.Error(w, "Failed to update RV info", http.StatusInternalServerError)
				return
			}
		}
	}

	// Return the data that was stored
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
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
