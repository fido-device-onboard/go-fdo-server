// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"gorm.io/gorm"
)

func OwnerInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Enhanced logging for Testing Farm debugging
	slog.Info("OwnerInfo request received", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "user_agent", r.UserAgent())

	switch r.Method {
	case http.MethodGet:
		slog.Info("Processing OwnerInfo GET request")
		getOwnerInfo(w, r)
	case http.MethodPost:
		slog.Info("Processing OwnerInfo POST request")
		createOwnerInfo(w, r)
	case http.MethodPut:
		slog.Info("Processing OwnerInfo PUT request")
		updateOwnerInfo(w, r)
	default:
		slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
		WriteErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", "HTTP method "+r.Method+" is not supported for this endpoint", "Method not allowed")
	}
}

func getOwnerInfo(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Fetching ownerInfo")

	// Defensive database validation for systemd environments
	// Add retry logic and better error handling for timing issues
	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		// Log detailed error information for systemd debugging
		slog.Error("Error fetching ownerInfo", "error", err, "error_type", fmt.Sprintf("%T", err))

		if HandleDBError(w, r, "ownerInfo", err) {
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error fetching ownerInfo", err.Error(), "Error fetching ownerInfo")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfoJSON)
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request) {
	slog.Info("Starting createOwnerInfo process")

	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		slog.Error("Failed to read request body in createOwnerInfo")
		return
	}

	slog.Info("Request body read successfully", "body_length", len(ownerInfo))

	if err := db.InsertOwnerInfo(ownerInfo); err != nil {
		// Enhanced error logging for systemd debugging
		slog.Error("Error inserting ownerInfo", "error", err, "error_type", fmt.Sprintf("%T", err))

		if HandleDBError(w, r, "ownerInfo", err) {
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid ownerInfo", err.Error(), "Invalid ownerInfo")
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error inserting ownerInfo", err.Error(), "Error inserting ownerInfo")
		return
	}

	slog.Info("ownerInfo created successfully")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(ownerInfo)
}

func updateOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.UpdateOwnerInfo(ownerInfo); err != nil {
		// Enhanced error logging for systemd debugging
		slog.Error("Error updating ownerInfo", "error", err, "error_type", fmt.Sprintf("%T", err))

		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteErrorResponse(w, r, http.StatusNotFound, "ownerInfo does not exist", "No ownerInfo found to update", "ownerInfo does not exist")
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid ownerInfo", err.Error(), "Invalid ownerInfo")
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error updating ownerInfo", err.Error(), "Error updating ownerInfo")
		return
	}

	slog.Debug("ownerInfo updated")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfo)
}
