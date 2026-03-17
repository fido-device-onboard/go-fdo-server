// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"errors"
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/fido-device-onboard/go-fdo-server/internal/manufacturing"
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

// RvInfo model stores rendezvous information as JSON
type RvInfo struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"type:text;not null"`
}

// TableName specifies the table name for RvInfo model
func (RvInfo) TableName() string {
	return "rv_info"
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

// FetchRvInfoJSON retrieves the current rendezvous information as JSON
func (s *RvInfoState) FetchRvInfoJSON(ctx context.Context) ([]byte, error) {
	var rvInfo RvInfo
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&rvInfo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRvInfoNotFound
		}
		return nil, err
	}
	return rvInfo.Value, nil
}

// InsertRvInfo creates new rendezvous information configuration
func (s *RvInfoState) InsertRvInfo(ctx context.Context, data []byte) error {
	// Validate data can be parsed into [][]protocol.RvInstruction
	if _, err := manufacturing.ParseOpenAPIRvJSON(data); err != nil {
		return errors.Join(ErrInvalidRvInfo, err)
	}

	rvInfo := RvInfo{
		ID:    1,
		Value: data,
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

// UpdateRvInfo updates existing rendezvous information configuration
func (s *RvInfoState) UpdateRvInfo(ctx context.Context, data []byte) error {
	// Validate data can be parsed into [][]protocol.RvInstruction
	if _, err := manufacturing.ParseOpenAPIRvJSON(data); err != nil {
		return errors.Join(ErrInvalidRvInfo, err)
	}

	tx := s.DB.WithContext(ctx).Model(&RvInfo{}).Where("id = ?", 1).Update("value", data)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return ErrRvInfoNotFound
	}
	return nil
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
