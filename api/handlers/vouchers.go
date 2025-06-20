// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"bytes"
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
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
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

	var ovBytes bytes.Buffer
	if err := pem.Encode(&ovBytes, &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: voucher.CBOR,
	}); err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(ovBytes.Bytes())
}

func InsertVoucherHandler(rvInfo *[][]protocol.RvInstruction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		blk, _ := pem.Decode(body)
		if blk == nil {
			return
		}
		if blk.Type != "OWNERSHIP VOUCHER" {
			return
		}
		var ov fdo.Voucher
		if err := cbor.Unmarshal(blk.Bytes, &ov); err != nil {
			return
		}

		slog.Debug("Inserting voucher", "GUID", ov.Header.Val.GUID)

		if err := db.InsertVoucher(db.Voucher{GUID: ov.Header.Val.GUID[:], CBOR: blk.Bytes}); err != nil {
			slog.Debug("Error inserting into database", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		newRvInfo, err := rvinfo.GetRvInfoFromVoucher(blk.Bytes)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		*rvInfo = newRvInfo
		w.WriteHeader(http.StatusOK)
	}
}
