// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"gorm.io/gorm"
)

// TEMPORARY: Backward compatibility wrapper for manufacturing server
// TODO: Remove once manufacturing server is refactored to use OpenAPI interface
func RvInfoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Received RV request", "method", r.Method, "path", r.URL.Path)
		s := &Server{} // Empty server for compatibility
		switch r.Method {
		case http.MethodGet:
			s.GetOwnerRedirect(w, r)
		case http.MethodPost:
			s.PostOwnerRedirect(w, r)
		case http.MethodPut:
			s.PutOwnerRedirect(w, r)
		default:
			slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
			WriteErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", "HTTP method "+r.Method+" is not supported for this endpoint", "Method not allowed")
		}
	}
}

// GetOwnerRedirect implements the rvInfo GET endpoint (OpenAPI interface method)
func (s *Server) GetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Fetching rvInfo")

	var rvInfoJSON []byte
	var err error

	// Use struct database if available, otherwise fall back to global db
	if s.db != nil {
		rvInfoJSON, err = s.db.FetchRvInfoJSON()
	} else {
		rvInfoJSON, err = db.FetchRvInfoJSON()
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No rvInfo found")
			WriteErrorResponse(w, r, http.StatusNotFound, "No rvInfo found", "rvInfo has not been configured", "No rvInfo found")
		} else {
			slog.Error("Error fetching rvInfo", "error", err)
			WriteErrorResponse(w, r, http.StatusInternalServerError, "Error fetching rvInfo", err.Error(), "Error fetching rvInfo")
		}
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(rvInfoJSON)
}

// PostOwnerRedirect implements the rvInfo POST endpoint (OpenAPI interface method)
func (s *Server) PostOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	slog.Debug("PostOwnerRedirect: Starting handler")
	
	rvInfo, ok := ReadRequestBody(w, r)
	if !ok {
		slog.Error("PostOwnerRedirect: Failed to read request body")
		return
	}
	
	slog.Debug("PostOwnerRedirect: Read request body", "size", len(rvInfo))

	// Use struct database if available, otherwise fall back to global db
	var err error
	if s.db != nil {
		slog.Debug("PostOwnerRedirect: Using struct database")
		err = s.db.InsertRvInfo(rvInfo)
	} else {
		slog.Debug("PostOwnerRedirect: Using global database (fallback)")
		err = db.InsertRvInfo(rvInfo)
	}
	
	slog.Debug("PostOwnerRedirect: Insert operation completed", "error", err)

	if err != nil {
		if HandleDBError(w, r, "rvInfo", err) {
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid rvInfo", err.Error(), "Invalid rvInfo")
			return
		}
		slog.Error("Error inserting rvInfo", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error inserting rvInfo", err.Error(), "Error inserting rvInfo")
		return
	}

	slog.Debug("rvInfo created")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(rvInfo)
	
	slog.Debug("PostOwnerRedirect: Successfully completed, response sent")
}

// PutOwnerRedirect implements the rvInfo PUT endpoint (OpenAPI interface method)
func (s *Server) PutOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	rvInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	// Use struct database if available, otherwise fall back to global db
	var err error
	if s.db != nil {
		err = s.db.UpdateRvInfo(rvInfo)
	} else {
		err = db.UpdateRvInfo(rvInfo)
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("rvInfo does not exist, cannot update")
			WriteErrorResponse(w, r, http.StatusNotFound, "rvInfo does not exist", "No rvInfo found to update", "rvInfo does not exist")
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid rvInfo", err.Error(), "Invalid rvInfo")
			return
		}
		slog.Error("Error updating rvInfo", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error updating rvInfo", err.Error(), "Error updating rvInfo")
		return
	}

	slog.Debug("rvInfo updated")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(rvInfo)
}
