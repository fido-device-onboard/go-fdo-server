// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"sync"

	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
)

func RvInfoHandler() http.HandlerFunc {
	var mu sync.Mutex
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Received RV request", "method", r.Method, "path", r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			getRvData(w, r)
		case http.MethodPost:
			createRvData(w, r, &mu)
		case http.MethodPut:
			updateRvData(w, r, &mu)
		default:
			slog.Debug("Method not allowed", "method", r.Method, "path", r.URL.Path)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getRvData(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching rvData")
	rvDataJSON, err := db.FetchRvDataJSON()
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Debug("No rvData found")
			http.Error(w, "No rvData found", http.StatusNotFound)
		} else {
			slog.Debug("Error fetching rvData", "error", err)
			http.Error(w, "Error fetching rvData", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvDataJSON)
}

func createRvData(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	rvData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchRvDataJSON(); err == nil {
		slog.Debug("rvData already exists, cannot create new entry")
		http.Error(w, "rvData already exists", http.StatusConflict)
		return
	} else if err != sql.ErrNoRows {
		slog.Debug("Error checking rvData existence", "error", err)
		http.Error(w, "Error processing rvData", http.StatusInternalServerError)
		return
	}

	if err := db.InsertRvData(rvData); err != nil {
		slog.Debug("Error inserting rvData", "error", err)
		http.Error(w, "Error inserting rvData", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvData created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(rvData)
}

func updateRvData(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	rvData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchRvDataJSON(); err == sql.ErrNoRows {
		slog.Debug("rvData does not exist, cannot update")
		http.Error(w, "rvData does not exist", http.StatusNotFound)
		return
	} else if err != nil {
		slog.Debug("Error checking rvData existence", "error", err)
		http.Error(w, "Error processing rvData", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateRvData(rvData); err != nil {
		slog.Debug("Error updating rvData", "error", err)
		http.Error(w, "Error updating rvData", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvData updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvData)
}
