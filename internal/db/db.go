// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

var db *sql.DB

func InitDb(state *sqlite.DB) error {
	db = state.DB()
	if err := createRvInfoTable(); err != nil {
		slog.Error("Failed to create table")
		return err
	}
	if err := createOwnerInfoTable(); err != nil {
		slog.Error("Failed to create table")
		return err
	}
	return nil
}

func createRvInfoTable() error {
	query := `CREATE TABLE IF NOT EXISTS rvinfo (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		value TEXT
	);`
	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	return nil
}

func createOwnerInfoTable() error {
	query := `CREATE TABLE IF NOT EXISTS owner_info (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		value TEXT
	);`
	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	return nil
}

// FetchVoucher doesn't take into account mfg_voucher where go-fdo stores vouchers that have not been extended yet
// we don't need it right now, but for use cases where manufacturers just initializes empty devices (tpm) this is going to be needed.
//
// FetchVoucher returns a single voucher filtered by provided fields.
// Supported filters (keys):
// - "guid" (expects []byte)
// - "device_info" (expects string)
// If more than one voucher matches, an error is returned.
// Note: This does not query mfg_voucher (unextended vouchers).
func FetchVoucher(filters map[string]interface{}) (*Voucher, error) {
	if len(filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}
	list, err := QueryVouchers(filters, true)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
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
	var query, fields string
	fields = "guid, device_info, created_at, updated_at"
	if includeCBOR {
		fields += ", cbor"
	}
	query = fmt.Sprintf("SELECT %s FROM owner_vouchers WHERE 1=1", fields)
	args := make([]interface{}, 0, 2)
	if v, ok := filters["guid"]; ok {
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid type for guid filter; want []byte")
		}
		query += " AND guid = ?"
		args = append(args, b)
	}
	if v, ok := filters["device_info"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid type for device_info filter; want string")
		}
		query += " AND device_info = ?"
		args = append(args, s)
	}
	query += " ORDER BY updated_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []Voucher
	for rows.Next() {
		var v Voucher
		var createdAt, updatedAt int64
		dest := []any{&v.GUID, &v.DeviceInfo, &createdAt, &updatedAt}
		if includeCBOR {
			dest = append(dest, &v.CBOR)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		v.CreatedAt = time.UnixMicro(createdAt)
		v.UpdatedAt = time.UnixMicro(updatedAt)
		list = append(list, v)
	}
	return list, nil
}

func InsertVoucher(voucher Voucher) error {
	_, err := db.Exec(
		"INSERT INTO owner_vouchers (guid, device_info, cbor, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		voucher.GUID,
		voucher.DeviceInfo,
		voucher.CBOR,
		voucher.CreatedAt.UnixMicro(),
		voucher.UpdatedAt.UnixMicro(),
	)
	return err
}

func insertData(data []byte, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (id, value) VALUES (1, ?)", tableName)
	if _, err := db.Exec(query, data); err != nil {
		return fmt.Errorf("error inserting data: %w", err)
	}
	return nil
}

func updateData(data []byte, tableName string) error {
	query := fmt.Sprintf("UPDATE %s SET value = ? WHERE id = 1", tableName)
	if _, err := db.Exec(query, data); err != nil {
		return fmt.Errorf("error updating data: %w", err)
	}
	return nil
}

func InsertOwnerInfo(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("error parsing ownerinfo data: %w", err)
	}
	return insertData(data, "owner_info")
}

func UpdateOwnerInfo(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("error parsing ownerinfo data: %w", err)
	}
	return updateData(data, "owner_info")
}

func FetchOwnerInfoJSON() ([]byte, error) {
	var value []byte
	if err := db.QueryRow("SELECT value FROM owner_info WHERE id = 1").Scan(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// FetchOwnerInfoData reads the owner_info JSON (stored as text) and converts it
// into []protocol.RvTO2Addr.
func FetchOwnerInfo() ([]protocol.RvTO2Addr, error) {
	ownerInfoData, err := FetchOwnerInfoJSON()
	if err != nil {
		return nil, err
	}
	return parseHumanToTO2AddrsJSON(ownerInfoData)
}

func InsertRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := parseHumanReadableRvJSON(data); err != nil {
		return fmt.Errorf("error parsing rvinfo data: %w", err)
	}
	return insertData(data, "rvinfo")
}

func UpdateRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := parseHumanReadableRvJSON(data); err != nil {
		return fmt.Errorf("error parsing rvinfo data: %w", err)
	}
	return updateData(data, "rvinfo")
}

func FetchRvInfoJSON() ([]byte, error) {
	var value []byte
	if err := db.QueryRow("SELECT value FROM rvinfo WHERE id = 1").Scan(&value); err != nil {
		return nil, err
	}
	return value, nil
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
			return cbor.Marshal(net.ParseIP(v))
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
		var (
			others    []protocol.RvInstruction
			protocols []protocol.RvInstruction
			ports     []protocol.RvInstruction
		)

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if item.DNS == "" && item.IP == "" {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", i)
		}

		if item.DNS != "" {
			enc, err := encodeRvValue(protocol.RVDns, item.DNS)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVDns, Value: enc})
		}
		if item.IP != "" {
			enc, err := encodeRvValue(protocol.RVIPAddress, item.IP)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc})
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
			protocols = append(protocols, protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc})
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
			others = append(others, protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc})
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
			ports = append(ports, protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc})
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
			ports = append(ports, protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc})
		}
		if item.WifiSSID != "" {
			enc, err := encodeRvValue(protocol.RVWifiSsid, item.WifiSSID)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc})
		}
		if item.WifiPW != "" {
			enc, err := encodeRvValue(protocol.RVWifiPw, item.WifiPW)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc})
		}
		if item.DevOnly {
			others = append(others, protocol.RvInstruction{Variable: protocol.RVDevOnly})
		}
		if item.OwnerOnly {
			others = append(others, protocol.RvInstruction{Variable: protocol.RVOwnerOnly})
		}
		if item.RvBypass {
			others = append(others, protocol.RvInstruction{Variable: protocol.RVBypass})
		}
		if item.DelaySeconds != nil {
			secs := uint64(*item.DelaySeconds)
			enc, err := encodeRvValue(protocol.RVDelaysec, secs)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc})
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
			others = append(others, protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc})
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
			others = append(others, protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc})
		}
		if item.UserInput != "" {
			enc, err := encodeRvValue(protocol.RVUserInput, item.UserInput)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVUserInput, Value: enc})
		}
		if item.ExtRV != "" {
			enc, err := encodeRvValue(protocol.RVExtRV, item.ExtRV)
			if err != nil {
				return nil, err
			}
			others = append(others, protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc})
		}

		// grouping is here because of a bug https://github.com/fido-device-onboard/go-fdo/issues/145
		group := make([]protocol.RvInstruction, 0, len(others)+len(protocols)+len(ports))
		group = append(group, others...)
		group = append(group, protocols...)
		group = append(group, ports...)
		out = append(out, group)
	}
	return out, nil
}

func parsePortValue(v any) (uint16, error) {
	switch t := v.(type) {
	case float64:
		return uint16(t), nil
	case string:
		if t == "" {
			return 0, fmt.Errorf("empty")
		}
		i, err := strconv.Atoi(t)
		if err != nil {
			return 0, err
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
