// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Sentinel errors for RV info operations
var (
	ErrInvalidRvInfo  = errors.New("invalid rvinfo data")
	ErrRvInfoNotFound = errors.New("rendezvous info not found")
)

// RvInfoState manages rendezvous information configuration state
type RvInfoState struct {
	DB *gorm.DB
}

// RvInfo model stores rendezvous information as CBOR-encoded [][]protocol.RvInstruction
type RvInfo struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"not null"` // CBOR-encoded [][]protocol.RvInstruction
}

// TableName specifies the table name for RvInfo model
// Uses same table as V1 API for unified storage
func (RvInfo) TableName() string {
	return "rvinfo"
}

// InitRvInfoDB initializes the RvInfo state with database migrations
func InitRvInfoDB(database *gorm.DB) (*RvInfoState, error) {
	state := &RvInfoState{
		DB: database,
	}

	// Auto-migrate schema
	if err := state.DB.AutoMigrate(&RvInfo{}); err != nil {
		slog.Error("Failed to migrate RvInfo schema", "error", err)
		return nil, err
	}

	slog.Debug("RvInfo state initialized successfully")
	return state, nil
}

// GetRVInfo retrieves the current rendezvous information as [][]protocol.RvInstruction
// State layer returns protocol structs - JSON conversion is API layer's responsibility
func (s *RvInfoState) GetRVInfo(ctx context.Context) ([][]protocol.RvInstruction, error) {
	var rvInfoRow RvInfo
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&rvInfoRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRvInfoNotFound
		}
		return nil, err
	}

	var rvInfo [][]protocol.RvInstruction
	if err := cbor.Unmarshal(rvInfoRow.Value, &rvInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CBOR: %w", err)
	}

	return rvInfo, nil
}

// CreateRVInfo creates new rendezvous information configuration
// Accepts pre-parsed RvInstructions - JSON parsing is the API layer's responsibility
func (s *RvInfoState) CreateRVInfo(ctx context.Context, rvInstructions [][]protocol.RvInstruction) error {
	cborData, err := cbor.Marshal(rvInstructions)
	if err != nil {
		return fmt.Errorf("failed to marshal rvinfo to CBOR: %w", err)
	}

	rvInfo := RvInfo{
		ID:    1,
		Value: cborData,
	}
	tx := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rvInfo)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

// UpdateRVInfo updates existing rendezvous information configuration
// Accepts pre-parsed RvInstructions - JSON parsing is the API layer's responsibility
func (s *RvInfoState) UpdateRVInfo(ctx context.Context, rvInstructions [][]protocol.RvInstruction) error {
	cborData, err := cbor.Marshal(rvInstructions)
	if err != nil {
		return fmt.Errorf("failed to marshal rvinfo to CBOR: %w", err)
	}

	tx := s.DB.WithContext(ctx).Model(&RvInfo{}).Where("id = ?", 1).Update("value", cborData)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrRvInfoNotFound
	}
	return nil
}

// CreateOrUpdateRVInfo atomically inserts or updates rendezvous information configuration
// Accepts pre-parsed RvInstructions - JSON parsing is the API layer's responsibility
// This method is race-condition safe for concurrent requests
func (s *RvInfoState) CreateOrUpdateRVInfo(ctx context.Context, rvInstructions [][]protocol.RvInstruction) error {
	cborData, err := cbor.Marshal(rvInstructions)
	if err != nil {
		return fmt.Errorf("failed to marshal rvinfo to CBOR: %w", err)
	}

	rvInfo := RvInfo{
		ID:    1,
		Value: cborData,
	}
	return s.DB.WithContext(ctx).Save(&rvInfo).Error
}

// DeleteRvInfo removes the rendezvous information configuration
func (s *RvInfoState) DeleteRvInfo(ctx context.Context) error {
	tx := s.DB.WithContext(ctx).Where("id = ?", 1).Delete(&RvInfo{})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrRvInfoNotFound
	}
	return nil
}
