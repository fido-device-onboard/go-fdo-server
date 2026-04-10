// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

// MigrateJSONToCBOR performs a one-time migration of V1 JSON RvInfo to CBOR format.
// This should be called after database initialization.
// If data is already in CBOR format, it does nothing.
func MigrateJSONToCBOR(ctx context.Context, rvInfoState *state.RvInfoState) error {
	// Read raw value from database
	var rvInfoRow struct {
		ID    int    `gorm:"primaryKey"`
		Value []byte `gorm:"not null"`
	}

	db := rvInfoState.DB.WithContext(ctx)
	err := db.Table("rvinfo").Where("id = ?", 1).First(&rvInfoRow).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Debug("No RvInfo to migrate")
			return nil
		}
		return fmt.Errorf("failed to check rvinfo for migration: %w", err)
	}

	// Try to unmarshal as CBOR - if successful, already migrated
	var testCBOR [][]protocol.RvInstruction
	if err = cbor.Unmarshal(rvInfoRow.Value, &testCBOR); err == nil {
		slog.Debug("RvInfo already in CBOR format, no migration needed")
		return nil
	}

	slog.Info("Migrating RvInfo from V1 JSON to CBOR format")

	// Parse as V1 JSON
	var v1RvInfo components.RVInfo
	if err = json.Unmarshal(rvInfoRow.Value, &v1RvInfo); err != nil {
		return fmt.Errorf("rvinfo is neither valid CBOR nor V1 JSON: %w", err)
	}

	// Convert V1 format to protocol format
	rvInstructions, err := RVInfoV1ToProtocol(v1RvInfo)
	if err != nil {
		return fmt.Errorf("failed to convert V1 JSON to protocol format: %w", err)
	}

	// Marshal to CBOR
	rvInfoCBOR, err := cbor.Marshal(rvInstructions)
	if err != nil {
		return fmt.Errorf("failed to marshal rvinfo to CBOR: %w", err)
	}

	// Update database with CBOR format
	result := db.Table("rvinfo").Where("id = ?", 1).Update("value", rvInfoCBOR)
	if result.Error != nil {
		return fmt.Errorf("failed to update rvinfo to CBOR: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		slog.Warn("RvInfo migration: no rows updated")
	} else {
		slog.Info("Successfully migrated RvInfo from V1 JSON to CBOR format")
	}

	return nil
}
