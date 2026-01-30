// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// Sentinel errors for device CA certificate operations
var (
	ErrDeviceCACertNotFound = errors.New("device CA certificate not found")
)

type TrustedDeviceCACertsState struct {
	DB                      *gorm.DB
	Mutex                   sync.RWMutex
	TrustedDeviceCACertPool *x509.CertPool
}

// DeviceCACertificate stores trusted device CA certificates

type DeviceCACertificate struct {
	Fingerprint string    `gorm:"type:varchar(64);primaryKey"`
	PEM         string    `gorm:"type:text;not null"`
	Subject     string    `gorm:"type:text;not null;index:idx_device_ca_subject"`
	Issuer      string    `gorm:"type:text;not null;index:idx_device_ca_issuer"`
	NotBefore   time.Time `gorm:"not null;index:idx_device_ca_not_before"`
	NotAfter    time.Time `gorm:"not null;index:idx_device_ca_not_after"`
	CreatedAt   time.Time `gorm:"autoCreateTime:milli;index:idx_device_ca_created_at"`
}

func InitTrustedDeviceCACertsDB(db *gorm.DB) (*TrustedDeviceCACertsState, error) {
	state := &TrustedDeviceCACertsState{
		DB: db,
	}
	// Auto-migrate all schemas
	err := state.DB.AutoMigrate(
		&DeviceCACertificate{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Debug("Trusted Device CA Certificates database initialized successfully")
	return state, nil
}

// TableName specifies the table name for DeviceCACertificate model
func (DeviceCACertificate) TableName() string {
	return "device_ca_certificates"
}

// ValidityStatus represents the validity status of a certificate
type ValidityStatus string

const (
	ValidityStatusValid       ValidityStatus = "valid"
	ValidityStatusExpired     ValidityStatus = "expired"
	ValidityStatusNotYetValid ValidityStatus = "not-yet-valid"
)

// ListDeviceCACertificates retrieves a paginated, filtered, and sorted list of device CA certificates
func (s *TrustedDeviceCACertsState) ListDeviceCACertificates(ctx context.Context, limit, offset int, issuer, subject, search *string, validityStatus *ValidityStatus, sortBy, sortOrder string) ([]DeviceCACertificate, int64, error) {
	var certs []DeviceCACertificate
	var total int64

	query := s.DB.WithContext(ctx).Model(&DeviceCACertificate{})

	// Apply filters
	if issuer != nil && *issuer != "" {
		query = query.Where("issuer = ?", *issuer)
	}
	if subject != nil && *subject != "" {
		query = query.Where("subject = ?", *subject)
	}
	if search != nil && *search != "" {
		searchPattern := "%" + *search + "%"
		query = query.Where("subject LIKE ? OR issuer LIKE ?", searchPattern, searchPattern)
	}
	if validityStatus != nil {
		now := time.Now()
		switch *validityStatus {
		case ValidityStatusValid:
			query = query.Where("not_before <= ? AND not_after > ?", now, now)
		case ValidityStatusExpired:
			query = query.Where("not_after <= ?", now)
		case ValidityStatusNotYetValid:
			query = query.Where("not_before > ?", now)
		}
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

// CertificateImportStats contains statistics about a certificate import operation
type CertificateImportStats struct {
	Detected     int
	Imported     int
	Skipped      int
	Malformed    int
	Messages     []string
	Certificates []DeviceCACertificate
}

// ImportDeviceCACertificates imports device CA certificates from PEM data in an idempotent manner
// - Valid certificates that don't exist and aren't expired are imported
// - Certificates that already exist are silently skipped
// - Expired certificates are silently skipped
// - Malformed certificates are silently skipped and counted
func (s *TrustedDeviceCACertsState) ImportDeviceCACertificates(ctx context.Context, pemData string) (*CertificateImportStats, error) {
	stats := &CertificateImportStats{
		Certificates: []DeviceCACertificate{},
		Messages:     []string{},
	}

	remaining := []byte(pemData)
	position := 0

	for {
		// Parse the next PEM block
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			remaining = rest
			continue
		}

		position++

		// Try to parse the certificate
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			// Malformed certificate - skip and count
			stats.Malformed++
			stats.Messages = append(stats.Messages, fmt.Sprintf("the certificate at position %d is malformed", position))
			remaining = rest
			continue
		}

		// Valid certificate detected
		stats.Detected++

		// Check if certificate is expired
		if time.Now().After(cert.NotAfter) {
			stats.Skipped++
			stats.Messages = append(stats.Messages, fmt.Sprintf("the certificate at position %d with subject '%s' was skipped because it is expired", position, cert.Subject.String()))
			remaining = rest
			continue
		}

		// Calculate SHA-256 fingerprint
		hash := sha256.Sum256(cert.Raw)
		fingerprint := hex.EncodeToString(hash[:])

		// Check if certificate already exists
		var existingCount int64
		if err := s.DB.WithContext(ctx).Model(&DeviceCACertificate{}).Where("fingerprint = ?", fingerprint).Count(&existingCount).Error; err != nil {
			return nil, fmt.Errorf("failed to check for existing certificate: %w", err)
		}

		if existingCount > 0 {
			// Certificate already exists - skip
			stats.Skipped++
			stats.Messages = append(stats.Messages, fmt.Sprintf("the certificate at position %d with subject '%s' was skipped because it already exists", position, cert.Subject.String()))
			remaining = rest
			continue
		}

		// Reconstruct PEM for this single certificate
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})

		// Create the database record
		dbCert := DeviceCACertificate{
			Fingerprint: fingerprint,
			PEM:         string(certPEM),
			Subject:     cert.Subject.String(),
			Issuer:      cert.Issuer.String(),
			NotBefore:   cert.NotBefore,
			NotAfter:    cert.NotAfter,
		}

		if err := s.DB.WithContext(ctx).Create(&dbCert).Error; err != nil {
			// If creation fails due to race condition (duplicate), treat as skipped
			// Otherwise, return the error
			if isDuplicateError(err) {
				stats.Skipped++
				stats.Messages = append(stats.Messages, fmt.Sprintf("the certificate at position %d with subject '%s' was skipped because it already exists", position, cert.Subject.String()))
				remaining = rest
				continue
			}
			return nil, fmt.Errorf("failed to create device CA certificate: %w", err)
		}

		// Successfully imported
		stats.Imported++
		stats.Messages = append(stats.Messages, fmt.Sprintf("the certificate at position %d with subject '%s' was imported successfully", position, cert.Subject.String()))
		stats.Certificates = append(stats.Certificates, dbCert)
		remaining = rest
	}

	return stats, nil
}

// isDuplicateError checks if the error is a duplicate key/unique constraint violation
func isDuplicateError(err error) bool {
	// PostgreSQL
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}

	// SQLite
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code == sqlite3.ErrConstraint
	}

	return false
}

// GetDeviceCACertificate retrieves a device CA certificate by fingerprint
func (s *TrustedDeviceCACertsState) GetDeviceCACertificate(ctx context.Context, fingerprint string) (*DeviceCACertificate, error) {
	var cert DeviceCACertificate
	if err := s.DB.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&cert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrDeviceCACertNotFound
		}
		return nil, fmt.Errorf("failed to get device CA certificate: %w", err)
	}
	return &cert, nil
}

// DeleteDeviceCACertificate deletes a device CA certificate by fingerprint
func (s *TrustedDeviceCACertsState) DeleteDeviceCACertificate(ctx context.Context, fingerprint string) error {
	result := s.DB.WithContext(ctx).Where("fingerprint = ?", fingerprint).Delete(&DeviceCACertificate{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete device CA certificate: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrDeviceCACertNotFound
	}
	return nil
}

// LoadTrustedDeviceCAs loads all trusted device CA certificates from the database
// into the TrustedDeviceCACertPool. This should be called on server startup
// and whenever device CA certificates are added or removed.
func (s *TrustedDeviceCACertsState) LoadTrustedDeviceCAs(ctx context.Context) error {
	// Create a new cert pool
	certPool := x509.NewCertPool()

	// Get all device CA certificates from the database
	var certs []DeviceCACertificate
	if err := s.DB.WithContext(ctx).Find(&certs).Error; err != nil {
		return fmt.Errorf("failed to load device CA certificates: %w", err)
	}

	// Parse and add each certificate to the pool
	for _, dbCert := range certs {
		block, _ := pem.Decode([]byte(dbCert.PEM))
		if block == nil {
			return fmt.Errorf("failed to decode PEM for certificate with fingerprint %s", dbCert.Fingerprint)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse certificate with fingerprint %s: %w", dbCert.Fingerprint, err)
		}

		certPool.AddCert(cert)
	}

	// Update the state with the new cert pool
	s.Mutex.Lock()
	s.TrustedDeviceCACertPool = certPool
	s.Mutex.Unlock()

	return nil
}
