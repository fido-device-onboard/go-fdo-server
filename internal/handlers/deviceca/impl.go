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

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"gorm.io/gorm"
)

// DeviceCACertStore implements the ServerInterface for device CA certificate management
type DeviceCACertStore struct {
	State *db.State
}

var _ StrictServerInterface = (*DeviceCACertStore)(nil)

// ListTrustedDeviceCACerts lists all trusted device CA certificates with pagination, filtering, and sorting
func (s *DeviceCACertStore) ListTrustedDeviceCACerts(ctx context.Context, request ListTrustedDeviceCACertsRequestObject) (ListTrustedDeviceCACertsResponseObject, error) {
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
		}
	}

	sortOrder := "asc"
	if request.Params.SortOrder != nil {
		sortOrder = string(*request.Params.SortOrder)
	}

	// Call the database layer
	certs, total, err := s.State.ListDeviceCACertificates(ctx, limit, offset, request.Params.Issuer, sortBy, sortOrder)
	if err != nil {
		slog.Error("Failed to list device CA certificates", "error", err)
		return ListTrustedDeviceCACerts500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to list device CA certificates",
			},
		}, nil
	}

	// Convert to response format
	data := make([]TrustedDeviceCACert, len(certs))
	for i, cert := range certs {
		data[i] = TrustedDeviceCACert{
			Id:        &cert.ID,
			Pem:       &cert.PEM,
			Subject:   &cert.Subject,
			Issuer:    &cert.Issuer,
			NotBefore: &cert.NotBefore,
			NotAfter:  &cert.NotAfter,
			CreatedAt: &cert.CreatedAt,
			UpdatedAt: &cert.UpdatedAt,
		}
	}

	totalInt := int(total)
	return ListTrustedDeviceCACerts200JSONResponse{
		Total:  &totalInt,
		Limit:  &limit,
		Offset: &offset,
		Data:   &data,
	}, nil
}

// CreateTrustedDeviceCACert creates a new trusted device CA certificate
func (s *DeviceCACertStore) CreateTrustedDeviceCACert(ctx context.Context, request CreateTrustedDeviceCACertRequestObject) (CreateTrustedDeviceCACertResponseObject, error) {
	// Read the PEM data from the request body
	pemData, err := io.ReadAll(request.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		return CreateTrustedDeviceCACert400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Create the certificate in the database
	cert, err := s.State.CreateDeviceCACertificate(ctx, string(pemData))
	if err != nil {
		slog.Error("Failed to create device CA certificate", "error", err)
		return CreateTrustedDeviceCACert400JSONResponse{
			BadRequest: components.BadRequest{
				Message: fmt.Sprintf("Failed to create certificate: %v", err),
			},
		}, nil
	}

	return CreateTrustedDeviceCACert201JSONResponse{
		Id:        &cert.ID,
		Pem:       &cert.PEM,
		Subject:   &cert.Subject,
		Issuer:    &cert.Issuer,
		NotBefore: &cert.NotBefore,
		NotAfter:  &cert.NotAfter,
		CreatedAt: &cert.CreatedAt,
		UpdatedAt: &cert.UpdatedAt,
	}, nil
}

// GetTrustedDeviceCACertByIdAsPem retrieves a device CA certificate by ID as PEM
func (s *DeviceCACertStore) GetTrustedDeviceCACertByIdAsPem(ctx context.Context, request GetTrustedDeviceCACertByIdAsPemRequestObject) (GetTrustedDeviceCACertByIdAsPemResponseObject, error) {
	cert, err := s.State.GetDeviceCACertificate(ctx, request.UUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "device CA certificate not found" {
			slog.Debug("Device CA certificate not found", "uuid", request.UUID)
			return GetTrustedDeviceCACertByIdAsPem404JSONResponse{
				NotFound: components.NotFound{
					Message: "Device CA certificate not found",
				},
			}, nil
		}
		slog.Error("Failed to get device CA certificate", "error", err, "uuid", request.UUID)
		return GetTrustedDeviceCACertByIdAsPem500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to retrieve certificate",
			},
		}, nil
	}

	// Return the PEM data
	pemReader := bytes.NewReader([]byte(cert.PEM))
	return GetTrustedDeviceCACertByIdAsPem200ApplicationxPemFileResponse{
		Body:          pemReader,
		ContentLength: int64(len(cert.PEM)),
	}, nil
}

// UpdateTrustedDeviceCACert updates an existing device CA certificate
func (s *DeviceCACertStore) UpdateTrustedDeviceCACert(ctx context.Context, request UpdateTrustedDeviceCACertRequestObject) (UpdateTrustedDeviceCACertResponseObject, error) {
	// Read the PEM data from the request body
	pemData, err := io.ReadAll(request.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		return UpdateTrustedDeviceCACert400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Update the certificate in the database
	cert, err := s.State.UpdateDeviceCACertificate(ctx, request.UUID, string(pemData))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "device CA certificate not found" {
			slog.Debug("Device CA certificate not found", "uuid", request.UUID)
			return UpdateTrustedDeviceCACert404JSONResponse{
				NotFound: components.NotFound{
					Message: "Device CA certificate not found",
				},
			}, nil
		}
		slog.Error("Failed to update device CA certificate", "error", err, "uuid", request.UUID)
		return UpdateTrustedDeviceCACert400JSONResponse{
			BadRequest: components.BadRequest{
				Message: fmt.Sprintf("Failed to update certificate: %v", err),
			},
		}, nil
	}

	return UpdateTrustedDeviceCACert200JSONResponse{
		Id:        &cert.ID,
		Pem:       &cert.PEM,
		Subject:   &cert.Subject,
		Issuer:    &cert.Issuer,
		NotBefore: &cert.NotBefore,
		NotAfter:  &cert.NotAfter,
		CreatedAt: &cert.CreatedAt,
		UpdatedAt: &cert.UpdatedAt,
	}, nil
}

// DeleteTrustedDeviceCACert deletes a device CA certificate by ID
func (s *DeviceCACertStore) DeleteTrustedDeviceCACert(ctx context.Context, request DeleteTrustedDeviceCACertRequestObject) (DeleteTrustedDeviceCACertResponseObject, error) {
	err := s.State.DeleteDeviceCACertificate(ctx, request.UUID)
	if err != nil {
		if err.Error() == "device CA certificate not found" {
			slog.Debug("Device CA certificate not found", "uuid", request.UUID)
			return DeleteTrustedDeviceCACert404JSONResponse{
				NotFound: components.NotFound{
					Message: "Device CA certificate not found",
				},
			}, nil
		}
		slog.Error("Failed to delete device CA certificate", "error", err, "uuid", request.UUID)
		return DeleteTrustedDeviceCACert500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to delete certificate",
			},
		}, nil
	}

	return DeleteTrustedDeviceCACert204Response{}, nil
}
