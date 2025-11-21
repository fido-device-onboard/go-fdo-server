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

func OwnerInfoHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Received OwnerInfo request", "method", r.Method, "path", r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		getOwnerInfo(w, r)
	case http.MethodPost:
		createOwnerInfo(w, r)
	case http.MethodPut:
		updateOwnerInfo(w, r)
	default:
		slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
		WriteErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", "HTTP method "+r.Method+" is not supported for this endpoint", "Method not allowed")
	}
}

func getOwnerInfo(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Fetching ownerInfo")
	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		if HandleDBError(w, r, "ownerInfo", err) {
			return
		}
		slog.Error("Error fetching ownerInfo", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error fetching ownerInfo", err.Error(), "Error fetching ownerInfo")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfoJSON)
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.InsertOwnerInfo(ownerInfo); err != nil {
		if HandleDBError(w, r, "ownerInfo", err) {
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid ownerInfo", err.Error(), "Invalid ownerInfo")
			return
		}
		slog.Error("Error inserting ownerInfo", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error inserting ownerInfo", err.Error(), "Error inserting ownerInfo")
		return
	}

	slog.Debug("ownerInfo created")

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("ownerInfo does not exist, cannot update")
			WriteErrorResponse(w, r, http.StatusNotFound, "ownerInfo does not exist", "No ownerInfo found to update", "ownerInfo does not exist")
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid ownerInfo", err.Error(), "Invalid ownerInfo")
			return
		}
		slog.Error("Error updating ownerInfo", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error updating ownerInfo", err.Error(), "Error updating ownerInfo")
		return
	}

	slog.Debug("ownerInfo updated")

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfo)
}
