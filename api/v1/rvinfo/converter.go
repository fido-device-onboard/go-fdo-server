// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
)

// RVInfoV1ToProtocol converts V1 API RVInfo (flat directives) to protocol format
func RVInfoV1ToProtocol(rvInfo components.RVInfo) ([][]protocol.RvInstruction, error) {
	out := make([][]protocol.RvInstruction, 0, len(rvInfo))

	for i, directive := range rvInfo {
		group := make([]protocol.RvInstruction, 0)

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if directive.Dns == nil && directive.Ip == nil {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", i)
		}

		// DNS
		if directive.Dns != nil {
			enc, err := cbor.Marshal(*directive.Dns)
			if err != nil {
				return nil, fmt.Errorf("failed to encode dns: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDns, Value: enc})
		}

		// IP
		if directive.Ip != nil {
			ip := net.ParseIP(*directive.Ip)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip %q", *directive.Ip)
			}
			enc, err := cbor.Marshal(ip)
			if err != nil {
				return nil, fmt.Errorf("failed to encode ip: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc})
		}

		// Protocol
		if directive.Protocol != nil {
			code, err := protocolStringToCode(string(*directive.Protocol))
			if err != nil {
				return nil, err
			}
			enc, err := cbor.Marshal(code)
			if err != nil {
				return nil, fmt.Errorf("failed to encode protocol: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc})
		}

		// Medium
		if directive.Medium != nil {
			code, err := mediumStringToCode(string(*directive.Medium))
			if err != nil {
				return nil, err
			}
			enc, err := cbor.Marshal(code)
			if err != nil {
				return nil, fmt.Errorf("failed to encode medium: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc})
		}

		// Device Port (V1 uses string format)
		if directive.DevicePort != nil {
			port, err := parsePortString(*directive.DevicePort)
			if err != nil {
				return nil, fmt.Errorf("device_port: %w", err)
			}
			enc, err := cbor.Marshal(port)
			if err != nil {
				return nil, fmt.Errorf("failed to encode device_port: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc})
		}

		// Owner Port (V1 uses string format)
		if directive.OwnerPort != nil {
			port, err := parsePortString(*directive.OwnerPort)
			if err != nil {
				return nil, fmt.Errorf("owner_port: %w", err)
			}
			enc, err := cbor.Marshal(port)
			if err != nil {
				return nil, fmt.Errorf("failed to encode owner_port: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc})
		}

		// WiFi SSID
		if directive.WifiSsid != nil {
			enc, err := cbor.Marshal(*directive.WifiSsid)
			if err != nil {
				return nil, fmt.Errorf("failed to encode wifi_ssid: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc})
		}

		// WiFi Password
		if directive.WifiPw != nil {
			enc, err := cbor.Marshal(*directive.WifiPw)
			if err != nil {
				return nil, fmt.Errorf("failed to encode wifi_pw: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc})
		}

		// DevOnly (boolean flag, only include if true)
		if directive.DevOnly != nil && *directive.DevOnly {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDevOnly})
		}

		// OwnerOnly (boolean flag, only include if true)
		if directive.OwnerOnly != nil && *directive.OwnerOnly {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerOnly})
		}

		// RvBypass (boolean flag, only include if true)
		if directive.RvBypass != nil && *directive.RvBypass {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVBypass})
		}

		// DelaySeconds
		if directive.DelaySeconds != nil {
			enc, err := cbor.Marshal(*directive.DelaySeconds)
			if err != nil {
				return nil, fmt.Errorf("failed to encode delay_seconds: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc})
		}

		// SvCertHash
		if directive.SvCertHash != nil {
			hash, err := hex.DecodeString(*directive.SvCertHash)
			if err != nil {
				return nil, fmt.Errorf("sv_cert_hash: %w", err)
			}
			enc, err := cbor.Marshal(hash)
			if err != nil {
				return nil, fmt.Errorf("failed to encode sv_cert_hash: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc})
		}

		// ClCertHash
		if directive.ClCertHash != nil {
			hash, err := hex.DecodeString(*directive.ClCertHash)
			if err != nil {
				return nil, fmt.Errorf("cl_cert_hash: %w", err)
			}
			enc, err := cbor.Marshal(hash)
			if err != nil {
				return nil, fmt.Errorf("failed to encode cl_cert_hash: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc})
		}

		// UserInput (V1 stores as string "true")
		if directive.UserInput != nil && *directive.UserInput != "" {
			group = append(group, protocol.RvInstruction{Variable: protocol.RVUserInput})
		}

		// ExtRv (V1 stores as JSON string)
		if directive.ExtRv != nil {
			var extrv []string
			if err := json.Unmarshal([]byte(*directive.ExtRv), &extrv); err != nil {
				return nil, fmt.Errorf("ext_rv: invalid JSON: %w", err)
			}
			enc, err := cbor.Marshal(extrv)
			if err != nil {
				return nil, fmt.Errorf("failed to encode ext_rv: %w", err)
			}
			group = append(group, protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc})
		}

		out = append(out, group)
	}

	return out, nil
}

// RVInfoV1FromProtocol converts protocol format back to V1 API RVInfo (flat directives)
func RVInfoV1FromProtocol(proto [][]protocol.RvInstruction) (components.RVInfo, error) {
	out := make(components.RVInfo, 0, len(proto))

	for _, directive := range proto {
		item := components.RVDirective{}

		for _, instr := range directive {
			switch instr.Variable {
			case protocol.RVDns:
				var dns string
				if err := cbor.Unmarshal(instr.Value, &dns); err != nil {
					return nil, fmt.Errorf("failed to unmarshal dns: %w", err)
				}
				item.Dns = &dns

			case protocol.RVIPAddress:
				var ip net.IP
				if err := cbor.Unmarshal(instr.Value, &ip); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ip: %w", err)
				}
				ipStr := ip.String()
				item.Ip = &ipStr

			case protocol.RVProtocol:
				var code uint8
				if err := cbor.Unmarshal(instr.Value, &code); err != nil {
					return nil, fmt.Errorf("failed to unmarshal protocol: %w", err)
				}
				protoStr, err := protocolCodeToString(code)
				if err != nil {
					return nil, err
				}
				protoType := components.ProtocolType(protoStr)
				item.Protocol = &protoType

			case protocol.RVMedium:
				var code uint8
				if err := cbor.Unmarshal(instr.Value, &code); err != nil {
					return nil, fmt.Errorf("failed to unmarshal medium: %w", err)
				}
				mediumStr, err := mediumCodeToString(code)
				if err != nil {
					return nil, err
				}
				mediumType := components.RVDirectiveMedium(mediumStr)
				item.Medium = &mediumType

			case protocol.RVDevPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal device_port: %w", err)
				}
				portStr := strconv.FormatUint(uint64(port), 10)
				item.DevicePort = &portStr

			case protocol.RVOwnerPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal owner_port: %w", err)
				}
				portStr := strconv.FormatUint(uint64(port), 10)
				item.OwnerPort = &portStr

			case protocol.RVWifiSsid:
				var ssid string
				if err := cbor.Unmarshal(instr.Value, &ssid); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_ssid: %w", err)
				}
				item.WifiSsid = &ssid

			case protocol.RVWifiPw:
				var pw string
				if err := cbor.Unmarshal(instr.Value, &pw); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_pw: %w", err)
				}
				item.WifiPw = &pw

			case protocol.RVDevOnly:
				devOnly := true
				item.DevOnly = &devOnly

			case protocol.RVOwnerOnly:
				ownerOnly := true
				item.OwnerOnly = &ownerOnly

			case protocol.RVBypass:
				rvBypass := true
				item.RvBypass = &rvBypass

			case protocol.RVDelaysec:
				var secs uint32
				if err := cbor.Unmarshal(instr.Value, &secs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal delay_seconds: %w", err)
				}
				item.DelaySeconds = &secs

			case protocol.RVSvCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal sv_cert_hash: %w", err)
				}
				hashStr := hex.EncodeToString(hash)
				item.SvCertHash = &hashStr

			case protocol.RVClCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal cl_cert_hash: %w", err)
				}
				hashStr := hex.EncodeToString(hash)
				item.ClCertHash = &hashStr

			case protocol.RVUserInput:
				// V1 format stores user_input as string "true"
				userInput := "true"
				item.UserInput = &userInput

			case protocol.RVExtRV:
				var extrv []string
				if err := cbor.Unmarshal(instr.Value, &extrv); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ext_rv: %w", err)
				}
				// V1 format stores ext_rv as JSON array string
				extrvJSON, err := json.Marshal(extrv)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal ext_rv to JSON: %w", err)
				}
				extrvStr := string(extrvJSON)
				item.ExtRv = &extrvStr
			}
		}

		out = append(out, item)
	}

	return out, nil
}

// protocolStringToCode converts V1 protocol string to FDO protocol code
// V1 API supports: rest, http, https, tcp, tls, coap+tcp, coap
func protocolStringToCode(s string) (uint8, error) {
	switch s {
	case "rest":
		return protocol.RVProtRest, nil
	case "http":
		return protocol.RVProtHTTP, nil
	case "https":
		return protocol.RVProtHTTPS, nil
	case "tcp":
		return protocol.RVProtTCP, nil
	case "tls":
		return protocol.RVProtTLS, nil
	case "coap+tcp":
		return protocol.RVProtCoapTCP, nil
	case "coap":
		return protocol.RVProtCoapUDP, nil
	default:
		return 0, fmt.Errorf("unknown protocol: %q", s)
	}
}

// protocolCodeToString converts FDO protocol code to V1 protocol string
func protocolCodeToString(code uint8) (string, error) {
	switch code {
	case protocol.RVProtRest:
		return "rest", nil
	case protocol.RVProtHTTP:
		return "http", nil
	case protocol.RVProtHTTPS:
		return "https", nil
	case protocol.RVProtTCP:
		return "tcp", nil
	case protocol.RVProtTLS:
		return "tls", nil
	case protocol.RVProtCoapTCP:
		return "coap+tcp", nil
	case protocol.RVProtCoapUDP:
		return "coap", nil
	default:
		return "", fmt.Errorf("unknown protocol code: %d", code)
	}
}

// mediumStringToCode converts V1 medium string to FDO medium code
// V1 API only supports: eth_all, wifi_all
func mediumStringToCode(s string) (uint8, error) {
	switch s {
	case "eth_all":
		return protocol.RVMedEthAll, nil
	case "wifi_all":
		return protocol.RVMedWifiAll, nil
	default:
		return 0, fmt.Errorf("unknown medium: %q (V1 API only supports eth_all, wifi_all)", s)
	}
}

// mediumCodeToString converts FDO medium code to V1 medium string
func mediumCodeToString(code uint8) (string, error) {
	switch code {
	case protocol.RVMedEthAll:
		return "eth_all", nil
	case protocol.RVMedWifiAll:
		return "wifi_all", nil
	default:
		return "", fmt.Errorf("unsupported medium code for V1 API: %d (only eth_all=20, wifi_all=21 supported)", code)
	}
}

// parsePortString parses a port string into uint16
// V1 API uses string format for ports (e.g., "8080")
func parsePortString(s string) (uint16, error) {
	port, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", s, err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return uint16(port), nil
}
