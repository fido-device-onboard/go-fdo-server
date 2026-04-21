// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package deviceca

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

// Server implements the ServerInterface for device CA certificate management
type Server struct {
	TrustedDeviceCACerts *state.TrustedDeviceCACertsState
}

func NewServer(state *state.TrustedDeviceCACertsState) Server {
	return Server{TrustedDeviceCACerts: state}
}

var _ StrictServerInterface = (*Server)(nil)

// ListTrustedDeviceCACerts lists all trusted device CA certificates with pagination, filtering, and sorting
func (s *Server) ListTrustedDeviceCACerts(ctx context.Context, request ListTrustedDeviceCACertsRequestObject) (ListTrustedDeviceCACertsResponseObject, error) {
	// Set defaults
	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	sortBy := "created_at"
	if request.Params.SortBy != nil {
		switch *request.Params.SortBy {
		case CreatedAt:
			sortBy = "created_at"
		case NotAfter:
			sortBy = "not_after"
		case NotBefore:
			sortBy = "not_before"
		case Subject:
			sortBy = "subject"
		case Issuer:
			sortBy = "issuer"
		}
	}

	sortOrder := "asc"
	if request.Params.SortOrder != nil {
		switch *request.Params.SortOrder {
		case Asc:
			sortOrder = "asc"
		case Desc:
			sortOrder = "desc"
		}
	}

	// Convert validityStatus to state.ValidityStatus
	var stateValidityStatus *state.ValidityStatus
	if request.Params.ValidityStatus != nil {
		status := state.ValidityStatus(string(*request.Params.ValidityStatus))
		stateValidityStatus = &status
	}

	// Call the database layer with all filters
	certs, total, err := s.TrustedDeviceCACerts.ListDeviceCACertificates(ctx, limit, offset, request.Params.Issuer, request.Params.Subject, request.Params.Search, stateValidityStatus, sortBy, sortOrder)
	if err != nil {
		slog.Error("Failed to list device CA certificates", "error", err)
		return ListTrustedDeviceCACerts500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to list device CA certificates",
			},
		}, nil
	}

	preferredContentType := middleware.PreferredContentType(ctx)

	// Return response based on content negotiation
	if preferredContentType == "application/x-pem-file" {
		// Concatenate all PEM certificates
		var pemData strings.Builder
		for _, cert := range certs {
			pemData.WriteString(cert.PEM)
		}

		pemBytes := pemData.String()
		pemReader := strings.NewReader(pemBytes)
		return ListTrustedDeviceCACerts200ApplicationxPemFileResponse{
			Body:          pemReader,
			ContentLength: int64(len(pemBytes)),
		}, nil
	}

	// Default to JSON format
	data := make([]TrustedDeviceCACert, len(certs))
	for i, cert := range certs {
		data[i] = TrustedDeviceCACert{
			Fingerprint: cert.Fingerprint,
			Pem:         cert.PEM,
			Subject:     cert.Subject,
			Issuer:      cert.Issuer,
			NotBefore:   cert.NotBefore,
			NotAfter:    cert.NotAfter,
			CreatedAt:   cert.CreatedAt,
		}
	}

	return ListTrustedDeviceCACerts200JSONResponse(TrustedDeviceCACertsPaginated{
		Total:  int(total),
		Limit:  limit,
		Offset: offset,
		Certs:  data,
	}), nil
}

// ImportTrustedDeviceCACerts creates one or more trusted device CA certificates (idempotent)
func (s *Server) ImportTrustedDeviceCACerts(ctx context.Context, request ImportTrustedDeviceCACertsRequestObject) (ImportTrustedDeviceCACertsResponseObject, error) {
	pemData, err := io.ReadAll(request.Body)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return ImportTrustedDeviceCACerts413JSONResponse(components.Error{
				Message: "Request payload too large",
			}), nil
		}
		slog.Error("Failed to read request body", "error", err)
		return ImportTrustedDeviceCACerts400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Import the certificates using the idempotent method
	stats, err := s.TrustedDeviceCACerts.ImportDeviceCACertificatesFromPEM(ctx, string(pemData))
	if err != nil {
		slog.Error("Failed to import device CA certificates", "error", err)
		return ImportTrustedDeviceCACerts500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to import certificates",
			},
		}, nil
	}

	slog.Debug("Importing device CA certificates", "messages", stats.Messages)

	// Convert imported certificates to response format
	certificates := make([]TrustedDeviceCACert, len(stats.Certificates))
	for i, cert := range stats.Certificates {
		certificates[i] = TrustedDeviceCACert{
			Fingerprint: cert.Fingerprint,
			Pem:         cert.PEM,
			Subject:     cert.Subject,
			Issuer:      cert.Issuer,
			NotBefore:   cert.NotBefore,
			NotAfter:    cert.NotAfter,
			CreatedAt:   cert.CreatedAt,
		}
	}

	return ImportTrustedDeviceCACerts200JSONResponse(TrustedDeviceCACertsImportResult{
		Detected:  stats.Detected,
		Imported:  stats.Imported,
		Skipped:   stats.Skipped,
		Malformed: stats.Malformed,
		Messages:  stats.Messages,
		Certs:     certificates,
	}), nil
}

// GetTrustedDeviceCACertByFingerprint retrieves a device CA certificate by fingerprint
func (s *Server) GetTrustedDeviceCACertByFingerprint(ctx context.Context, request GetTrustedDeviceCACertByFingerprintRequestObject) (GetTrustedDeviceCACertByFingerprintResponseObject, error) {
	cert, err := s.TrustedDeviceCACerts.GetDeviceCACertificate(ctx, request.Fingerprint)
	if err != nil {
		if errors.Is(err, state.ErrDeviceCACertNotFound) {
			slog.Debug("Device CA certificate not found", "fingerprint", request.Fingerprint)
			return GetTrustedDeviceCACertByFingerprint404JSONResponse{
				NotFound: components.NotFound{
					Message: "Device CA certificate not found",
				},
			}, nil
		}
		slog.Error("Failed to get device CA certificate", "error", err, "fingerprint", request.Fingerprint)
		return GetTrustedDeviceCACertByFingerprint500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to retrieve certificate",
			},
		}, nil
	}

	preferredContentType := middleware.PreferredContentType(ctx)

	// Return response based on content negotiation
	if preferredContentType == "application/x-pem-file" {
		// Return PEM format
		pemReader := strings.NewReader(cert.PEM)
		return GetTrustedDeviceCACertByFingerprint200ApplicationxPemFileResponse{
			Body:          pemReader,
			ContentLength: int64(len(cert.PEM)),
		}, nil
	}

	// Default to JSON format
	return GetTrustedDeviceCACertByFingerprint200JSONResponse(TrustedDeviceCACert{
		Fingerprint: cert.Fingerprint,
		Pem:         cert.PEM,
		Subject:     cert.Subject,
		Issuer:      cert.Issuer,
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		CreatedAt:   cert.CreatedAt,
	}), nil
}

// DeleteTrustedDeviceCACert deletes a device CA certificate by fingerprint
func (s *Server) DeleteTrustedDeviceCACert(ctx context.Context, request DeleteTrustedDeviceCACertRequestObject) (DeleteTrustedDeviceCACertResponseObject, error) {
	err := s.TrustedDeviceCACerts.DeleteDeviceCACertificate(ctx, request.Fingerprint)
	if err != nil {
		if errors.Is(err, state.ErrDeviceCACertNotFound) {
			slog.Debug("Device CA certificate not found", "fingerprint", request.Fingerprint)
			return DeleteTrustedDeviceCACert404JSONResponse{
				NotFound: components.NotFound{
					Message: "Device CA certificate not found",
				},
			}, nil
		}
		slog.Error("Failed to delete device CA certificate", "error", err, "fingerprint", request.Fingerprint)
		return DeleteTrustedDeviceCACert500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to delete certificate",
			},
		}, nil
	}

	return DeleteTrustedDeviceCACert204Response{}, nil
}
