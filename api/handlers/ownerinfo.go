// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"database/sql"
	"encoding/json"
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
		getOwnerData(w, r)
	case http.MethodPost:
		createOwnerData(w, r, &mu)
	case http.MethodPut:
		updateOwnerData(w, r, &mu)
	default:
		slog.Debug("Method not allowed", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getOwnerData(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching ownerinfo data")
	ownerData, err := db.FetchData("owner_info")
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
	json.NewEncoder(w).Encode(ownerData)
}

func createOwnerData(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerData, err := parseRequestBody(r)
	if err != nil {
		slog.Debug("Error parsing request body", "error", err)
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if exists, err := db.CheckDataExists("owner_info"); err != nil {
		slog.Debug("Error checking ownerData existence", "error", err)
		http.Error(w, "Error processing ownerData", http.StatusInternalServerError)
		return
	} else if exists {
		slog.Debug("ownerData already exists, cannot create new entry")
		http.Error(w, "ownerData already exists", http.StatusConflict)
		return
	}

	if err := db.InsertData(ownerData, "owner_info"); err != nil {
		slog.Debug("Error inserting ownerData", "error", err)
		http.Error(w, "Error inserting ownerData", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerData created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ownerData)
}

func updateOwnerData(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	ownerData, err := parseRequestBody(r)
	if err != nil {
		slog.Debug("Error parsing request body", "error", err)
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	if exists, err := db.CheckDataExists("owner_info"); err != nil {
		slog.Debug("Error checking ownerData existence", "error", err)
		http.Error(w, "Error processing ownerData", http.StatusInternalServerError)
		return
	} else if !exists {
		slog.Debug("No ownerData found to update")
		http.Error(w, "No ownerData found", http.StatusNotFound)
		return
	}

	if err := db.UpdateDataInDB(ownerData, "owner_info"); err != nil {
		slog.Debug("Error updating ownerData", "error", err)
		http.Error(w, "Error updating ownerData", http.StatusInternalServerError)
		return
	}

	slog.Debug("ownerData updated")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ownerData)
}
