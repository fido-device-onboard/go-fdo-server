// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

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

func FetchVoucher(guid []byte) (Voucher, error) {
	var voucher Voucher
	err := db.QueryRow("SELECT guid, cbor FROM owner_vouchers WHERE guid = ?", guid).Scan(&voucher.GUID, &voucher.CBOR)
	return voucher, err
}

func FetchVouchers() ([]Voucher, error) {
	var vouchers []Voucher
	rows, err := db.Query("SELECT guid, cbor FROM owner_vouchers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var voucher Voucher
		if err := rows.Scan(&voucher.GUID, &voucher.CBOR); err != nil {
			return vouchers, err
		}
		vouchers = append(vouchers, voucher)
	}
	if err = rows.Err(); err != nil {
		return vouchers, err
	}
	return vouchers, err
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
	_, err := db.Exec("INSERT INTO owner_vouchers (guid, cbor) VALUES (?, ?)", voucher.GUID, voucher.CBOR)
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
