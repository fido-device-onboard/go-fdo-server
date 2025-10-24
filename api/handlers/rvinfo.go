// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"gorm.io/gorm"
)

func RvInfoHandler() http.HandlerFunc {
	var mu sync.Mutex
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Received RV request", "method", r.Method, "path", r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			getRvInfo(w, r)
		case http.MethodPost:
			createRvInfo(w, r, &mu)
		case http.MethodPut:
			updateRvInfo(w, r, &mu)
		default:
			slog.Debug("Method not allowed", "method", r.Method, "path", r.URL.Path)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getRvInfo(w http.ResponseWriter, _ *http.Request) {
	slog.Debug("Fetching rvInfo")
	rvInfoJSON, err := db.FetchRvInfoJSON()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Debug("No rvInfo found")
			http.Error(w, "No rvInfo found", http.StatusNotFound)
		} else {
			slog.Debug("Error fetching rvInfo", "error", err)
			http.Error(w, "Error fetching rvInfo", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvInfoJSON)
}

func createRvInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchRvInfoJSON(); err == nil {
		slog.Debug("rvInfo already exists, cannot create new entry")
		http.Error(w, "rvInfo already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Debug("Error checking rvInfo existence", "error", err)
		http.Error(w, "Error processing rvInfo", http.StatusInternalServerError)
		return
	}

	if err := db.InsertRvInfo(rvInfo); err != nil {
		slog.Debug("Error inserting rvInfo", "error", err)
		http.Error(w, "Error inserting rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(rvInfo)
}

func updateRvInfo(w http.ResponseWriter, r *http.Request, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Debug("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if _, err := db.FetchRvInfoJSON(); errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Debug("rvInfo does not exist, cannot update")
		http.Error(w, "rvInfo does not exist", http.StatusNotFound)
		return
	} else if err != nil {
		slog.Debug("Error checking rvInfo existence", "error", err)
		http.Error(w, "Error processing rvInfo", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateRvInfo(rvInfo); err != nil {
		slog.Debug("Error updating rvInfo", "error", err)
		http.Error(w, "Error updating rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo updated")

	w.Header().Set("Content-Type", "application/json")
	w.Write(rvInfo)
}
