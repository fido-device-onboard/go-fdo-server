// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var db *gorm.DB

// Sentinel errors to classify client input issues
var (
	ErrInvalidRVTO2Addr = errors.New("invalid rvto2addr data")
	ErrInvalidRvInfo    = errors.New("invalid rvinfo data")
)

// FetchVoucher returns a single voucher filtered by provided fields.
// Supported filters (keys):
// - "guid" (expects []byte)
// - "device_info" (expects string)
// If more than one voucher matches, an error is returned.
func FetchVoucher(filters map[string]interface{}) (*Voucher, error) {
	if len(filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}
	list, err := QueryVouchers(filters, true)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(list) > 1 {
		return nil, fmt.Errorf("multiple vouchers matched filters")
	}
	return &list[0], nil
}

// QueryVouchers returns owner vouchers matching optional filters.
// If includeCBOR is true, the CBOR column is selected and populated.
// Results are ordered by updated_at DESC.
func QueryVouchers(filters map[string]interface{}, includeCBOR bool) ([]Voucher, error) {
	query := db.Model(&Voucher{})

	// Apply filters
	if v, ok := filters["guid"]; ok {
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid type for guid filter; want []byte")
		}
		query = query.Where("guid = ?", b)
	}
	if v, ok := filters["device_info"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid type for device_info filter; want string")
		}
		query = query.Where("device_info = ?", s)
	}

	// Omit CBOR if not needed
	if !includeCBOR {
		query = query.Omit("cbor")
	}

	// Order by updated_at DESC
	query = query.Order("updated_at DESC")

	var list []Voucher
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}

	return list, nil
}

func InsertVoucher(voucher Voucher) error {
	return db.Create(&voucher).Error
}

func InsertRVTO2Addr(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRVTO2Addr, err)
	}

	rvto2Addr := RVTO2Addr{
		ID:    1,
		Value: data,
	}
	tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rvto2Addr)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

func UpdateRVTO2Addr(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRVTO2Addr, err)
	}

	tx := db.Model(&RVTO2Addr{}).Where("id = ?", 1).Update("value", data)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func FetchRVTO2AddrJSON() ([]byte, error) {
	var rvto2Addr RVTO2Addr
	if err := db.Where("id = ?", 1).First(&rvto2Addr).Error; err != nil {
		return nil, err
	}
	return rvto2Addr.Value, nil
}

// FetchRVTO2Addr reads the rv_to2_addr JSON (stored as text) and converts it
// into []protocol.RvTO2Addr.
func FetchRVTO2Addr() ([]protocol.RvTO2Addr, error) {
	rvto2AddrData, err := FetchRVTO2AddrJSON()
	if err != nil {
		return nil, err
	}
	return parseHumanToTO2AddrsJSON(rvto2AddrData)
}

func InsertRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := parseHumanReadableRvJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	rvInfo := RvInfo{
		ID:    1,
		Value: data,
	}
	tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rvInfo)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

func UpdateRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := parseHumanReadableRvJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	tx := db.Model(&RvInfo{}).Where("id = ?", 1).Update("value", data)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func FetchRvInfoJSON() ([]byte, error) {
	var rvInfo RvInfo
	if err := db.Where("id = ?", 1).First(&rvInfo).Error; err != nil {
		return nil, err
	}
	return rvInfo.Value, nil
}

// FetchRvInfo reads the rvinfo JSON (stored as text) and converts it into
// [][]protocol.RvInstruction, CBOR-encoding each value as required by go-fdo.
func FetchRvInfo() ([][]protocol.RvInstruction, error) {
	rvInfo, err := FetchRvInfoJSON()
	if err != nil {
		return nil, err
	}
	return parseHumanReadableRvJSON(rvInfo)
}

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

// ParseHumanToTO2AddrsJSON parses a JSON like
// [{"dns":"fdo.example.com","port":"8082","protocol":"http","ip":"127.0.0.1"}]
// into []protocol.RvTO2Addr.
func parseHumanToTO2AddrsJSON(rawJSON []byte) ([]protocol.RvTO2Addr, error) {
	// Strongly-typed JSON for validation
	type to2Human struct {
		DNS      string `json:"dns"`
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
	}
	var items []to2Human
	if err := json.Unmarshal(rawJSON, &items); err != nil {
		return nil, fmt.Errorf("invalid TO2 addrs JSON: %w", err)
	}

	out := make([]protocol.RvTO2Addr, 0, len(items))
	for i, item := range items {
		var (
			ipPtr  *net.IP
			dnsPtr *string
			port   uint16
			proto  protocol.TransportProtocol
		)

		if item.IP != "" {
			ip := net.ParseIP(item.IP)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip %q", item.IP)
			}
			ipPtr = &ip
		}
		if item.DNS != "" {
			dns := item.DNS
			dnsPtr = &dns
		}
		// Spec: A given RVTO2Addr must have at least one of RVIP or RVDNS
		if ipPtr == nil && dnsPtr == nil {
			return nil, fmt.Errorf("to2[%d]: at least one of dns or ip must be specified", i)
		}
		if item.Port != "" {
			p, err := parsePortValue(item.Port)
			if err != nil {
				return nil, fmt.Errorf("port: %w", err)
			}
			port = p
		}
		if item.Protocol != "" {
			tp, err := transportProtocolFromString(item.Protocol)
			if err != nil {
				return nil, err
			}
			proto = tp
		}

		out = append(out, protocol.RvTO2Addr{
			IPAddress:         ipPtr,
			DNSAddress:        dnsPtr,
			Port:              port,
			TransportProtocol: proto,
		})
	}
	return out, nil
}

func transportProtocolFromString(s string) (protocol.TransportProtocol, error) {
	switch s {
	case "tcp":
		return protocol.TCPTransport, nil
	case "tls":
		return protocol.TLSTransport, nil
	case "http":
		return protocol.HTTPTransport, nil
	case "coap":
		return protocol.CoAPTransport, nil
	case "https":
		return protocol.HTTPSTransport, nil
	case "coaps":
		return protocol.CoAPSTransport, nil
	default:
		return 0, fmt.Errorf("unsupported transport protocol %q", s)
	}
}
