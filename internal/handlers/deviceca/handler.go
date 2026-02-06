// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package deviceca

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/elnormous/contenttype"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
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

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	contentTypeKey contextKey = "preferred-content-type"
)

// ContentNegotiationMiddleware extracts the Accept header from the request
// and stores the preferred content type in the context using RFC 7231-compliant
// content negotiation with quality factor support
func ContentNegotiationMiddleware(f StrictHandlerFunc, operationID string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
		// Extract Accept header
		acceptHeader := r.Header.Get("Accept")

		// Determine preferred content type based on Accept header
		// Default to application/json for all endpoints
		preferredContentType := "application/json"

		if acceptHeader != "" {
			// Available media types this endpoint can produce
			availableMediaTypes := []contenttype.MediaType{
				contenttype.NewMediaType("application/json"),
				contenttype.NewMediaType("application/x-pem-file"),
			}

			// Parse and negotiate the best match based on Accept header
			// This properly handles quality factors (q values)
			accepted, _, err := contenttype.GetAcceptableMediaType(r, availableMediaTypes)
			if err == nil {
				// Successfully negotiated a content type
				preferredContentType = strings.ToLower(accepted.String())
			}
			// If negotiation fails, keep the default (application/json)
		}

		// Add preferred content type to context
		ctx = context.WithValue(ctx, contentTypeKey, preferredContentType)

		// Call the next handler
		return f(ctx, w, r, request)
	}
}

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

	// Check preferred content type from context
	preferredContentType, _ := ctx.Value(contentTypeKey).(string)

	// Return response based on content negotiation
	if preferredContentType == "application/x-pem-file" {
		// Concatenate all PEM certificates
		var pemData strings.Builder
		for _, cert := range certs {
			pemData.WriteString(cert.PEM)
		}

		pemBytes := pemData.String()
		pemReader := bytes.NewReader([]byte(pemBytes))
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
	// Read the PEM data from the request body with size limit (1MB)
	const maxSize = 1048576 // 1MB
	pemData, err := io.ReadAll(io.LimitReader(request.Body, maxSize+1))
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		return ImportTrustedDeviceCACerts400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Check payload size
	if len(pemData) > maxSize {
		slog.Warn("Request payload too large", "size", len(pemData), "max", maxSize)
		return ImportTrustedDeviceCACerts413JSONResponse(components.Error{
			Message: fmt.Sprintf("Request payload exceeds maximum size of %d bytes", maxSize),
		}), nil
	}

	// Import the certificates using the idempotent method
	stats, err := s.TrustedDeviceCACerts.ImportDeviceCACertificates(ctx, string(pemData))
	if err != nil {
		slog.Error("Failed to import device CA certificates", "error", err)
		return ImportTrustedDeviceCACerts500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to import certificates",
			},
		}, nil
	}

	slog.Debug("Importing device CA certificates", "messages", stats.Messages)

	// Reload the trusted device CA cert pool if any certificates were imported
	if stats.Imported > 0 {
		if err := s.TrustedDeviceCACerts.LoadTrustedDeviceCAs(ctx); err != nil {
			slog.Error("Failed to reload trusted device CA cert pool", "error", err)
			return ImportTrustedDeviceCACerts500JSONResponse{
				InternalServerError: components.InternalServerError{
					Message: "Failed to reload trusted device CA certificates",
				},
			}, nil
		}
		slog.Info("Reloaded trusted device CA cert pool", "imported", stats.Imported)
	}

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

	// Check preferred content type from context
	preferredContentType, _ := ctx.Value(contentTypeKey).(string)

	// Return response based on content negotiation
	if preferredContentType == "application/x-pem-file" {
		// Return PEM format
		pemReader := bytes.NewReader([]byte(cert.PEM))
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

	// Reload the trusted device CA cert pool after deletion
	if err := s.TrustedDeviceCACerts.LoadTrustedDeviceCAs(ctx); err != nil {
		slog.Error("Failed to reload trusted device CA cert pool", "error", err)
		return DeleteTrustedDeviceCACert500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to reload trusted device CA certificates",
			},
		}, nil
	}
	slog.Info("Reloaded trusted device CA cert pool after deletion", "fingerprint", request.Fingerprint)

	return DeleteTrustedDeviceCACert204Response{}, nil
}
