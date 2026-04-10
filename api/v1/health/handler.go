// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"net/http"
)

type HealthResponse struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// HealthHandler responds with the version and status
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeResponse(w, []byte("Method not allowed"))
		return
	}
	response := HealthResponse{
		Version: "1.1",
		Status:  "OK",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	encodeJSONResponse(w, response)
}
