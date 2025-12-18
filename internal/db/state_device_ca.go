// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

// DeviceCACertificate stores trusted device CA certificates
type DeviceCACertificate struct {
	ID        openapi_types.UUID `gorm:"type:uuid;primaryKey"`
	PEM       string             `gorm:"type:text;not null"`
	Subject   string             `gorm:"type:text;not null"`
	Issuer    string             `gorm:"type:text;not null"`
	NotBefore time.Time          `gorm:"not null;index:idx_device_ca_not_before"`
	NotAfter  time.Time          `gorm:"not null;index:idx_device_ca_not_after"`
	CreatedAt time.Time          `gorm:"autoCreateTime:milli"`
	UpdatedAt time.Time          `gorm:"autoUpdateTime:milli"`
}

// TableName specifies the table name for DeviceCACertificate model
func (DeviceCACertificate) TableName() string {
	return "device_ca_certificates"
}

// ListDeviceCACertificates retrieves a paginated, filtered, and sorted list of device CA certificates
func (s *State) ListDeviceCACertificates(ctx context.Context, limit, offset int, issuer *string, sortBy, sortOrder string) ([]DeviceCACertificate, int64, error) {
	var certs []DeviceCACertificate
	var total int64

	query := s.DB.Model(&DeviceCACertificate{})

	// Apply filters
	if issuer != nil && *issuer != "" {
		query = query.Where("issuer = ?", *issuer)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count device CA certificates: %w", err)
	}

	// Apply sorting
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}
	orderClause := fmt.Sprintf("%s %s", sortBy, sortOrder)
	query = query.Order(orderClause)

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&certs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list device CA certificates: %w", err)
	}

	return certs, total, nil
}

// CreateDeviceCACertificate creates a new device CA certificate
func (s *State) CreateDeviceCACertificate(ctx context.Context, pemData string) (*DeviceCACertificate, error) {
	// Parse the PEM certificate
	block, _ := pem.Decode([]byte(pemData))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode PEM block containing certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Generate UUID
	newUUID := uuid.New()

	// Create the database record
	dbCert := &DeviceCACertificate{
		ID:        newUUID,
		PEM:       pemData,
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
	}

	if err := s.DB.Create(dbCert).Error; err != nil {
		return nil, fmt.Errorf("failed to create device CA certificate: %w", err)
	}

	return dbCert, nil
}

// GetDeviceCACertificate retrieves a device CA certificate by ID
func (s *State) GetDeviceCACertificate(ctx context.Context, id openapi_types.UUID) (*DeviceCACertificate, error) {
	var cert DeviceCACertificate
	if err := s.DB.Where("id = ?", id).First(&cert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("device CA certificate not found")
		}
		return nil, fmt.Errorf("failed to get device CA certificate: %w", err)
	}
	return &cert, nil
}

// UpdateDeviceCACertificate updates an existing device CA certificate
func (s *State) UpdateDeviceCACertificate(ctx context.Context, id openapi_types.UUID, pemData string) (*DeviceCACertificate, error) {
	// Parse the PEM certificate
	block, _ := pem.Decode([]byte(pemData))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode PEM block containing certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Find the existing certificate
	var dbCert DeviceCACertificate
	if err := s.DB.Where("id = ?", id).First(&dbCert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("device CA certificate not found")
		}
		return nil, fmt.Errorf("failed to find device CA certificate: %w", err)
	}

	// Update the certificate
	dbCert.PEM = pemData
	dbCert.Subject = cert.Subject.String()
	dbCert.Issuer = cert.Issuer.String()
	dbCert.NotBefore = cert.NotBefore
	dbCert.NotAfter = cert.NotAfter

	if err := s.DB.Save(&dbCert).Error; err != nil {
		return nil, fmt.Errorf("failed to update device CA certificate: %w", err)
	}

	return &dbCert, nil
}

// DeleteDeviceCACertificate deletes a device CA certificate by ID
func (s *State) DeleteDeviceCACertificate(ctx context.Context, id openapi_types.UUID) error {
	result := s.DB.Where("id = ?", id).Delete(&DeviceCACertificate{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete device CA certificate: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("device CA certificate not found")
	}
	return nil
}
