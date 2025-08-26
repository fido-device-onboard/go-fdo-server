// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

type Data struct {
	Value interface{} `json:"value"`
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
	GUID       GUID      `json:"guid"`
	CBOR       []byte    `json:"cbor,omitempty"`
	DeviceInfo string    `json:"device_info"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OwnerKey struct {
	Type      int    `json:"type"`
	PKCS8     []byte `json:"pkcs8"`
	X509Chain []byte `json:"x509_chain"`
}
