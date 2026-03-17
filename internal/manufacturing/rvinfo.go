// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package manufacturing

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// ParseOpenAPIRvJSON parses OpenAPI-formatted RvInfo JSON into [][]protocol.RvInstruction.
//
// Expected format (array of arrays of single-key objects):
//
//	[
//	  [
//	    {"dns": "rendezvous.example.com"},
//	    {"protocol": "http"},
//	    {"owner_port": 8080}
//	  ],
//	  [
//	    {"ip": "192.168.1.100"},
//	    {"protocol": "https"},
//	    {"owner_port": 8443}
//	  ]
//	]
//
// Each outer array element is an RV directive (fallback options).
// Each inner array element is a single instruction (key-value pair).
// This format aligns with the OpenAPI specification and FDO protocol CBOR structure.
func ParseOpenAPIRvJSON(rawJSON []byte) ([][]protocol.RvInstruction, error) {
	// Parse as array of arrays of single-key objects
	var directives [][]map[string]interface{}
	if err := json.Unmarshal(rawJSON, &directives); err != nil {
		return nil, fmt.Errorf("invalid rvinfo JSON (expected array of arrays): %w", err)
	}

	out := make([][]protocol.RvInstruction, 0, len(directives))

	for directiveIdx, instructions := range directives {
		group := make([]protocol.RvInstruction, 0, len(instructions))
		hasDNSorIP := false

		for instrIdx, instruction := range instructions {
			// Each instruction should be a single-key object
			if len(instruction) != 1 {
				return nil, fmt.Errorf("rvinfo[%d][%d]: each instruction must be a single-key object, got %d keys",
					directiveIdx, instrIdx, len(instruction))
			}

			// Extract the single key-value pair
			var key string
			var value interface{}
			for k, v := range instruction {
				key = k
				value = v
				break
			}

			// Map key to protocol instruction and encode value
			rvInstr, err := parseRvInstruction(key, value, parsePortValue)
			if err != nil {
				return nil, fmt.Errorf("rvinfo[%d][%d]: %w", directiveIdx, instrIdx, err)
			}

			// Track if we have DNS or IP
			if rvInstr.Variable == protocol.RVDns || rvInstr.Variable == protocol.RVIPAddress {
				hasDNSorIP = true
			}

			group = append(group, *rvInstr)
		}

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if !hasDNSorIP {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", directiveIdx)
		}

		out = append(out, group)
	}

	return out, nil
}

// parseRvInstruction converts a single key-value pair into a protocol.RvInstruction
func parseRvInstruction(key string, value interface{}, portParser func(any) (uint16, error)) (*protocol.RvInstruction, error) {
	switch key {
	case "dns":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("dns must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVDns, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDns, Value: enc}, nil

	case "ip":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("ip must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVIPAddress, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc}, nil

	case "protocol":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("protocol must be a string, got %T", value)
		}
		code, err := protocolCodeFromString(str)
		if err != nil {
			return nil, err
		}
		enc, err := encodeRvValue(protocol.RVProtocol, uint8(code))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc}, nil

	case "medium":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("medium must be a string, got %T", value)
		}
		m, err := parseMediumValue(str)
		if err != nil {
			return nil, err
		}
		enc, err := encodeRvValue(protocol.RVMedium, uint8(m))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc}, nil

	case "device_port":
		num, err := portParser(value)
		if err != nil {
			return nil, fmt.Errorf("device_port: %w", err)
		}
		enc, err := encodeRvValue(protocol.RVDevPort, num)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc}, nil

	case "owner_port":
		num, err := portParser(value)
		if err != nil {
			return nil, fmt.Errorf("owner_port: %w", err)
		}
		enc, err := encodeRvValue(protocol.RVOwnerPort, num)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc}, nil

	case "wifi_ssid":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("wifi_ssid must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVWifiSsid, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc}, nil

	case "wifi_pw":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("wifi_pw must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVWifiPw, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc}, nil

	case "dev_only":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("dev_only must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVDevOnly}, nil
		}
		return nil, fmt.Errorf("dev_only is false, should be omitted instead")

	case "owner_only":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("owner_only must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVOwnerOnly}, nil
		}
		return nil, fmt.Errorf("owner_only is false, should be omitted instead")

	case "rv_bypass":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("rv_bypass must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVBypass}, nil
		}
		return nil, fmt.Errorf("rv_bypass is false, should be omitted instead")

	case "delay_seconds":
		// OpenAPI spec requires integer type
		num, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("delay_seconds must be an integer, got %T", value)
		}

		// Ensure it's a whole number
		if num != math.Trunc(num) {
			return nil, fmt.Errorf("delay_seconds must be a whole number, got %v", num)
		}

		// Validate non-negative
		if num < 0 {
			return nil, fmt.Errorf("delay_seconds must be non-negative, got %v", num)
		}

		secs := uint32(num)
		enc, err := encodeRvValue(protocol.RVDelaysec, uint64(secs))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc}, nil

	case "sv_cert_hash":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("sv_cert_hash must be a string, got %T", value)
		}
		b, err := hex.DecodeString(str)
		if err != nil {
			return nil, fmt.Errorf("sv_cert_hash: %w", err)
		}
		enc, err := cbor.Marshal(b)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc}, nil

	case "cl_cert_hash":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("cl_cert_hash must be a string, got %T", value)
		}
		b, err := hex.DecodeString(str)
		if err != nil {
			return nil, fmt.Errorf("cl_cert_hash: %w", err)
		}
		enc, err := cbor.Marshal(b)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc}, nil

	case "user_input":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("user_input must be a boolean, got %T", value)
		}
		if b {
			// Only include instruction if true (per FDO spec)
			return &protocol.RvInstruction{Variable: protocol.RVUserInput}, nil
		}
		return nil, fmt.Errorf("user_input is false, should be omitted instead")

	case "ext_rv":
		arr, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("ext_rv must be an array, got %T", value)
		}
		// Convert []interface{} to []string
		strArr := make([]string, len(arr))
		for i, v := range arr {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ext_rv[%d] must be a string, got %T", i, v)
			}
			strArr[i] = s
		}
		enc, err := encodeRvValue(protocol.RVExtRV, strArr)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc}, nil

	default:
		return nil, fmt.Errorf("unknown instruction key: %q", key)
	}
}

// encodeRvValue encodes a value for an RV instruction based on its variable type
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

// parsePortValue validates port values for V2 OpenAPI spec.
// Only accepts integers (float64 in JSON). Rejects strings for strict type compliance.
func parsePortValue(v any) (uint16, error) {
	// Only accept integer (float64 in JSON)
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("port must be an integer, got %T", v)
	}

	// Ensure it's a whole number (not 8080.5)
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("port must be a whole number, got %v", f)
	}

	// Validate port range (1-65535)
	if f < 1 || f > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %v", f)
	}

	return uint16(f), nil
}

// protocolCodeFromString converts protocol string to protocol code
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

// parseMediumValue parses the medium value (network interface type)
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
