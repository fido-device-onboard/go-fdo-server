// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"io"
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getOwnerInfo(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching ownerInfo")
	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No ownerInfo found")
			http.Error(w, "No ownerInfo found", http.StatusNotFound)
		} else {
			slog.Error("Error fetching ownerInfo", "error", err)
			http.Error(w, "Error fetching ownerInfo", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, ownerInfoJSON)
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.InsertOwnerInfo(ownerInfo); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("ownerInfo already exists (constraint)", "error", err)
			http.Error(w, "ownerInfo already exists", http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			http.Error(w, "Invalid ownerInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error inserting ownerInfo", "error", err)
		http.Error(w, "Error inserting ownerInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerInfo created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeResponse(w, ownerInfo)
}

func updateOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateOwnerInfo(ownerInfo); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("ownerInfo does not exist, cannot update")
			http.Error(w, "ownerInfo does not exist", http.StatusNotFound)
			return
		}
		if errors.Is(err, db.ErrInvalidOwnerInfo) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			http.Error(w, "Invalid ownerInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error updating ownerInfo", "error", err)
		http.Error(w, "Error updating ownerInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerInfo updated")

	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, ownerInfo)
}
