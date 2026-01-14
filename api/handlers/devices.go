// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// OwnerDevicesHandler returns the list of devices known to the owner service,
// combining voucher metadata with onboarding (TO2) state.
// Exposed as GET /api/v1/owner/devices.
func OwnerDevicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slog.Debug("Listing owner devices")

	filters := make(map[string]interface{})
	if guidHex := r.URL.Query().Get("old_guid"); guidHex != "" {
		if !utils.IsValidGUID(guidHex) {
			http.Error(w, "Invalid GUID", http.StatusBadRequest)
			return
		}
		decoded, err := hex.DecodeString(guidHex)
		if err != nil {
			http.Error(w, "Invalid GUID format", http.StatusBadRequest)
			return
		}
		filters["old_guid"] = decoded
	}

	devices, err := db.ListDevices(filters)
	if err != nil {
		slog.Error("Error listing devices", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(devices); err != nil {
		slog.Error("Error encoding devices response", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
