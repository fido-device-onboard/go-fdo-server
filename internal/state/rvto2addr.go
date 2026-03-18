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

// RVTO2Addr model
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

// Get retrieves the current RVTO2Addr configuration
func (s *RVTO2AddrState) Get(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	var record RVTO2Addr
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return empty array when no configuration exists
			return []protocol.RvTO2Addr{}, nil
		}
		return nil, fmt.Errorf("failed to get RVTO2Addr: %w", err)
	}

	// Unmarshal CBOR to protocol.RvTO2Addr slice
	var protocolAddrs []protocol.RvTO2Addr
	if err := cbor.Unmarshal(record.Value, &protocolAddrs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RVTO2Addr: %w", err)
	}

	return protocolAddrs, nil
}

// Update updates the RVTO2Addr configuration
func (s *RVTO2AddrState) Update(ctx context.Context, addrs []protocol.RvTO2Addr) error {
	// Validate that each entry has at least one of dns or ip
	for i, addr := range addrs {
		if (addr.DNSAddress == nil || *addr.DNSAddress == "") && addr.IPAddress == nil {
			return fmt.Errorf("%w: entry at index %d has neither dns nor ip", ErrInvalidRVTO2Addr, i)
		}
	}

	// Marshal to CBOR
	cborData, err := cbor.Marshal(addrs)
	if err != nil {
		return fmt.Errorf("failed to marshal RVTO2Addr: %w", err)
	}

	// Store in database (single row with ID=1)
	record := RVTO2Addr{
		ID:    1,
		Value: cborData,
	}

	// Use Save to insert or update
	if err := s.DB.WithContext(ctx).Save(&record).Error; err != nil {
		return fmt.Errorf("failed to save RVTO2Addr: %w", err)
	}

	return nil
}

// Delete deletes the RVTO2Addr configuration and returns the previous value
func (s *RVTO2AddrState) Delete(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	// First, get the current configuration
	currentAddrs, err := s.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current RVTO2Addr: %w", err)
	}

	// Delete the record
	result := s.DB.WithContext(ctx).Where("id = ?", 1).Delete(&RVTO2Addr{})
	if result.Error != nil {
		return nil, fmt.Errorf("failed to delete RVTO2Addr: %w", result.Error)
	}

	// Return the previous configuration (may be empty array if nothing was configured)
	return currentAddrs, nil
}
