// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/api/openapi"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"gorm.io/gorm"
)

// Content type constants
const (
	ContentTypeJSON = "application/json"
	ContentTypePEM  = "application/x-pem-file"
	ContentTypeForm = "application/x-www-form-urlencoded"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// WriteJSONVoucher writes a voucher response in JSON format
func WriteJSONVoucher(w http.ResponseWriter, voucherData []byte, encoding string, guid []byte) error {
	response := openapi.VoucherResponse{
		Voucher:  base64.StdEncoding.EncodeToString(voucherData),
		Encoding: encoding,
		Guid:     hex.EncodeToString(guid),
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	return json.NewEncoder(w).Encode(response)
}

// WriteJSONError writes a standard error response in JSON format
func WriteJSONError(w http.ResponseWriter, statusCode int, errorMsg, details string) {
	response := ErrorResponse{
		Error:   errorMsg,
		Details: details,
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// WriteJSONVoucherInsertResponse writes a voucher insert response in JSON format
func WriteJSONVoucherInsertResponse(w http.ResponseWriter, response openapi.VoucherInsertResponse) error {
	w.Header().Set("Content-Type", ContentTypeJSON)
	if response.Errors != nil && len(*response.Errors) > 0 {
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
	if accept == ContentTypePEM {
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

// AppendError adds an error message to the voucher insert response
func AppendError(response *openapi.VoucherInsertResponse, format string, args ...interface{}) {
	if response.Errors == nil {
		errors := make([]string, 0)
		response.Errors = &errors
	}
	*response.Errors = append(*response.Errors, fmt.Sprintf(format, args...))
}

// ReadRequestBody reads the request body and handles errors consistently
func ReadRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Failed to read request body", err.Error(), "Error reading body")
		return nil, false
	}
	return body, true
}

// HandleDBError handles GORM errors consistently (duplicate key or not found)
func HandleDBError(w http.ResponseWriter, r *http.Request, entityName string, err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		slog.Error(entityName+" already exists (constraint)", "error", err)
		WriteErrorResponse(w, r, http.StatusConflict, entityName+" already exists", "A "+entityName+" with the same identifier already exists", entityName+" already exists")
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("No " + entityName + " found")
		WriteErrorResponse(w, r, http.StatusNotFound, "No "+entityName+" found", entityName+" has not been configured", "No "+entityName+" found")
		return true
	}
	return false
}

// HandleEncodingError writes a consistent encoding error response
func HandleEncodingError(w http.ResponseWriter, r *http.Request, format string, err error) {
	WriteErrorResponse(w, r, http.StatusInternalServerError, "Failed to encode "+format, err.Error(), "Failed to encode "+format)
}
