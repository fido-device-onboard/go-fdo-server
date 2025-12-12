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
		switch r.Method {
		case http.MethodGet:
			getRvInfo(w, r)
		case http.MethodPost:
			createRvInfo(w, r)
		case http.MethodPut:
			updateRvInfo(w, r)
		default:
			slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
			WriteErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", "HTTP method "+r.Method+" is not supported for this endpoint", "Method not allowed")
		}
	}
}

// GetOwnerRedirect implements the owner redirect GET endpoint (OpenAPI interface method)
// Manages TO2 redirect addresses (RvTO2Addr), not rendezvous instructions (RvInstruction)
func (s *Server) GetOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Fetching owner redirect addresses (TO2)")

	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No owner redirect addresses found")
			WriteErrorResponse(w, r, http.StatusNotFound, "No owner redirect addresses found", "Owner redirect addresses have not been configured", "No owner redirect addresses found")
		} else {
			slog.Error("Error fetching owner redirect addresses", "error", err)
			WriteErrorResponse(w, r, http.StatusInternalServerError, "Error fetching owner redirect addresses", err.Error(), "Error fetching owner redirect addresses")
		}
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfoJSON)
}

// PostOwnerRedirect implements the owner redirect POST endpoint (OpenAPI interface method)
// Manages TO2 redirect addresses (RvTO2Addr), not rendezvous instructions (RvInstruction)
func (s *Server) PostOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	err := db.InsertOwnerInfo(ownerInfo)
	if err != nil {
		if HandleDBError(w, r, "owner redirect addresses", err) {
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid owner redirect addresses payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid owner redirect addresses", err.Error(), "Invalid owner redirect addresses")
			return
		}
		slog.Error("Error inserting owner redirect addresses", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error inserting owner redirect addresses", err.Error(), "Error inserting owner redirect addresses")
		return
	}

	slog.Debug("owner redirect addresses created")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(ownerInfo)
}

// PutOwnerRedirect implements the owner redirect PUT endpoint (OpenAPI interface method)
// Manages TO2 redirect addresses (RvTO2Addr), not rendezvous instructions (RvInstruction)
func (s *Server) PutOwnerRedirect(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	err := db.UpdateOwnerInfo(ownerInfo)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("owner redirect addresses do not exist, cannot update")
			WriteErrorResponse(w, r, http.StatusNotFound, "owner redirect addresses do not exist", "No owner redirect addresses found to update", "owner redirect addresses do not exist")
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid owner redirect addresses payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid owner redirect addresses", err.Error(), "Invalid owner redirect addresses")
			return
		}
		slog.Error("Error updating owner redirect addresses", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error updating owner redirect addresses", err.Error(), "Error updating owner redirect addresses")
		return
	}

	slog.Debug("owner redirect addresses updated")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfo)
}

// Original RvInfo functions for manufacturing server backward compatibility
func getRvInfo(w http.ResponseWriter, r *http.Request) {
	rvInfoJSON, err := db.FetchRvInfoJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteErrorResponse(w, r, http.StatusNotFound, "No rvInfo found", "rvInfo has not been configured", "No rvInfo found")
		} else {
			WriteErrorResponse(w, r, http.StatusInternalServerError, "Error fetching rvInfo", err.Error(), "Error fetching rvInfo")
		}
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(rvInfoJSON)
}

func createRvInfo(w http.ResponseWriter, r *http.Request) {
	rvInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.InsertRvInfo(rvInfo); err != nil {
		if HandleDBError(w, r, "rvInfo", err) {
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid rvInfo", err.Error(), "Invalid rvInfo")
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error inserting rvInfo", err.Error(), "Error inserting rvInfo")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(rvInfo)
}

func updateRvInfo(w http.ResponseWriter, r *http.Request) {
	rvInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.UpdateRvInfo(rvInfo); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteErrorResponse(w, r, http.StatusNotFound, "rvInfo does not exist", "No rvInfo found to update", "rvInfo does not exist")
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid rvInfo", err.Error(), "Invalid rvInfo")
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error updating rvInfo", err.Error(), "Error updating rvInfo")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(rvInfo)
}
