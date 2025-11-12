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

func RVTO2AddrHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Received RVTO2Addr request", "method", r.Method, "path", r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		getRVTO2Addr(w, r)
	case http.MethodPost:
		createRVTO2Addr(w, r)
	case http.MethodPut:
		updateRVTO2Addr(w, r)
	default:
		slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getRVTO2Addr(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching rvto2Addr")
	rvto2AddrJSON, err := db.FetchRVTO2AddrJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No rvto2Addr found")
			http.Error(w, "No rvto2Addr found", http.StatusNotFound)
		} else {
			slog.Error("Error fetching rvto2Addr", "error", err)
			http.Error(w, "Error fetching rvto2Addr", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvto2AddrJSON)
}

func createRVTO2Addr(w http.ResponseWriter, r *http.Request) {
	rvto2Addr, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.InsertRVTO2Addr(rvto2Addr); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("rvto2Addr already exists (constraint)", "error", err)
			http.Error(w, "rvto2Addr already exists", http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrInvalidRVTO2Addr) {
			slog.Error("Invalid rvto2Addr payload", "error", err)
			http.Error(w, "Invalid rvto2Addr", http.StatusBadRequest)
			return
		}
		slog.Error("Error inserting rvto2Addr", "error", err)
		http.Error(w, "Error inserting rvto2Addr", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvto2Addr created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(rvto2Addr)
}

func updateRVTO2Addr(w http.ResponseWriter, r *http.Request) {
	rvto2Addr, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateRVTO2Addr(rvto2Addr); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("rvto2Addr does not exist, cannot update")
			http.Error(w, "rvto2Addr does not exist", http.StatusNotFound)
			return
		}
		if errors.Is(err, db.ErrInvalidRVTO2Addr) {
			slog.Error("Invalid rvto2Addr payload", "error", err)
			http.Error(w, "Invalid rvto2Addr", http.StatusBadRequest)
			return
		}
		slog.Error("Error updating rvto2Addr", "error", err)
		http.Error(w, "Error updating rvto2Addr", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvto2Addr updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvto2Addr)
}
