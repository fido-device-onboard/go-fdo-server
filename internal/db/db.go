// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

var db *sql.DB

func InitDb(state *sqlite.DB) error {
	db = state.DB()
	if err := createRvTable(); err != nil {
		slog.Error("Failed to create table")
		return err
	}
	if err := createOwnerInfoTable(); err != nil {
		slog.Error("Failed to create table")
		return err
	}
	return nil
}

func createRvTable() error {
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

func FetchOwnerKeys() ([]OwnerKey, error) {
	rows, err := db.Query("SELECT type, pkcs8, x509_chain FROM owner_keys")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ownerKeys []OwnerKey
	for rows.Next() {
		var ownerKey OwnerKey
		if err := rows.Scan(&ownerKey.Type, &ownerKey.PKCS8, &ownerKey.X509Chain); err != nil {
			return nil, err
		}
		ownerKeys = append(ownerKeys, ownerKey)
	}
	return ownerKeys, nil
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

func UpdateOwnerKeys(ownerKeys []OwnerKey) error {
	for _, ownerKey := range ownerKeys {
		_, err := db.Exec("UPDATE owner_keys SET pkcs8 = ?, x509_chain = ? WHERE type = ?", ownerKey.PKCS8, ownerKey.X509Chain, ownerKey.Type)
		if err != nil {
			return err
		}
	}
	return nil
}

func CheckDataExists(tableName string) (bool, error) {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = 1", tableName)
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("error counting rows: %w", err)
	}
	return count > 0, nil
}

func InsertData(data Data, tableName string) error {
	value, err := json.Marshal(data.Value)
	if err != nil {
		return fmt.Errorf("error marshalling value: %w", err)
	}
	query := fmt.Sprintf("INSERT INTO %s (id, value) VALUES (1, ?)", tableName)
	_, err = db.Exec(query, string(value))
	if err != nil {
		return fmt.Errorf("error inserting data: %w", err)
	}
	return nil
}

func UpdateDataInDB(data Data, tableName string) error {
	value, err := json.Marshal(data.Value)
	if err != nil {
		return fmt.Errorf("error marshalling value: %w", err)
	}
	query := fmt.Sprintf("UPDATE %s SET value = ? WHERE id = 1", tableName)
	_, err = db.Exec(query, string(value))
	if err != nil {
		return fmt.Errorf("error updating data: %w", err)
	}
	return nil
}

func FetchData(tableName string) (Data, error) {
	var data Data
	var value string
	query := fmt.Sprintf("SELECT value FROM %s WHERE id = 1", tableName)
	err := db.QueryRow(query).Scan(&value)
	if err != nil {
		return data, err
	}

	if err := json.Unmarshal([]byte(value), &data.Value); err != nil {
		return data, err
	}

	return data, nil
}

// FetchRvData reads the rvinfo JSON (stored as text) and converts it into
// [][]protocol.RvInstruction, CBOR-encoding each value as required by go-fdo.
// Expected JSON format: [[[var, value], [var, value], ...], ...]
func FetchRvData() ([][]protocol.RvInstruction, error) {
	var value string
	if err := db.QueryRow("SELECT value FROM rvinfo WHERE id = 1").Scan(&value); err != nil {
		return nil, err
	}

	var raw any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, fmt.Errorf("error unmarshalling rvInfo: %w", err)
	}

	outer, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid rvinfo format: outer not array")
	}

	out := make([][]protocol.RvInstruction, 0, len(outer))
	for _, groupVal := range outer {
		groupArr, ok := groupVal.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid rvinfo format: group not array")
		}
		group := make([]protocol.RvInstruction, 0, len(groupArr))
		for _, pairVal := range groupArr {
			var (
				rvVar protocol.RvVar
				val   any
			)
			typed, ok := pairVal.([]any)
			if !ok || len(typed) != 2 {
				return nil, fmt.Errorf("invalid rvinfo format: pair not [var,value]")
			}
			fvar, ok := typed[0].(float64)
			if !ok {
				return nil, fmt.Errorf("invalid rv var type: %T", typed[0])
			}
			rvVar = protocol.RvVar(uint8(fvar))
			val = typed[1]

			// Value CBOR-encoding by variable type
			enc, err := encodeRvValue(rvVar, val)
			if err != nil {
				return nil, err
			}
			// Bucketize to ensure ports override protocol defaults
			instr := protocol.RvInstruction{Variable: rvVar, Value: enc}
			group = append(group, instr)
		}
		// Reorder: others -> protocol -> ports
		var others, protocols, ports []protocol.RvInstruction
		for _, instr := range group {
			switch instr.Variable {
			case protocol.RVDevPort, protocol.RVOwnerPort:
				ports = append(ports, instr)
			case protocol.RVProtocol:
				protocols = append(protocols, instr)
			default:
				others = append(others, instr)
			}
		}
		reordered := make([]protocol.RvInstruction, 0, len(group))
		reordered = append(reordered, others...)
		reordered = append(reordered, protocols...)
		reordered = append(reordered, ports...)
		out = append(out, reordered)
	}

	return out, nil
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
	case float64:
		// JSON numbers -> coerce by variable semantics
		switch rvVar {
		case protocol.RVDevPort, protocol.RVOwnerPort:
			return cbor.Marshal(uint16(v))
		case protocol.RVProtocol, protocol.RVMedium:
			return cbor.Marshal(uint8(v))
		case protocol.RVDelaysec:
			return cbor.Marshal(uint64(v))
		default:
			return cbor.Marshal(int64(v))
		}
	default:
		return cbor.Marshal(v)
	}
}

// FetchOwnerInfoData reads the owner_info JSON (stored as text) and converts it
// into []protocol.RvTO2Addr. Expected JSON format:
// [[ipOrNull, dnsOrNull, port, proto], ...]
func FetchOwnerInfoData() ([]protocol.RvTO2Addr, error) {
	var value string
	if err := db.QueryRow("SELECT value FROM owner_info WHERE id = 1").Scan(&value); err != nil {
		return nil, err
	}

	var raw any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, fmt.Errorf("error unmarshalling owner_info: %w", err)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid owner_info format: outer not array")
	}

	out := make([]protocol.RvTO2Addr, 0, len(items))
	for _, it := range items {
		arr, ok := it.([]any)
		if !ok || len(arr) != 4 {
			return nil, fmt.Errorf("invalid owner_info item: expected 4 elements")
		}

		// ip (string or null)
		var ipPtr *net.IP
		if arr[0] != nil {
			if s, ok := arr[0].(string); ok {
				ip := net.ParseIP(s)
				ipPtr = &ip
			} else {
				return nil, fmt.Errorf("invalid ip element type: %T", arr[0])
			}
		}

		// dns (string or null)
		var dnsPtr *string
		if arr[1] != nil {
			if s, ok := arr[1].(string); ok {
				dnsPtr = &s
			} else {
				return nil, fmt.Errorf("invalid dns element type: %T", arr[1])
			}
		}

		// port (number)
		fport, ok := arr[2].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid port element type: %T", arr[2])
		}
		port := uint16(fport)

		// proto (number)
		fproto, ok := arr[3].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid proto element type: %T", arr[3])
		}
		proto := protocol.TransportProtocol(fproto)

		out = append(out, protocol.RvTO2Addr{
			IPAddress:         ipPtr,
			DNSAddress:        dnsPtr,
			Port:              port,
			TransportProtocol: proto,
		})
	}
	return out, nil
}
