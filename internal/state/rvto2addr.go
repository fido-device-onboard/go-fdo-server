// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// RVTO2Addr model.
// Singleton table: the CHECK constraint and application-level ID=1 enforcement ensure only one row exists.
// The CHECK constraint syntax is standard SQL and portable across SQLite and PostgreSQL.
type RVTO2Addr struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"not null"` // GORM will use bytea for PostgreSQL, blob for SQLite
}

// TableName specifies the table name for RVTO2Addr model
func (RVTO2Addr) TableName() string {
	return "rvto2addr"
}

// Sentinel errors for RVTO2Addr operations
var (
	ErrRVTO2AddrNotFound = errors.New("RVTO2Addr configuration not found")
	ErrRVTO2AddrExists   = errors.New("RVTO2Addr configuration already exists")
	ErrInvalidRVTO2Addr  = errors.New("invalid RVTO2Addr configuration: at least one of dns or ip must be specified")
)

type RVTO2AddrState struct {
	DB *gorm.DB
}

func InitRVTO2AddrDB(db *gorm.DB) (*RVTO2AddrState, error) {
	state := &RVTO2AddrState{
		DB: db,
	}
	// Auto-migrate schema
	err := state.DB.AutoMigrate(&RVTO2Addr{})
	if err != nil {
		slog.Error("Failed to migrate RVTO2Addr schema", "error", err)
		return nil, err
	}
	slog.Debug("RVTO2Addr database initialized successfully")
	return state, nil
}

// ReadLegacyOwnerInfoJSON returns the raw JSON bytes stored in the legacy
// owner_info table, or nil when no migration is needed (the table does not
// exist, is empty, or rvto2addr already contains data). JSON parsing is
// intentionally left to the API layer so that the state layer stays free of
// presentation-format concerns.
func (s *RVTO2AddrState) ReadLegacyOwnerInfoJSON(ctx context.Context) ([]byte, error) {
	if !s.DB.Migrator().HasTable("owner_info") {
		return nil, nil
	}

	var count int64
	if err := s.DB.WithContext(ctx).Table("rvto2addr").Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to check rvto2addr table: %w", err)
	}
	if count > 0 {
		slog.Debug("rvto2addr already has data, skipping owner_info migration")
		s.DropLegacyOwnerInfo()
		return nil, nil
	}

	var row struct {
		ID    int    `gorm:"primaryKey"`
		Value []byte `gorm:"not null"`
	}
	err := s.DB.WithContext(ctx).Table("owner_info").Where("id = ?", 1).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Debug("No owner_info data to migrate")
		s.DropLegacyOwnerInfo()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read legacy owner_info table: %w", err)
	}

	return row.Value, nil
}

// DropLegacyOwnerInfo drops the legacy owner_info table. It is a best-effort
// operation: failures are logged as a warning and not returned to the caller.
func (s *RVTO2AddrState) DropLegacyOwnerInfo() {
	if err := s.DB.Migrator().DropTable("owner_info"); err != nil {
		slog.Warn("Failed to drop legacy owner_info table", "error", err)
	}
}

// Get retrieves the current RVTO2Addr configuration
func (s *RVTO2AddrState) Get(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	var record RVTO2Addr
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []protocol.RvTO2Addr{}, nil
		}
		return nil, fmt.Errorf("failed to get RVTO2Addr: %w", err)
	}

	var protocolAddrs []protocol.RvTO2Addr
	if err := cbor.Unmarshal(record.Value, &protocolAddrs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RVTO2Addr: %w", err)
	}

	return protocolAddrs, nil
}

// validateRVTO2Addrs validates and marshals addresses to CBOR.
func validateAndMarshalRVTO2Addrs(addrs []protocol.RvTO2Addr) ([]byte, error) {
	for i, addr := range addrs {
		if (addr.DNSAddress == nil || *addr.DNSAddress == "") && addr.IPAddress == nil {
			return nil, fmt.Errorf("%w: entry at index %d has neither dns nor ip", ErrInvalidRVTO2Addr, i)
		}
	}
	cborData, err := cbor.Marshal(addrs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RVTO2Addr: %w", err)
	}
	return cborData, nil
}

// Create inserts a new RVTO2Addr configuration.
// Returns ErrRVTO2AddrExists if a configuration already exists.
func (s *RVTO2AddrState) Create(ctx context.Context, addrs []protocol.RvTO2Addr) error {
	cborData, err := validateAndMarshalRVTO2Addrs(addrs)
	if err != nil {
		return err
	}

	record := RVTO2Addr{ID: 1, Value: cborData}
	if err := s.DB.WithContext(ctx).Create(&record).Error; err != nil {
		if isDuplicateError(err) {
			return ErrRVTO2AddrExists
		}
		return fmt.Errorf("failed to create RVTO2Addr: %w", err)
	}
	return nil
}

// Update updates an existing RVTO2Addr configuration.
// Returns ErrRVTO2AddrNotFound if no configuration exists.
func (s *RVTO2AddrState) Update(ctx context.Context, addrs []protocol.RvTO2Addr) error {
	cborData, err := validateAndMarshalRVTO2Addrs(addrs)
	if err != nil {
		return err
	}

	tx := s.DB.WithContext(ctx).Model(&RVTO2Addr{}).Where("id = ?", 1).Update("value", cborData)
	if tx.Error != nil {
		return fmt.Errorf("failed to update RVTO2Addr: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return ErrRVTO2AddrNotFound
	}
	return nil
}

// Upsert creates or updates the RVTO2Addr configuration atomically.
// Used by migration paths that don't care about create-vs-update semantics.
func (s *RVTO2AddrState) Upsert(ctx context.Context, addrs []protocol.RvTO2Addr) error {
	cborData, err := validateAndMarshalRVTO2Addrs(addrs)
	if err != nil {
		return err
	}

	record := RVTO2Addr{ID: 1, Value: cborData}
	if err := s.DB.WithContext(ctx).Save(&record).Error; err != nil {
		return fmt.Errorf("failed to save RVTO2Addr: %w", err)
	}
	return nil
}

// Delete deletes the RVTO2Addr configuration and returns the previous value.
// The read and delete are wrapped in a transaction to ensure atomicity.
func (s *RVTO2AddrState) Delete(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	var currentAddrs []protocol.RvTO2Addr

	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Read the current value inside the transaction
		var record RVTO2Addr
		if err := tx.Where("id = ?", 1).First(&record).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				currentAddrs = []protocol.RvTO2Addr{}
				return nil
			}
			return fmt.Errorf("failed to get current RVTO2Addr: %w", err)
		}

		if err := cbor.Unmarshal(record.Value, &currentAddrs); err != nil {
			return fmt.Errorf("failed to unmarshal RVTO2Addr: %w", err)
		}

		// Delete inside the same transaction
		if err := tx.Where("id = ?", 1).Delete(&RVTO2Addr{}).Error; err != nil {
			return fmt.Errorf("failed to delete RVTO2Addr: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return currentAddrs, nil
}
