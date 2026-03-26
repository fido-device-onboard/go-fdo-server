// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"

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

// convertRvInstructionsToV2JSON converts [][]protocol.RvInstruction to V2 OpenAPI JSON format
func convertRvInstructionsToV2JSON(rvInstructions [][]protocol.RvInstruction) ([]byte, error) {
	// V2 format: array of arrays of single-key objects
	// Example: [[{"dns":"host"},{"protocol":"http"},{"owner_port":8080}]]
	out := make([][]map[string]interface{}, 0, len(rvInstructions))

	for _, directive := range rvInstructions {
		group := make([]map[string]interface{}, 0, len(directive))

		for _, instr := range directive {
			item := make(map[string]interface{})

			switch instr.Variable {
			case protocol.RVDns:
				var dns string
				if err := cbor.Unmarshal(instr.Value, &dns); err != nil {
					return nil, fmt.Errorf("failed to unmarshal dns: %w", err)
				}
				item["dns"] = dns

			case protocol.RVIPAddress:
				var ip net.IP
				if err := cbor.Unmarshal(instr.Value, &ip); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ip: %w", err)
				}
				item["ip"] = ip.String()

			case protocol.RVProtocol:
				var code uint8
				if err := cbor.Unmarshal(instr.Value, &code); err != nil {
					return nil, fmt.Errorf("failed to unmarshal protocol: %w", err)
				}
				item["protocol"] = ProtocolStringFromCode(code)

			case protocol.RVMedium:
				var medium uint8
				if err := cbor.Unmarshal(instr.Value, &medium); err != nil {
					return nil, fmt.Errorf("failed to unmarshal medium: %w", err)
				}
				item["medium"] = MediumStringFromCode(medium)

			case protocol.RVDevPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal device_port: %w", err)
				}
				item["device_port"] = int(port) // V2 uses integer, not string

			case protocol.RVOwnerPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal owner_port: %w", err)
				}
				item["owner_port"] = int(port) // V2 uses integer, not string

			case protocol.RVWifiSsid:
				var ssid string
				if err := cbor.Unmarshal(instr.Value, &ssid); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_ssid: %w", err)
				}
				item["wifi_ssid"] = ssid

			case protocol.RVWifiPw:
				var pw string
				if err := cbor.Unmarshal(instr.Value, &pw); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_pw: %w", err)
				}
				item["wifi_pw"] = pw

			case protocol.RVDevOnly:
				item["dev_only"] = true

			case protocol.RVOwnerOnly:
				item["owner_only"] = true

			case protocol.RVBypass:
				item["rv_bypass"] = true

			case protocol.RVDelaysec:
				var secs uint32
				if err := cbor.Unmarshal(instr.Value, &secs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal delay_seconds: %w", err)
				}
				item["delay_seconds"] = int(secs)

			case protocol.RVSvCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal sv_cert_hash: %w", err)
				}
				item["sv_cert_hash"] = hex.EncodeToString(hash)

			case protocol.RVClCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal cl_cert_hash: %w", err)
				}
				item["cl_cert_hash"] = hex.EncodeToString(hash)

			case protocol.RVUserInput:
				item["user_input"] = true

			case protocol.RVExtRV:
				var extrv []string
				if err := cbor.Unmarshal(instr.Value, &extrv); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ext_rv: %w", err)
				}
				item["ext_rv"] = extrv
			}

			group = append(group, item)
		}

		out = append(out, group)
	}

	return json.Marshal(out)
}

// FetchRvInfoJSON retrieves the current rendezvous information as V2 OpenAPI JSON
// Supports automatic migration from old JSON format to CBOR
func (s *RvInfoState) FetchRvInfoJSON(ctx context.Context) ([]byte, error) {
	var rvInfoRow RvInfo
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&rvInfoRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRvInfoNotFound
		}
		return nil, err
	}

	var rvInfo [][]protocol.RvInstruction

	// Try to unmarshal as CBOR first (new format)
	if err := cbor.Unmarshal(rvInfoRow.Value, &rvInfo); err != nil {
		// CBOR failed, try to parse as V2 OpenAPI JSON (old format)
		// This handles migration if database still has JSON from before CBOR implementation
		var parseErr error
		rvInfo, parseErr = ParseOpenAPIRvJSON(rvInfoRow.Value)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: failed to parse as CBOR or V2 JSON: cbor error: %v, json error: %v",
				ErrInvalidRvInfo, err, parseErr)
		}

		// Auto-migrate: Convert to CBOR and update database
		rvInfoCBOR, err := cbor.Marshal(rvInfo)
		if err != nil {
			// Log warning but continue - we have valid data
			slog.Warn("Failed to marshal rvinfo to CBOR during migration, continuing with parsed data",
				"error", err)
		} else {
			// Update database with CBOR format
			tx := s.DB.WithContext(ctx).Model(&RvInfo{}).Where("id = ?", 1).Update("value", rvInfoCBOR)
			if tx.Error != nil {
				// Log warning but continue - migration can be retried on next read
				slog.Warn("Failed to update database during CBOR migration, will retry on next read",
					"error", tx.Error)
			} else if tx.RowsAffected == 0 {
				slog.Warn("No rows updated during CBOR migration (row may have been deleted)")
			} else {
				slog.Info("Successfully migrated RvInfo from V2 JSON to CBOR format")
			}
		}
	}

	// Convert [][]protocol.RvInstruction to V2 OpenAPI JSON format
	return convertRvInstructionsToV2JSON(rvInfo)
}

// InsertRvInfo creates new rendezvous information configuration
func (s *RvInfoState) InsertRvInfo(ctx context.Context, data []byte) error {
	// Parse V2 OpenAPI JSON into [][]protocol.RvInstruction
	rvInstructions, err := ParseOpenAPIRvJSON(data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	// Marshal to CBOR for storage
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

// UpdateRvInfo updates existing rendezvous information configuration
func (s *RvInfoState) UpdateRvInfo(ctx context.Context, data []byte) error {
	// Parse V2 OpenAPI JSON into [][]protocol.RvInstruction
	rvInstructions, err := ParseOpenAPIRvJSON(data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	// Marshal to CBOR for storage
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
