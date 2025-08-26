// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import "time"

type Data struct {
	Value interface{} `json:"value"`
}

type Voucher struct {
	GUID       []byte    `json:"guid"`
	CBOR       []byte    `json:"cbor"`
	DeviceInfo string    `json:"device_info"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OwnerKey struct {
	Type      int    `json:"type"`
	PKCS8     []byte `json:"pkcs8"`
	X509Chain []byte `json:"x509_chain"`
}
