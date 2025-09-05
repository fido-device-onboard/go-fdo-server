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

func OwnerInfoHandler(w http.ResponseWriter, r *http.Request) {
	var mu sync.Mutex
	slog.Debug("Received OwnerInfo request", "method", r.Method, "path", r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		getOwnerInfo(w, r)
	case http.MethodPost:
		createOwnerInfo(w, r, &mu)
	case http.MethodPut:
		updateOwnerInfo(w, r, &mu)
	default:
		slog.Debug("Method not allowed", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getOwnerInfo(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching ownerinfo data")
	ownerDataJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Debug("No ownerData found")
			http.Error(w, "No ownerData found", http.StatusNotFound)
		} else {
			slog.Debug("Error fetching ownerData", "error", err)
			http.Error(w, "Error fetching ownerData", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(ownerDataJSON)
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchOwnerInfoJSON(); err == nil {
		slog.Debug("ownerData already exists, cannot create new entry")
		http.Error(w, "ownerData already exists", http.StatusConflict)
		return
	} else if err != sql.ErrNoRows {
		slog.Debug("Error checking ownerData existence", "error", err)
		http.Error(w, "Error processing ownerData", http.StatusInternalServerError)
		return
	}

	if err := db.InsertOwnerInfo(ownerData); err != nil {
		slog.Debug("Error inserting ownerData", "error", err)
		http.Error(w, "Error inserting ownerData", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerData created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(ownerData)
}

func updateOwnerInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}
	if _, err := db.FetchOwnerInfoJSON(); err == sql.ErrNoRows {
		slog.Debug("ownerData does not exist, cannot update")
		http.Error(w, "ownerData does not exist", http.StatusNotFound)
		return
	} else if err != nil {
		slog.Debug("Error checking ownerData existence", "error", err)
		http.Error(w, "Error processing ownerData", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateOwnerInfo(ownerData); err != nil {
		slog.Debug("Error updating ownerData", "error", err)
		http.Error(w, "Error updating ownerData", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerData updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(ownerData)
}
