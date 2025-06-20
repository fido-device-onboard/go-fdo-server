// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"

	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/cbor"
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

	w.Header().Set("Content-Type", "application/x-pem-file")
	if err := pem.Encode(w, &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: voucher.CBOR,
	}); err != nil {
		slog.Debug("Error encoding voucher", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func InsertVoucherHandler(rvInfo *[][]protocol.RvInstruction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failure to read the request body", http.StatusInternalServerError)
			return
		}

		for block, rest := pem.Decode(body); block != nil; block, rest = pem.Decode(rest) {
			if block.Type != "OWNERSHIP VOUCHER" {
				slog.Debug("Got unknown label type", "type", block.Type)
				continue
			}
			var ov fdo.Voucher
			if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
				slog.Debug("Unable to decode cbor", "block", block.Bytes)
				http.Error(w, "Unable to decode cbor", http.StatusInternalServerError)
				return
			}

			// TODO: https://github.com/fido-device-onboard/go-fdo-server/issues/18
			slog.Debug("Inserting voucher", "GUID", ov.Header.Val.GUID)

			if err := db.InsertVoucher(db.Voucher{GUID: ov.Header.Val.GUID[:], CBOR: block.Bytes}); err != nil {
				slog.Debug("Error inserting into database", "error", err.Error())
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			*rvInfo = ov.Header.Val.RvInfo
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}
}
