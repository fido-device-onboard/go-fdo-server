// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

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
	return "owner_vouchers"
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
