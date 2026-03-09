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
	"math"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// RvInfo model
type RvInfo struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"type:text;not null"`
}

// TableName specifies the table name for RvInfo model
func (RvInfo) TableName() string {
	return "rvinfo"
}

// Sentinel errors for RvInfo operations
var (
	ErrRvInfoNotFound = errors.New("RvInfo configuration not found")
	ErrInvalidRvInfo  = errors.New("invalid RvInfo configuration")
)

type RvInfoState struct {
	DB *gorm.DB
}

func InitRvInfoDB(db *gorm.DB) (*RvInfoState, error) {
	state := &RvInfoState{
		DB: db,
	}
	// Auto-migrate schema
	err := state.DB.AutoMigrate(&RvInfo{})
	if err != nil {
		slog.Error("Failed to migrate RvInfo schema", "error", err)
		return nil, err
	}
	slog.Debug("RvInfo database initialized successfully")
	return state, nil
}

// validateRvInfoJSON validates that the data is valid JSON
// The actual parsing is handled by the db layer's parseHumanReadableRvJSON function
func validateRvInfoJSON(data []byte) error {
	// Basic validation - ensure it's valid JSON
	var test interface{}
	if err := json.Unmarshal(data, &test); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	// Additional validation: should be an array
	if _, ok := test.([]interface{}); !ok {
		return fmt.Errorf("RvInfo must be a JSON array")
	}
	return nil
}

// Get retrieves the current RvInfo configuration as JSON
// Returns ErrRvInfoNotFound if no configuration exists
func (s *RvInfoState) Get(ctx context.Context) ([]byte, error) {
	var record RvInfo
	if err := s.DB.WithContext(ctx).Where("id = ?", 1).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRvInfoNotFound
		}
		return nil, fmt.Errorf("failed to get RvInfo: %w", err)
	}

	return record.Value, nil
}

// GetParsed retrieves and parses the RvInfo configuration into protocol instructions
// Returns ErrRvInfoNotFound if no configuration exists
func (s *RvInfoState) GetParsed(ctx context.Context) ([][]protocol.RvInstruction, error) {
	// Get the JSON representation
	jsonData, err := s.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Parse the JSON into protocol instructions
	return parseHumanReadableRvJSON(jsonData)
}

// Update updates or creates the RvInfo configuration (upsert behavior per OpenAPI spec)
func (s *RvInfoState) Update(ctx context.Context, data []byte) error {
	// Validate the data
	if err := validateRvInfoJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	// Use Save for upsert behavior (create or update)
	record := RvInfo{
		ID:    1,
		Value: data,
	}

	if err := s.DB.WithContext(ctx).Save(&record).Error; err != nil {
		return fmt.Errorf("failed to save RvInfo: %w", err)
	}

	return nil
}

// Create creates the RvInfo configuration (fails if already exists)
func (s *RvInfoState) Create(ctx context.Context, data []byte) error {
	// Validate the data
	if err := validateRvInfoJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	record := RvInfo{
		ID:    1,
		Value: data,
	}

	// Use Create with conflict check
	tx := s.DB.WithContext(ctx).Create(&record)
	if tx.Error != nil {
		// Check for UNIQUE constraint error (SQLite and PostgreSQL)
		errMsg := tx.Error.Error()
		if contains(errMsg, "UNIQUE constraint") || contains(errMsg, "duplicate key") {
			return gorm.ErrDuplicatedKey
		}
		return fmt.Errorf("failed to create RvInfo: %w", tx.Error)
	}

	return nil
}

// contains checks if a string contains a substring (case-sensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Delete deletes the RvInfo configuration and returns the previous value
// Returns ErrRvInfoNotFound if no configuration existed (let handler decide how to respond)
func (s *RvInfoState) Delete(ctx context.Context) ([]byte, error) {
	// First, get the current configuration
	currentData, err := s.Get(ctx)
	if err != nil {
		// Return the error (including ErrRvInfoNotFound) so handler can decide how to respond
		return nil, err
	}

	// Delete the record
	result := s.DB.WithContext(ctx).Where("id = ?", 1).Delete(&RvInfo{})
	if result.Error != nil {
		return nil, fmt.Errorf("failed to delete RvInfo: %w", result.Error)
	}

	return currentData, nil
}

// Parsing functions for converting human-readable JSON to protocol instructions

// encodeRvValue converts a value to CBOR-encoded bytes based on the RV variable type
func encodeRvValue(rvVar protocol.RvVar, val any) ([]byte, error) {
	switch v := val.(type) {
	case string:
		switch rvVar {
		case protocol.RVDns:
			return cbor.Marshal(v)
		case protocol.RVIPAddress:
			ip := net.ParseIP(v)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip %q", v)
			}
			return cbor.Marshal(ip)
		default:
			return cbor.Marshal(v)
		}
	case bool:
		return cbor.Marshal(v)
	case float64:
		// JSON numbers -> coerce by variable semantics
		switch rvVar {
		case protocol.RVDevPort, protocol.RVOwnerPort:
			return cbor.Marshal(uint16(v))
		case protocol.RVProtocol, protocol.RVMedium:
			return cbor.Marshal(uint8(v))
		case protocol.RVDelaysec:
			return cbor.Marshal(uint32(v))
		default:
			return cbor.Marshal(int64(v))
		}
	default:
		return cbor.Marshal(v)
	}
}

// protocolCodeFromString maps protocol string names to their numeric codes
func protocolCodeFromString(s string) (uint8, error) {
	switch s {
	case "rest":
		return uint8(protocol.RVProtRest), nil
	case "http":
		return uint8(protocol.RVProtHTTP), nil
	case "https":
		return uint8(protocol.RVProtHTTPS), nil
	case "tcp":
		return uint8(protocol.RVProtTCP), nil
	case "tls":
		return uint8(protocol.RVProtTLS), nil
	case "coap+tcp":
		return uint8(protocol.RVProtCoapTCP), nil
	case "coap":
		return uint8(protocol.RVProtCoapUDP), nil
	default:
		return 0, fmt.Errorf("unsupported protocol %q", s)
	}
}

// parseMediumValue parses medium values from JSON
func parseMediumValue(v any) (uint8, error) {
	switch t := v.(type) {
	case float64:
		return uint8(t), nil
	case string:
		switch t {
		case "eth_all":
			return protocol.RVMedEthAll, nil
		case "wifi_all":
			return protocol.RVMedWifiAll, nil
		default:
			return 0, fmt.Errorf("unsupported medium %q", t)
		}
	default:
		return 0, fmt.Errorf("unsupported medium type %T", v)
	}
}

// parsePortValue parses port values from JSON (string or number)
func parsePortValue(v any) (uint16, error) {
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			return 0, fmt.Errorf("port must be an integer, got %v", t)
		}
		if t < 1 || t > 65535 {
			return 0, fmt.Errorf("port out of range: %v", t)
		}
		return uint16(t), nil
	case string:
		if t == "" {
			return 0, fmt.Errorf("empty port")
		}
		i, err := strconv.Atoi(t)
		if err != nil {
			return 0, err
		}
		if i < 1 || i > 65535 {
			return 0, fmt.Errorf("port out of range: %d", i)
		}
		return uint16(i), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

// parseHumanReadableRvJSON parses a JSON like
// [{"dns":"fdo.example.com","device_port":"8082","owner_port":"8082","protocol":"http","ip":"127.0.0.1"}]
// into [][]protocol.RvInstruction. It maps human-readable keys to RV variables
// and converts protocol strings to the appropriate numeric code.
func parseHumanReadableRvJSON(rawJSON []byte) ([][]protocol.RvInstruction, error) {
	type rvHuman struct {
		DNS          string  `json:"dns"`
		IP           string  `json:"ip"`
		Protocol     string  `json:"protocol"`
		Medium       string  `json:"medium"`
		DevicePort   string  `json:"device_port"`
		OwnerPort    string  `json:"owner_port"`
		WifiSSID     string  `json:"wifi_ssid"`
		WifiPW       string  `json:"wifi_pw"`
		DevOnly      bool    `json:"dev_only"`
		OwnerOnly    bool    `json:"owner_only"`
		RvBypass     bool    `json:"rv_bypass"`
		DelaySeconds *uint32 `json:"delay_seconds"`
		SvCertHash   string  `json:"sv_cert_hash"`
		ClCertHash   string  `json:"cl_cert_hash"`
		UserInput    string  `json:"user_input"`
		ExtRV        string  `json:"ext_rv"`
	}
	var items []rvHuman
	if err := json.Unmarshal(rawJSON, &items); err != nil {
		return nil, fmt.Errorf("invalid rvinfo JSON: %w", err)
	}

	out := make([][]protocol.RvInstruction, 0, len(items))
	for i, item := range items {
		group := make([]protocol.RvInstruction, 0)

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if item.DNS == "" && item.IP == "" {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", i)
		}

		if item.DNS != "" {
			enc, err := encodeRvValue(protocol.RVDns, item.DNS)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDns, Value: enc})
		}
		if item.IP != "" {
			enc, err := encodeRvValue(protocol.RVIPAddress, item.IP)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc})
		}
		if item.Protocol != "" {
			code, err := protocolCodeFromString(item.Protocol)
			if err != nil {
				return nil, err
			}
			enc, err := encodeRvValue(protocol.RVProtocol, uint8(code))
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc})
		}
		if item.Medium != "" {
			m, err := parseMediumValue(item.Medium)
			if err != nil {
				return nil, fmt.Errorf("medium: %w", err)
			}
			enc, err := encodeRvValue(protocol.RVMedium, uint8(m))
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc})
		}
		if item.DevicePort != "" {
			num, err := parsePortValue(item.DevicePort)
			if err != nil {
				return nil, fmt.Errorf("device_port: %w", err)
			}
			enc, err := encodeRvValue(protocol.RVDevPort, num)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc})
		}
		if item.OwnerPort != "" {
			num, err := parsePortValue(item.OwnerPort)
			if err != nil {
				return nil, fmt.Errorf("owner_port: %w", err)
			}
			enc, err := encodeRvValue(protocol.RVOwnerPort, num)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc})
		}
		if item.WifiSSID != "" {
			enc, err := encodeRvValue(protocol.RVWifiSsid, item.WifiSSID)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc})
		}
		if item.WifiPW != "" {
			enc, err := encodeRvValue(protocol.RVWifiPw, item.WifiPW)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc})
		}
		if item.DevOnly {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDevOnly})
		}
		if item.OwnerOnly {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerOnly})
		}
		if item.RvBypass {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVBypass})
		}
		if item.DelaySeconds != nil {
			secs := uint64(*item.DelaySeconds)
			enc, err := encodeRvValue(protocol.RVDelaysec, secs)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc})
		}
		if item.SvCertHash != "" {
			b, err := hex.DecodeString(item.SvCertHash)
			if err != nil {
				return nil, fmt.Errorf("sv_cert_hash: %w", err)
			}
			enc, err := cbor.Marshal(b)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc})
		}
		if item.ClCertHash != "" {
			b, err := hex.DecodeString(item.ClCertHash)
			if err != nil {
				return nil, fmt.Errorf("cl_cert_hash: %w", err)
			}
			enc, err := cbor.Marshal(b)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc})
		}
		if item.UserInput != "" {
			enc, err := encodeRvValue(protocol.RVUserInput, item.UserInput)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVUserInput, Value: enc})
		}
		if item.ExtRV != "" {
			enc, err := encodeRvValue(protocol.RVExtRV, item.ExtRV)
			if err != nil {
				return nil, err
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc})
		}

		out = append(out, group)
	}
	return out, nil
}
