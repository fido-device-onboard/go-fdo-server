// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"

	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

func GetVoucherHandler(w http.ResponseWriter, r *http.Request) {
	guidHex := r.URL.Query().Get("guid")
	if guidHex == "" {
		http.Error(w, "GUID is required", http.StatusBadRequest)
		return
	}

	if !utils.IsValidGUID(guidHex) {
		http.Error(w, fmt.Sprintf("Invalid GUID: %s", guidHex), http.StatusBadRequest)
		return
	}

	guid, err := hex.DecodeString(guidHex)
	if err != nil {
		http.Error(w, "Invalid GUID format", http.StatusBadRequest)
		return
	}

	voucher, err := db.FetchVoucher(guid)
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Debug("Voucher not found", "GUID", guidHex)
			http.Error(w, "Voucher not found", http.StatusNotFound)
		} else {
			slog.Debug("Error querying database", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	ownerKeys, err := db.FetchOwnerKeys()
	if err != nil {
		slog.Debug("Error querying owner_keys", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		Voucher   db.Voucher    `json:"voucher"`
		OwnerKeys []db.OwnerKey `json:"owner_keys"`
	}{
		Voucher:   voucher,
		OwnerKeys: ownerKeys,
	}

	data, err := json.Marshal(response)
	if err != nil {
		slog.Debug("Error marshalling JSON", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func InsertVoucherHandler(rvInfo *[][]protocol.RvInstruction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Voucher   db.Voucher    `json:"voucher"`
			OwnerKeys []db.OwnerKey `json:"owner_keys"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		guidHex := hex.EncodeToString(request.Voucher.GUID)
		slog.Debug("Inserting voucher", "GUID", guidHex)

		if err := db.InsertVoucher(request.Voucher); err != nil {
			slog.Debug("Error inserting into database", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := db.UpdateOwnerKeys(request.OwnerKeys); err != nil {
			slog.Debug("Error updating owner key in database", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		newRvInfo, err := rvinfo.GetRvInfoFromVoucher(request.Voucher.CBOR)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		*rvInfo = newRvInfo
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(guidHex))
	}
}
