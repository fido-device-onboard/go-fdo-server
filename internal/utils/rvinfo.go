// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"fmt"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

// ProtocolStringFromCode converts protocol code to string representation
func ProtocolStringFromCode(code uint8) string {
	switch code {
	case uint8(protocol.RVProtRest):
		return "rest"
	case uint8(protocol.RVProtHTTP):
		return "http"
	case uint8(protocol.RVProtHTTPS):
		return "https"
	case uint8(protocol.RVProtTCP):
		return "tcp"
	case uint8(protocol.RVProtTLS):
		return "tls"
	case uint8(protocol.RVProtCoapTCP):
		return "coap+tcp"
	case uint8(protocol.RVProtCoapUDP):
		return "coap"
	default:
		return fmt.Sprintf("%d", code)
	}
}

// MediumStringFromCode converts medium code to string representation
func MediumStringFromCode(medium uint8) string {
	switch medium {
	case protocol.RVMedEthAll:
		return "eth_all"
	case protocol.RVMedWifiAll:
		return "wifi_all"
	default:
		return fmt.Sprintf("%d", medium)
	}
}

// ProtocolCodeFromString converts protocol string to protocol code
// This is the inverse of ProtocolStringFromCode
func ProtocolCodeFromString(s string) (uint8, error) {
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

// MediumCodeFromString converts medium string to medium code
// This is the inverse of MediumStringFromCode
func MediumCodeFromString(s string) (uint8, error) {
	switch s {
	case "eth_all":
		return protocol.RVMedEthAll, nil
	case "wifi_all":
		return protocol.RVMedWifiAll, nil
	default:
		return 0, fmt.Errorf("unsupported medium %q", s)
	}
}
