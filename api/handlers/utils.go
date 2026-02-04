// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeResponse handles error checking for http.ResponseWriter.Write
func writeResponse(w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

// encodeJSONResponse handles error checking for json.Encoder.Encode
func encodeJSONResponse(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
	}
}
