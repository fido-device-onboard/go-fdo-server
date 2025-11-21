// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/api/openapi"
	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

// GetHealth responds with the version and status (OpenAPI interface method)
func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	response := openapi.HealthResponse{
		Version: version.VERSION,
		Status:  "OK",
	}
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// TEMPORARY: Legacy function for tests and compatibility
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method not allowed"))
		return
	}
	s := &Server{} // Empty server for compatibility
	s.GetHealth(w, r)
}
