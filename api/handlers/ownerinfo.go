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
	slog.Debug("Fetching ownerInfo")
	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Debug("No ownerInfo found")
			http.Error(w, "No ownerInfo found", http.StatusNotFound)
		} else {
			slog.Debug("Error fetching ownerInfo", "error", err)
			http.Error(w, "Error fetching ownerInfo", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(ownerInfoJSON)
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchOwnerInfoJSON(); err == nil {
		slog.Debug("ownerInfo already exists, cannot create new entry")
		http.Error(w, "ownerInfo already exists", http.StatusConflict)
		return
	} else if err != sql.ErrNoRows {
		slog.Debug("Error checking ownerInfo existence", "error", err)
		http.Error(w, "Error processing ownerInfo", http.StatusInternalServerError)
		return
	}

	if err := db.InsertOwnerInfo(ownerInfo); err != nil {
		slog.Debug("Error inserting ownerInfo", "error", err)
		http.Error(w, "Error inserting ownerInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerInfo created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(ownerInfo)
}

func updateOwnerInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}
	if _, err := db.FetchOwnerInfoJSON(); err == sql.ErrNoRows {
		slog.Debug("ownerInfo does not exist, cannot update")
		http.Error(w, "ownerInfo does not exist", http.StatusNotFound)
		return
	} else if err != nil {
		slog.Debug("Error checking ownerInfo existence", "error", err)
		http.Error(w, "Error processing ownerInfo", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateOwnerInfo(ownerInfo); err != nil {
		slog.Debug("Error updating ownerInfo", "error", err)
		http.Error(w, "Error updating ownerInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerInfo updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(ownerInfo)
}
