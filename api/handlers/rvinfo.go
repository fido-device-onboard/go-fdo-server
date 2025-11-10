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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getRvInfo(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching rvInfo")
	rvInfoJSON, err := db.FetchRvInfoJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No rvInfo found")
			http.Error(w, "No rvInfo found", http.StatusNotFound)
		} else {
			slog.Error("Error fetching rvInfo", "error", err)
			http.Error(w, "Error fetching rvInfo", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvInfoJSON)
}

func createRvInfo(w http.ResponseWriter, r *http.Request) {
	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.InsertRvInfo(rvInfo); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("rvInfo already exists (constraint)", "error", err)
			http.Error(w, "rvInfo already exists", http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			http.Error(w, "Invalid rvInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error inserting rvInfo", "error", err)
		http.Error(w, "Error inserting rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(rvInfo)
}

func updateRvInfo(w http.ResponseWriter, r *http.Request) {
	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateRvInfo(rvInfo); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("rvInfo does not exist, cannot update")
			http.Error(w, "rvInfo does not exist", http.StatusNotFound)
			return
		}
		if errors.Is(err, db.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			http.Error(w, "Invalid rvInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error updating rvInfo", "error", err)
		http.Error(w, "Error updating rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvInfo)
}
