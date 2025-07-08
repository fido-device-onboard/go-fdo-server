// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"net/http"
	"path"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"

	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

func To0Handler(rvInfo *[][]protocol.RvInstruction, state *sqlite.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to0Guid := path.Base(r.URL.Path)
		if to0Guid == "" {
			http.Error(w, "GUID is required", http.StatusBadRequest)
			return
		}

		if !utils.IsValidGUID(to0Guid) {
			http.Error(w, "GUID is not a valid GUID", http.StatusBadRequest)
			return
		}

		if to0Guid != "" {
			err := to0.RegisterRvBlob(*rvInfo, to0Guid, state)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(to0Guid))
	}
}
