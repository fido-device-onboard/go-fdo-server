// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"encoding/hex"
	"fmt"
	"net"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
)

// RVInfoV2ToProtocol converts V2 API RVInfo (nested instructions) to protocol format
//
// Expected V2 format (array of arrays of single-key objects):
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
func RVInfoV2ToProtocol(rvInfo components.RVInfo) ([][]protocol.RvInstruction, error) {
	out := make([][]protocol.RvInstruction, 0, len(rvInfo))

	for directiveIdx, instructions := range rvInfo {
		group := make([]protocol.RvInstruction, 0, len(instructions))
		hasDNSorIP := false

		for instrIdx, instruction := range instructions {
			// Each instruction in V2 is represented as RVInstruction_Item union type
			// We need to try to unmarshal it as each possible type to determine what it is

			// Try DNS
			if dns, err := instruction.AsDNS(); err == nil && dns.Dns != nil {
				enc, err := cbor.Marshal(*dns.Dns)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode dns: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVDns, Value: enc})
				hasDNSorIP = true
				continue
			}

			// Try IP
			if ipInstr, err := instruction.AsIP(); err == nil && ipInstr.Ip != nil {
				ip := net.ParseIP(*ipInstr.Ip)
				if ip == nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: invalid ip %q", directiveIdx, instrIdx, *ipInstr.Ip)
				}
				enc, err := cbor.Marshal(ip)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode ip: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc})
				hasDNSorIP = true
				continue
			}

			// Try Protocol
			if protoInstr, err := instruction.AsProtocol(); err == nil && protoInstr.Protocol != nil {
				code, err := protocolStringToCode(string(*protoInstr.Protocol))
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: %w", directiveIdx, instrIdx, err)
				}
				enc, err := cbor.Marshal(code)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode protocol: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc})
				continue
			}

			// Try Medium
			if mediumInstr, err := instruction.AsMedium(); err == nil && mediumInstr.Medium != nil {
				code, err := mediumStringToCode(string(*mediumInstr.Medium))
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: %w", directiveIdx, instrIdx, err)
				}
				enc, err := cbor.Marshal(code)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode medium: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc})
				continue
			}

			// Try DevicePort
			if portInstr, err := instruction.AsDevicePort(); err == nil && portInstr.DevicePort != nil {
				port := validatePortNumber(*portInstr.DevicePort)
				if port == 0 {
					return nil, fmt.Errorf("rvinfo[%d][%d]: invalid device_port %d", directiveIdx, instrIdx, *portInstr.DevicePort)
				}
				enc, err := cbor.Marshal(uint16(port))
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode device_port: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc})
				continue
			}

			// Try OwnerPort
			if portInstr, err := instruction.AsOwnerPort(); err == nil && portInstr.OwnerPort != nil {
				port := validatePortNumber(*portInstr.OwnerPort)
				if port == 0 {
					return nil, fmt.Errorf("rvinfo[%d][%d]: invalid owner_port %d", directiveIdx, instrIdx, *portInstr.OwnerPort)
				}
				enc, err := cbor.Marshal(uint16(port))
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode owner_port: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc})
				continue
			}

			// Try WifiSSID
			if ssidInstr, err := instruction.AsWifiSSID(); err == nil && ssidInstr.WifiSsid != nil {
				enc, err := cbor.Marshal(*ssidInstr.WifiSsid)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode wifi_ssid: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc})
				continue
			}

			// Try WifiPW
			if pwInstr, err := instruction.AsWifiPW(); err == nil && pwInstr.WifiPw != nil {
				enc, err := cbor.Marshal(*pwInstr.WifiPw)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode wifi_pw: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc})
				continue
			}

			// Try DevOnly
			if devOnlyInstr, err := instruction.AsDevOnly(); err == nil && devOnlyInstr.DevOnly != nil && *devOnlyInstr.DevOnly {
				group = append(group, protocol.RvInstruction{Variable: protocol.RVDevOnly})
				continue
			}

			// Try OwnerOnly
			if ownerOnlyInstr, err := instruction.AsOwnerOnly(); err == nil && ownerOnlyInstr.OwnerOnly != nil && *ownerOnlyInstr.OwnerOnly {
				group = append(group, protocol.RvInstruction{Variable: protocol.RVOwnerOnly})
				continue
			}

			// Try RVBypass
			if bypassInstr, err := instruction.AsRVBypass(); err == nil && bypassInstr.RvBypass != nil && *bypassInstr.RvBypass {
				group = append(group, protocol.RvInstruction{Variable: protocol.RVBypass})
				continue
			}

			// Try DelaySeconds
			if delayInstr, err := instruction.AsDelaySeconds(); err == nil && delayInstr.DelaySeconds != nil {
				secs := uint32(*delayInstr.DelaySeconds)
				if *delayInstr.DelaySeconds < 0 {
					return nil, fmt.Errorf("rvinfo[%d][%d]: delay_seconds must be non-negative", directiveIdx, instrIdx)
				}
				enc, err := cbor.Marshal(secs)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode delay_seconds: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc})
				continue
			}

			// Try SvCertHash
			if hashInstr, err := instruction.AsSvCertHash(); err == nil && hashInstr.SvCertHash != nil {
				hash, err := hex.DecodeString(*hashInstr.SvCertHash)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: sv_cert_hash: %w", directiveIdx, instrIdx, err)
				}
				enc, err := cbor.Marshal(hash)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode sv_cert_hash: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc})
				continue
			}

			// Try ClCertHash
			if hashInstr, err := instruction.AsClCertHash(); err == nil && hashInstr.ClCertHash != nil {
				hash, err := hex.DecodeString(*hashInstr.ClCertHash)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: cl_cert_hash: %w", directiveIdx, instrIdx, err)
				}
				enc, err := cbor.Marshal(hash)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode cl_cert_hash: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc})
				continue
			}

			// Try UserInput
			if userInputInstr, err := instruction.AsUserInput(); err == nil && userInputInstr.UserInput != nil && *userInputInstr.UserInput {
				group = append(group, protocol.RvInstruction{Variable: protocol.RVUserInput})
				continue
			}

			// Try ExtRV
			if extrvInstr, err := instruction.AsExtRV(); err == nil && extrvInstr.ExtRv != nil {
				enc, err := cbor.Marshal(*extrvInstr.ExtRv)
				if err != nil {
					return nil, fmt.Errorf("rvinfo[%d][%d]: failed to encode ext_rv: %w", directiveIdx, instrIdx, err)
				}
				group = append(group, protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc})
				continue
			}

			// If we get here, we couldn't parse the instruction
			return nil, fmt.Errorf("rvinfo[%d][%d]: unknown or invalid instruction type", directiveIdx, instrIdx)
		}

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if !hasDNSorIP {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", directiveIdx)
		}

		out = append(out, group)
	}

	return out, nil
}

// RVInfoV2FromProtocol converts protocol format back to V2 API RVInfo (nested instructions)
//
// Output format (array of arrays of single-key objects):
// [[{"dns":"host"},{"protocol":"http"},{"owner_port":8080}]]
func RVInfoV2FromProtocol(proto [][]protocol.RvInstruction) (components.RVInfo, error) {
	out := make(components.RVInfo, 0, len(proto))

	for _, directive := range proto {
		group := make(components.RVInstruction, 0, len(directive))

		for _, instr := range directive {
			var item components.RVInstruction_Item

			switch instr.Variable {
			case protocol.RVDns:
				var dns string
				if err := cbor.Unmarshal(instr.Value, &dns); err != nil {
					return nil, fmt.Errorf("failed to unmarshal dns: %w", err)
				}
				if err := item.FromDNS(components.DNS{Dns: &dns}); err != nil {
					return nil, err
				}

			case protocol.RVIPAddress:
				var ip net.IP
				if err := cbor.Unmarshal(instr.Value, &ip); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ip: %w", err)
				}
				ipStr := ip.String()
				if err := item.FromIP(components.IP{Ip: &ipStr}); err != nil {
					return nil, err
				}

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
				if err := item.FromProtocol(components.Protocol{Protocol: &protoType}); err != nil {
					return nil, err
				}

			case protocol.RVMedium:
				var code uint8
				if err := cbor.Unmarshal(instr.Value, &code); err != nil {
					return nil, fmt.Errorf("failed to unmarshal medium: %w", err)
				}
				mediumStr, err := mediumCodeToString(code)
				if err != nil {
					return nil, err
				}
				mediumType := components.MediumMedium(mediumStr)
				if err := item.FromMedium(components.Medium{Medium: &mediumType}); err != nil {
					return nil, err
				}

			case protocol.RVDevPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal device_port: %w", err)
				}
				portNum := int(port)
				if err := item.FromDevicePort(components.DevicePort{DevicePort: &portNum}); err != nil {
					return nil, err
				}

			case protocol.RVOwnerPort:
				var port uint16
				if err := cbor.Unmarshal(instr.Value, &port); err != nil {
					return nil, fmt.Errorf("failed to unmarshal owner_port: %w", err)
				}
				portNum := int(port)
				if err := item.FromOwnerPort(components.OwnerPort{OwnerPort: &portNum}); err != nil {
					return nil, err
				}

			case protocol.RVWifiSsid:
				var ssid string
				if err := cbor.Unmarshal(instr.Value, &ssid); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_ssid: %w", err)
				}
				if err := item.FromWifiSSID(components.WifiSSID{WifiSsid: &ssid}); err != nil {
					return nil, err
				}

			case protocol.RVWifiPw:
				var pw string
				if err := cbor.Unmarshal(instr.Value, &pw); err != nil {
					return nil, fmt.Errorf("failed to unmarshal wifi_pw: %w", err)
				}
				if err := item.FromWifiPW(components.WifiPW{WifiPw: &pw}); err != nil {
					return nil, err
				}

			case protocol.RVDevOnly:
				devOnly := true
				if err := item.FromDevOnly(components.DevOnly{DevOnly: &devOnly}); err != nil {
					return nil, err
				}

			case protocol.RVOwnerOnly:
				ownerOnly := true
				if err := item.FromOwnerOnly(components.OwnerOnly{OwnerOnly: &ownerOnly}); err != nil {
					return nil, err
				}

			case protocol.RVBypass:
				rvBypass := true
				if err := item.FromRVBypass(components.RVBypass{RvBypass: &rvBypass}); err != nil {
					return nil, err
				}

			case protocol.RVDelaysec:
				var secs uint32
				if err := cbor.Unmarshal(instr.Value, &secs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal delay_seconds: %w", err)
				}
				secsInt := int32(secs)
				if err := item.FromDelaySeconds(components.DelaySeconds{DelaySeconds: &secsInt}); err != nil {
					return nil, err
				}

			case protocol.RVSvCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal sv_cert_hash: %w", err)
				}
				hashStr := hex.EncodeToString(hash)
				if err := item.FromSvCertHash(components.SvCertHash{SvCertHash: &hashStr}); err != nil {
					return nil, err
				}

			case protocol.RVClCertHash:
				var hash []byte
				if err := cbor.Unmarshal(instr.Value, &hash); err != nil {
					return nil, fmt.Errorf("failed to unmarshal cl_cert_hash: %w", err)
				}
				hashStr := hex.EncodeToString(hash)
				if err := item.FromClCertHash(components.ClCertHash{ClCertHash: &hashStr}); err != nil {
					return nil, err
				}

			case protocol.RVUserInput:
				userInput := true
				if err := item.FromUserInput(components.UserInput{UserInput: &userInput}); err != nil {
					return nil, err
				}

			case protocol.RVExtRV:
				var extrv []string
				if err := cbor.Unmarshal(instr.Value, &extrv); err != nil {
					return nil, fmt.Errorf("failed to unmarshal ext_rv: %w", err)
				}
				if err := item.FromExtRV(components.ExtRV{ExtRv: &extrv}); err != nil {
					return nil, err
				}
			}

			group = append(group, item)
		}

		out = append(out, group)
	}

	return out, nil
}

// protocolStringToCode converts V2 protocol string to FDO protocol code
// V2 API supports: rest, http, https, tcp, tls, coap+tcp, coap
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

// protocolCodeToString converts FDO protocol code to V2 protocol string
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

// mediumStringToCode converts V2 medium string to FDO medium code
// V2 API supports: wifi_1-9, wifi_all, wired_1-9, wired_all
func mediumStringToCode(s string) (uint8, error) {
	switch s {
	case "wifi_all":
		return protocol.RVMedWifiAll, nil
	case "wired_all":
		return protocol.RVMedEthAll, nil
	// V2 API also supports specific interfaces - these map to FDO protocol codes
	case "wifi_1":
		return 0, nil // WiFi interface 1
	case "wifi_2":
		return 1, nil
	case "wifi_3":
		return 2, nil
	case "wifi_4":
		return 3, nil
	case "wifi_5":
		return 4, nil
	case "wifi_6":
		return 5, nil
	case "wifi_7":
		return 6, nil
	case "wifi_8":
		return 7, nil
	case "wifi_9":
		return 8, nil
	case "wired_1":
		return 10, nil // Wired interface 1
	case "wired_2":
		return 11, nil
	case "wired_3":
		return 12, nil
	case "wired_4":
		return 13, nil
	case "wired_5":
		return 14, nil
	case "wired_6":
		return 15, nil
	case "wired_7":
		return 16, nil
	case "wired_8":
		return 17, nil
	case "wired_9":
		return 18, nil
	default:
		return 0, fmt.Errorf("unknown medium: %q", s)
	}
}

// mediumCodeToString converts FDO medium code to V2 medium string
func mediumCodeToString(code uint8) (string, error) {
	switch code {
	case protocol.RVMedWifiAll:
		return "wifi_all", nil
	case protocol.RVMedEthAll:
		return "wired_all", nil
	// Specific interfaces
	case 0:
		return "wifi_1", nil
	case 1:
		return "wifi_2", nil
	case 2:
		return "wifi_3", nil
	case 3:
		return "wifi_4", nil
	case 4:
		return "wifi_5", nil
	case 5:
		return "wifi_6", nil
	case 6:
		return "wifi_7", nil
	case 7:
		return "wifi_8", nil
	case 8:
		return "wifi_9", nil
	case 10:
		return "wired_1", nil
	case 11:
		return "wired_2", nil
	case 12:
		return "wired_3", nil
	case 13:
		return "wired_4", nil
	case 14:
		return "wired_5", nil
	case 15:
		return "wired_6", nil
	case 16:
		return "wired_7", nil
	case 17:
		return "wired_8", nil
	case 18:
		return "wired_9", nil
	default:
		return "", fmt.Errorf("unknown medium code: %d", code)
	}
}

// validatePortNumber validates that a port number is in valid range (1-65535)
func validatePortNumber(port int) int {
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}
