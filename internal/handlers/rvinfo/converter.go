// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// convertToV2JSON converts [][]protocol.RvInstruction to V2 OpenAPI JSON format.
// This is the V2 API's responsibility - JSON conversion happens at the API layer.
//
// Output format (array of arrays of single-key objects):
// [[{"dns":"host"},{"protocol":"http"},{"owner_port":8080}]]
func convertToV2JSON(rvInstructions [][]protocol.RvInstruction) ([]byte, error) {
	out := make([][]map[string]interface{}, 0, len(rvInstructions))

	for _, directive := range rvInstructions {
		group := make([]map[string]interface{}, 0, len(directive))

		for _, instr := range directive {
			item := make(map[string]interface{})

			switch instr.Variable {
			case protocol.RVDns:
				var dns string
				if err := cbor.Unmarshal(instr.Value, &dns); err != nil {
					return nil, fmt.Errorf("failed to unmarshal dns: %w", err)
				}
				item["dns"] = dns

			case protocol.RVIPAddress:
				var ip net.IP
				if err := cbor.Unmarshal(instr.Value, &ip); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ip: %w", err)
				}
				item["ip"] = ip.String()

			case protocol.RVProtocol:
				var code uint8
				if err := cbor.Unmarshal(instr.Value, &code); err != nil {
					return nil, fmt.Errorf("failed to unmarshal protocol: %w", err)
				}
				item["protocol"] = utils.ProtocolStringFromCode(code)

			case protocol.RVMedium:
				var medium uint8
				if err := cbor.Unmarshal(instr.Value, &medium); err != nil {
					return nil, fmt.Errorf("failed to unmarshal medium: %w", err)
				}
				item["medium"] = utils.MediumStringFromCode(medium)

			case protocol.RVDevPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal device_port: %w", err)
				}
				item["device_port"] = int(port) // V2 uses integer, not string

			case protocol.RVOwnerPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal owner_port: %w", err)
				}
				item["owner_port"] = int(port) // V2 uses integer, not string

			case protocol.RVWifiSsid:
				var ssid string
				if err := cbor.Unmarshal(instr.Value, &ssid); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_ssid: %w", err)
				}
				item["wifi_ssid"] = ssid

			case protocol.RVWifiPw:
				var pw string
				if err := cbor.Unmarshal(instr.Value, &pw); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_pw: %w", err)
				}
				item["wifi_pw"] = pw

			case protocol.RVDevOnly:
				item["dev_only"] = true

			case protocol.RVOwnerOnly:
				item["owner_only"] = true

			case protocol.RVBypass:
				item["rv_bypass"] = true

			case protocol.RVDelaysec:
				var secs uint32
				if err := cbor.Unmarshal(instr.Value, &secs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal delay_seconds: %w", err)
				}
				item["delay_seconds"] = int(secs)

			case protocol.RVSvCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal sv_cert_hash: %w", err)
				}
				item["sv_cert_hash"] = hex.EncodeToString(hash)

			case protocol.RVClCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal cl_cert_hash: %w", err)
				}
				item["cl_cert_hash"] = hex.EncodeToString(hash)

			case protocol.RVUserInput:
				item["user_input"] = true

			case protocol.RVExtRV:
				var extrv []string
				if err := cbor.Unmarshal(instr.Value, &extrv); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ext_rv: %w", err)
				}
				item["ext_rv"] = extrv
			}

			group = append(group, item)
		}

		out = append(out, group)
	}

	return json.Marshal(out)
}
