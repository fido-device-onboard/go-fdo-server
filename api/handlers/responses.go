// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// VoucherResponse represents a voucher in JSON format
type VoucherResponse struct {
	Voucher  string `json:"voucher"`
	Encoding string `json:"encoding"`
	GUID     string `json:"guid"`
}

// VoucherInsertResponse represents the result of voucher insertion
type VoucherInsertResponse struct {
	Processed int      `json:"processed"`
	Inserted  int      `json:"inserted"`
	Errors    []string `json:"errors,omitempty"`
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// WriteJSONVoucher writes a voucher response in JSON format
func WriteJSONVoucher(w http.ResponseWriter, voucherData []byte, encoding string, guid []byte) error {
	response := VoucherResponse{
		Voucher:  base64.StdEncoding.EncodeToString(voucherData),
		Encoding: encoding,
		GUID:     hex.EncodeToString(guid),
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(response)
}

// WriteJSONError writes a standard error response in JSON format
func WriteJSONError(w http.ResponseWriter, statusCode int, errorMsg, details string) {
	response := ErrorResponse{
		Error:   errorMsg,
		Details: details,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// WriteJSONVoucherInsertResponse writes a voucher insert response in JSON format
func WriteJSONVoucherInsertResponse(w http.ResponseWriter, response VoucherInsertResponse) error {
	w.Header().Set("Content-Type", "application/json")
	if len(response.Errors) > 0 && response.Inserted == 0 {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	return json.NewEncoder(w).Encode(response)
}

// ShouldReturnJSON checks if the client prefers JSON response
// This allows for backward compatibility with existing PEM clients
func ShouldReturnJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")

	// If client specifically requests PEM, honor it for backward compatibility
	if accept == "application/x-pem-file" {
		return false
	}

	// Default to JSON for everything else (including no preference)
	return true
}

// WriteErrorResponse writes an error response in JSON or text format based on Accept header
func WriteErrorResponse(w http.ResponseWriter, r *http.Request, statusCode int, jsonMsg, jsonDetails, textMsg string) {
	if ShouldReturnJSON(r) {
		WriteJSONError(w, statusCode, jsonMsg, jsonDetails)
	} else {
		http.Error(w, textMsg, statusCode)
	}
}

// ValidateAndDecodeGUID validates a GUID string and returns the decoded bytes
// Returns the decoded GUID bytes or an error response written to the writer
func ValidateAndDecodeGUID(w http.ResponseWriter, r *http.Request, guidHex string) ([]byte, bool) {
	if !utils.IsValidGUID(guidHex) {
		WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid GUID", "GUID must be 32 hexadecimal characters", "Invalid GUID")
		return nil, false
	}

	guid, err := hex.DecodeString(guidHex)
	if err != nil {
		WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid GUID format", err.Error(), "Invalid GUID format")
		return nil, false
	}

	return guid, true
}
