// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

// Secret stores encryption secrets
type Secret struct {
	Type   string `gorm:"type:text;not null"`
	Secret []byte `gorm:"not null"`
}

// TableName specifies the table name for Secret model
func (Secret) TableName() string {
	return "secrets"
}

// MfgKey stores manufacturer keys
type MfgKey struct {
	Type      int    `gorm:"type:integer;not null;primaryKey"`
	PKCS8     []byte `gorm:"not null"`
	RsaBits   *int   `gorm:"type:integer;primaryKey"`
	X509Chain []byte `gorm:"not null"`
}

// TableName specifies the table name for MfgKey model
func (MfgKey) TableName() string {
	return "mfg_keys"
}

// OwnerKey stores owner keys
type OwnerKey struct {
	Type      int    `gorm:"type:integer;not null;primaryKey"`
	PKCS8     []byte `gorm:"not null"`
	RsaBits   *int   `gorm:"type:integer;primaryKey"`
	X509Chain []byte
}

// TableName specifies the table name for OwnerKey model
func (OwnerKey) TableName() string {
	return "owner_keys"
}

// RvBlob stores rendezvous blobs
type RvBlob struct {
	GUID    []byte    `gorm:"primaryKey"`
	RV      []byte    `gorm:"not null"`
	Voucher []byte    `gorm:"not null"`
	Exp     time.Time `gorm:"not null;index:idx_rv_blob_exp"`
}

// TableName specifies the table name for RvBlob model
func (RvBlob) TableName() string {
	return "rv_blobs"
}

// Session stores session information
type Session struct {
	ID       []byte `gorm:"primaryKey"`
	Protocol int    `gorm:"type:integer;not null"`
}

// TableName specifies the table name for Session model
func (Session) TableName() string {
	return "sessions"
}

// DeviceInfo stores device information
type DeviceInfo struct {
	Session      []byte  `gorm:"index"`
	KeyType      *int    `gorm:"type:integer"`
	KeyEncoding  *int    `gorm:"type:integer"`
	SerialNumber *string `gorm:"type:text"`
	InfoString   *string `gorm:"type:text"`
	CSR          []byte
	X509Chain    []byte `gorm:"not null"`
}

// TableName specifies the table name for DeviceInfo model
func (DeviceInfo) TableName() string {
	return "device_info"
}

// IncompleteVoucher stores incomplete voucher headers
type IncompleteVoucher struct {
	Session []byte `gorm:"primaryKey"`
	Header  []byte `gorm:"not null"`
}

// TableName specifies the table name for IncompleteVoucher model
func (IncompleteVoucher) TableName() string {
	return "incomplete_vouchers"
}

// TO0Session stores TO0 session state
type TO0Session struct {
	Session []byte `gorm:"primaryKey"`
	Nonce   []byte
}

// TableName specifies the table name for TO0Session model
func (TO0Session) TableName() string {
	return "to0_sessions"
}

// TO1Session stores TO1 session state
type TO1Session struct {
	Session []byte `gorm:"primaryKey"`
	Nonce   []byte
	Alg     *int `gorm:"type:integer"`
}

// TableName specifies the table name for TO1Session model
func (TO1Session) TableName() string {
	return "to1_sessions"
}

// TO2Session stores TO2 session state
type TO2Session struct {
	Session        []byte `gorm:"primaryKey"`
	GUID           []byte
	RvInfo         []byte
	ProveDevice    []byte
	SetupDevice    []byte
	MTU            *int `gorm:"type:integer"`
	Devmod         []byte
	Modules        []byte
	DevmodComplete *bool `gorm:"type:boolean"`
}

// TableName specifies the table name for TO2Session model
func (TO2Session) TableName() string {
	return "to2_sessions"
}

// ReplacementVoucher stores replacement vouchers during TO2
type ReplacementVoucher struct {
	Session []byte `gorm:"primaryKey"`
	GUID    []byte
	Hmac    []byte
}

// TableName specifies the table name for ReplacementVoucher model
func (ReplacementVoucher) TableName() string {
	return "replacement_vouchers"
}

// KeyExchange stores key exchange sessions
type KeyExchange struct {
	Session []byte `gorm:"primaryKey"`
	Suite   string `gorm:"type:text;not null"`
	CBOR    []byte `gorm:"not null"`
}

// TableName specifies the table name for KeyExchange model
func (KeyExchange) TableName() string {
	return "key_exchanges"
}

type GUID []byte

func (t *GUID) UnmarshalJSON(b []byte) (err error) {
	var g string
	if err = json.Unmarshal(b, &g); err != nil {
		return
	}
	*t, err = hex.DecodeString(g)
	return
}

func (t *GUID) MarshalJSON() (b []byte, err error) {
	return json.Marshal(hex.EncodeToString(*t))
}

type Voucher struct {
	GUID       GUID      `json:"guid" gorm:"primaryKey"`
	CBOR       []byte    `json:"cbor,omitempty"`
	DeviceInfo string    `json:"device_info" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

// TableName specifies the table name for Voucher model
func (Voucher) TableName() string {
	return "vouchers"
}

type OwnerInfo struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"type:text;not null"`
}

// TableName specifies the table name for OwnerInfo model
func (OwnerInfo) TableName() string {
	return "owner_info"
}

type RvInfo struct {
	ID    int    `gorm:"primaryKey;check:id = 1"`
	Value []byte `gorm:"type:text;not null"`
}

// TableName specifies the table name for RvInfo model
func (RvInfo) TableName() string {
	return "rvinfo"
}

// DeviceOnboarding tracks TO2 completion per device GUID
type DeviceOnboarding struct {
	GUID           GUID `gorm:"primaryKey"`
	NewGUID        GUID `gorm:"index"`
	TO2Completed   bool `gorm:"type:boolean;not null;default:false"`
	TO2CompletedAt *time.Time
}

// TableName specifies the table name for DeviceOnboarding model
func (DeviceOnboarding) TableName() string {
	return "device_onboarding"
}

// Device is a projection used by the owner API to expose
// voucher metadata together with TO2 onboarding state for each device.
type Device struct {
	GUID           GUID       `json:"guid" gorm:"column:guid"`
	OldGUID        GUID       `json:"old_guid" gorm:"column:old_guid"`
	DeviceInfo     string     `json:"device_info" gorm:"column:device_info"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at"`
	TO2Completed   bool       `json:"to2_completed" gorm:"column:to2_completed"`
	TO2CompletedAt *time.Time `json:"to2_completed_at,omitempty" gorm:"column:to2_completed_at"`
}
