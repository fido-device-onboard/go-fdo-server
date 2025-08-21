// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/hex"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

type To0HandlerState struct {
	VoucherState fdo.OwnerVoucherPersistentState
	KeyState     fdo.OwnerKeyPersistentState
	UseTLS       bool
}

func To0Handler(state *To0HandlerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to0Guid := r.PathValue("guid")

		if !utils.IsValidGUID(to0Guid) {
			http.Error(w, "GUID is not a valid GUID", http.StatusBadRequest)
			return
		}

		guid, err := hex.DecodeString(to0Guid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ov, err := db.FetchVoucher(guid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rvInfo, err := rvinfo.GetRvInfoFromVoucher(ov.CBOR)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := to0.RegisterRvBlob(rvInfo, to0Guid, state.VoucherState, state.KeyState, state.UseTLS); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(to0Guid))
	}
}
