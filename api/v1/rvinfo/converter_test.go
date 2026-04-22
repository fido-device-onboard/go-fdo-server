// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"math"
	"testing"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

// helper to create a pointer to a value
func ptr[T any](v T) *T {
	return &v
}

// TestProtocolStringToCode tests protocolStringToCode for all known protocols and error cases.
func TestProtocolStringToCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint8
		wantErr bool
	}{
		{"rest", "rest", protocol.RVProtRest, false},
		{"http", "http", protocol.RVProtHTTP, false},
		{"https", "https", protocol.RVProtHTTPS, false},
		{"tcp", "tcp", protocol.RVProtTCP, false},
		{"tls", "tls", protocol.RVProtTLS, false},
		{"coap+tcp", "coap+tcp", protocol.RVProtCoapTCP, false},
		{"coap", "coap", protocol.RVProtCoapUDP, false},
		{"unknown", "grpc", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protocolStringToCode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("protocolStringToCode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("protocolStringToCode(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestProtocolCodeToString tests protocolCodeToString for all known codes and error cases.
func TestProtocolCodeToString(t *testing.T) {
	tests := []struct {
		name    string
		code    uint8
		want    string
		wantErr bool
	}{
		{"rest", protocol.RVProtRest, "rest", false},
		{"http", protocol.RVProtHTTP, "http", false},
		{"https", protocol.RVProtHTTPS, "https", false},
		{"tcp", protocol.RVProtTCP, "tcp", false},
		{"tls", protocol.RVProtTLS, "tls", false},
		{"coap+tcp", protocol.RVProtCoapTCP, "coap+tcp", false},
		{"coap", protocol.RVProtCoapUDP, "coap", false},
		{"unknown code 255", 255, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protocolCodeToString(tt.code)
			if (err != nil) != tt.wantErr {
				t.Fatalf("protocolCodeToString(%d) error = %v, wantErr %v", tt.code, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("protocolCodeToString(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

// TestProtocolRoundTrip verifies string->code->string round-trips for all protocols.
func TestProtocolRoundTrip(t *testing.T) {
	protocols := []string{"rest", "http", "https", "tcp", "tls", "coap+tcp", "coap"}
	for _, p := range protocols {
		t.Run(p, func(t *testing.T) {
			code, err := protocolStringToCode(p)
			if err != nil {
				t.Fatalf("protocolStringToCode(%q) unexpected error: %v", p, err)
			}
			back, err := protocolCodeToString(code)
			if err != nil {
				t.Fatalf("protocolCodeToString(%d) unexpected error: %v", code, err)
			}
			if back != p {
				t.Fatalf("round-trip failed: %q -> %d -> %q", p, code, back)
			}
		})
	}
}

// TestMediumStringToCode tests mediumStringToCode for known mediums and error cases.
func TestMediumStringToCode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint8
		wantErr bool
	}{
		{"eth_all", "eth_all", protocol.RVMedEthAll, false},
		{"wifi_all", "wifi_all", protocol.RVMedWifiAll, false},
		{"unknown", "bluetooth", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mediumStringToCode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mediumStringToCode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("mediumStringToCode(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestMediumCodeToString tests mediumCodeToString for known codes and error cases.
func TestMediumCodeToString(t *testing.T) {
	tests := []struct {
		name    string
		code    uint8
		want    string
		wantErr bool
	}{
		{"eth_all", protocol.RVMedEthAll, "eth_all", false},
		{"wifi_all", protocol.RVMedWifiAll, "wifi_all", false},
		{"unknown code 99", 99, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mediumCodeToString(tt.code)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mediumCodeToString(%d) error = %v, wantErr %v", tt.code, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("mediumCodeToString(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

// TestMediumRoundTrip verifies string->code->string round-trips for all mediums.
func TestMediumRoundTrip(t *testing.T) {
	mediums := []string{"eth_all", "wifi_all"}
	for _, m := range mediums {
		t.Run(m, func(t *testing.T) {
			code, err := mediumStringToCode(m)
			if err != nil {
				t.Fatalf("mediumStringToCode(%q) unexpected error: %v", m, err)
			}
			back, err := mediumCodeToString(code)
			if err != nil {
				t.Fatalf("mediumCodeToString(%d) unexpected error: %v", code, err)
			}
			if back != m {
				t.Fatalf("round-trip failed: %q -> %d -> %q", m, code, back)
			}
		})
	}
}

// TestParsePortString tests parsePortString for valid ports and edge/error cases.
func TestParsePortString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint16
		wantErr bool
	}{
		{"valid port 80", "80", 80, false},
		{"valid port 443", "443", 443, false},
		{"valid port 8080", "8080", 8080, false},
		{"valid port 8443", "8443", 8443, false},
		{"valid port 1", "1", 1, false},
		{"valid port 65535", "65535", 65535, false},
		{"port 0", "0", 0, true},
		{"negative port", "-1", 0, true},
		{"port above 65535", "65536", 0, true},
		{"non-numeric", "abc", 0, true},
		{"empty string", "", 0, true},
		{"decimal", "80.5", 0, true},
		{"leading space", " 80", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePortString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePortString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parsePortString(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestRendezvousInfoToProtocol_MinimalDNS tests conversion with only DNS set.
func TestRendezvousInfoToProtocol_MinimalDNS(t *testing.T) {
	rvInfo := RendezvousInfo{
		{Dns: ptr("example.com")},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 directive group, got %d", len(result))
	}
	if len(result[0]) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(result[0]))
	}
	if result[0][0].Variable != protocol.RVDns {
		t.Fatalf("expected RVDns variable, got %d", result[0][0].Variable)
	}
}

// TestRendezvousInfoToProtocol_MinimalIP tests conversion with only IP set.
func TestRendezvousInfoToProtocol_MinimalIP(t *testing.T) {
	rvInfo := RendezvousInfo{
		{Ip: ptr("192.168.1.1")},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 directive group, got %d", len(result))
	}
	if result[0][0].Variable != protocol.RVIPAddress {
		t.Fatalf("expected RVIPAddress variable, got %d", result[0][0].Variable)
	}
}

// TestRendezvousInfoToProtocol_MissingDNSAndIP tests that missing both DNS and IP returns error.
func TestRendezvousInfoToProtocol_MissingDNSAndIP(t *testing.T) {
	rvInfo := RendezvousInfo{
		{Protocol: ptr(RVProtocol("https"))},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error when both dns and ip are missing")
	}
}

// TestRendezvousInfoToProtocol_InvalidIP tests that an invalid IP returns error.
func TestRendezvousInfoToProtocol_InvalidIP(t *testing.T) {
	rvInfo := RendezvousInfo{
		{Ip: ptr("not-an-ip")},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// TestRendezvousInfoToProtocol_InvalidProtocol tests that an invalid protocol returns error.
func TestRendezvousInfoToProtocol_InvalidProtocol(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:      ptr("example.com"),
			Protocol: ptr(RVProtocol("grpc")),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

// TestRendezvousInfoToProtocol_InvalidMedium tests that an invalid medium returns error.
func TestRendezvousInfoToProtocol_InvalidMedium(t *testing.T) {
	medium := RendezvousDirectiveMedium("bluetooth")
	rvInfo := RendezvousInfo{
		{
			Dns:    ptr("example.com"),
			Medium: &medium,
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid medium")
	}
}

// TestRendezvousInfoToProtocol_InvalidDevicePort tests error for invalid device port.
func TestRendezvousInfoToProtocol_InvalidDevicePort(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:        ptr("example.com"),
			DevicePort: ptr("0"),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid device port")
	}
}

// TestRendezvousInfoToProtocol_InvalidOwnerPort tests error for invalid owner port.
func TestRendezvousInfoToProtocol_InvalidOwnerPort(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			OwnerPort: ptr("99999"),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid owner port")
	}
}

// TestRendezvousInfoToProtocol_InvalidSvCertHash tests error for non-hex sv_cert_hash.
func TestRendezvousInfoToProtocol_InvalidSvCertHash(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:        ptr("example.com"),
			SvCertHash: ptr("not-hex"),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid sv_cert_hash hex")
	}
}

// TestRendezvousInfoToProtocol_InvalidClCertHash tests error for non-hex cl_cert_hash.
func TestRendezvousInfoToProtocol_InvalidClCertHash(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:        ptr("example.com"),
			ClCertHash: ptr("zzzz"),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid cl_cert_hash hex")
	}
}

// TestRendezvousInfoToProtocol_InvalidExtRvJSON tests error for invalid ext_rv JSON.
func TestRendezvousInfoToProtocol_InvalidExtRvJSON(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:   ptr("example.com"),
			ExtRv: ptr("not valid json"),
		},
	}
	_, err := RendezvousInfoToProtocol(rvInfo)
	if err == nil {
		t.Fatal("expected error for invalid ext_rv JSON")
	}
}

// TestRendezvousInfoToProtocol_EmptyInput tests that empty input produces empty output.
func TestRendezvousInfoToProtocol_EmptyInput(t *testing.T) {
	result, err := RendezvousInfoToProtocol(RendezvousInfo{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 directive groups, got %d", len(result))
	}
}

// TestRendezvousInfoToProtocol_BooleanFlags tests that boolean flags are only included when true.
func TestRendezvousInfoToProtocol_BooleanFlags(t *testing.T) {
	// All flags true
	rvInfo := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			DevOnly:   ptr(true),
			OwnerOnly: ptr(true),
			RvBypass:  ptr(true),
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DNS + 3 boolean flags = 4 instructions
	if len(result[0]) != 4 {
		t.Fatalf("expected 4 instructions with all flags true, got %d", len(result[0]))
	}

	// All flags false - should not be included
	rvInfoFalse := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			DevOnly:   ptr(false),
			OwnerOnly: ptr(false),
			RvBypass:  ptr(false),
		},
	}
	resultFalse, err := RendezvousInfoToProtocol(rvInfoFalse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only DNS instruction when all flags are false
	if len(resultFalse[0]) != 1 {
		t.Fatalf("expected 1 instruction with all flags false, got %d", len(resultFalse[0]))
	}
}

// TestRendezvousInfoToProtocol_UserInput tests user_input field handling.
func TestRendezvousInfoToProtocol_UserInput(t *testing.T) {
	// Non-empty user_input should produce an instruction
	rvInfo := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			UserInput: ptr("true"),
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0]) != 2 {
		t.Fatalf("expected 2 instructions (dns + user_input), got %d", len(result[0]))
	}

	// Empty user_input should not produce an instruction
	rvInfoEmpty := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			UserInput: ptr(""),
		},
	}
	resultEmpty, err := RendezvousInfoToProtocol(rvInfoEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resultEmpty[0]) != 1 {
		t.Fatalf("expected 1 instruction (dns only), got %d", len(resultEmpty[0]))
	}
}

// TestRendezvousInfoToProtocol_WiFiFields tests wifi_ssid and wifi_pw fields.
func TestRendezvousInfoToProtocol_WiFiFields(t *testing.T) {
	rvInfo := RendezvousInfo{
		{
			Dns:      ptr("example.com"),
			WifiSsid: ptr("MyNetwork"),
			WifiPw:   ptr("secret123"),
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DNS + wifi_ssid + wifi_pw = 3
	if len(result[0]) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(result[0]))
	}

	foundSsid := false
	foundPw := false
	for _, instr := range result[0] {
		if instr.Variable == protocol.RVWifiSsid {
			foundSsid = true
		}
		if instr.Variable == protocol.RVWifiPw {
			foundPw = true
		}
	}
	if !foundSsid {
		t.Fatal("expected RVWifiSsid instruction")
	}
	if !foundPw {
		t.Fatal("expected RVWifiPw instruction")
	}
}

// TestRendezvousInfoToProtocol_DelaySeconds tests delay_seconds field.
func TestRendezvousInfoToProtocol_DelaySeconds(t *testing.T) {
	delay := uint32(30)
	rvInfo := RendezvousInfo{
		{
			Dns:          ptr("example.com"),
			DelaySeconds: &delay,
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0]) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(result[0]))
	}

	found := false
	for _, instr := range result[0] {
		if instr.Variable == protocol.RVDelaysec {
			found = true
		}
	}
	if !found {
		t.Fatal("expected RVDelaysec instruction")
	}
}

// TestRendezvousInfoToProtocol_DelaySecondsMaxUint32 tests delay_seconds at max uint32 boundary.
func TestRendezvousInfoToProtocol_DelaySecondsMaxUint32(t *testing.T) {
	delay := uint32(math.MaxUint32)
	rvInfo := RendezvousInfo{
		{
			Dns:          ptr("example.com"),
			DelaySeconds: &delay,
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0]) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(result[0]))
	}
}

// TestRendezvousInfoToProtocol_ValidHashes tests sv_cert_hash and cl_cert_hash with valid hex.
func TestRendezvousInfoToProtocol_ValidHashes(t *testing.T) {
	hexHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	rvInfo := RendezvousInfo{
		{
			Dns:        ptr("example.com"),
			SvCertHash: &hexHash,
			ClCertHash: &hexHash,
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DNS + sv_cert_hash + cl_cert_hash = 3
	if len(result[0]) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(result[0]))
	}
}

// TestRendezvousInfoToProtocol_ExtRvValid tests ext_rv with valid JSON array.
func TestRendezvousInfoToProtocol_ExtRvValid(t *testing.T) {
	extRv := `["value1","value2"]`
	rvInfo := RendezvousInfo{
		{
			Dns:   ptr("example.com"),
			ExtRv: &extRv,
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0]) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(result[0]))
	}
}

// TestRendezvousInfoToProtocol_AllFields tests conversion with all fields populated.
func TestRendezvousInfoToProtocol_AllFields(t *testing.T) {
	hexHash := "abcd1234"
	delay := uint32(10)
	medium := RendezvousDirectiveMedium("wifi_all")
	extRv := `["ext1"]`

	rvInfo := RendezvousInfo{
		{
			Dns:          ptr("rv.example.com"),
			Ip:           ptr("10.0.0.1"),
			Protocol:     ptr(RVProtocol("https")),
			Medium:       &medium,
			DevicePort:   ptr("8080"),
			OwnerPort:    ptr("8443"),
			WifiSsid:     ptr("TestSSID"),
			WifiPw:       ptr("TestPW"),
			DevOnly:      ptr(true),
			OwnerOnly:    ptr(false),
			RvBypass:     ptr(true),
			DelaySeconds: &delay,
			SvCertHash:   &hexHash,
			ClCertHash:   &hexHash,
			UserInput:    ptr("true"),
			ExtRv:        &extRv,
		},
	}
	result, err := RendezvousInfoToProtocol(rvInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 directive group, got %d", len(result))
	}

	// Count expected instructions:
	// dns, ip, protocol, medium, device_port, owner_port,
	// wifi_ssid, wifi_pw, dev_only(true), rv_bypass(true),
	// delay_seconds, sv_cert_hash, cl_cert_hash, user_input, ext_rv = 15
	// owner_only is false so not included
	expected := 15
	if len(result[0]) != expected {
		t.Fatalf("expected %d instructions, got %d", expected, len(result[0]))
	}
}

// TestRendezvousInfoRoundTrip tests that ToProtocol followed by FromProtocol preserves data.
func TestRendezvousInfoRoundTrip(t *testing.T) {
	hexHash := "abcdef01"
	delay := uint32(42)
	medium := RendezvousDirectiveMedium("eth_all")
	extRv := `["ext1","ext2"]`

	original := RendezvousInfo{
		{
			Dns:          ptr("rv.example.com"),
			Ip:           ptr("192.168.1.100"),
			Protocol:     ptr(RVProtocol("https")),
			Medium:       &medium,
			DevicePort:   ptr("8080"),
			OwnerPort:    ptr("8443"),
			WifiSsid:     ptr("TestNet"),
			WifiPw:       ptr("pass123"),
			DevOnly:      ptr(true),
			RvBypass:     ptr(true),
			DelaySeconds: &delay,
			SvCertHash:   &hexHash,
			ClCertHash:   &hexHash,
			UserInput:    ptr("true"),
			ExtRv:        &extRv,
		},
	}

	proto, err := RendezvousInfoToProtocol(original)
	if err != nil {
		t.Fatalf("ToProtocol error: %v", err)
	}

	roundTripped, err := RendezvousInfoFromProtocol(proto)
	if err != nil {
		t.Fatalf("FromProtocol error: %v", err)
	}

	if len(roundTripped) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(roundTripped))
	}

	rt := roundTripped[0]

	// Check DNS
	if rt.Dns == nil || *rt.Dns != "rv.example.com" {
		t.Fatalf("dns mismatch: got %v", rt.Dns)
	}
	// Check IP (net.IP may normalize)
	if rt.Ip == nil || *rt.Ip != "192.168.1.100" {
		t.Fatalf("ip mismatch: got %v", rt.Ip)
	}
	// Check protocol
	if rt.Protocol == nil || string(*rt.Protocol) != "https" {
		t.Fatalf("protocol mismatch: got %v", rt.Protocol)
	}
	// Check medium
	if rt.Medium == nil || string(*rt.Medium) != "eth_all" {
		t.Fatalf("medium mismatch: got %v", rt.Medium)
	}
	// Check ports
	if rt.DevicePort == nil || *rt.DevicePort != "8080" {
		t.Fatalf("device_port mismatch: got %v", rt.DevicePort)
	}
	if rt.OwnerPort == nil || *rt.OwnerPort != "8443" {
		t.Fatalf("owner_port mismatch: got %v", rt.OwnerPort)
	}
	// Check wifi
	if rt.WifiSsid == nil || *rt.WifiSsid != "TestNet" {
		t.Fatalf("wifi_ssid mismatch: got %v", rt.WifiSsid)
	}
	if rt.WifiPw == nil || *rt.WifiPw != "pass123" {
		t.Fatalf("wifi_pw mismatch: got %v", rt.WifiPw)
	}
	// Check boolean flags
	if rt.DevOnly == nil || !*rt.DevOnly {
		t.Fatalf("dev_only mismatch: got %v", rt.DevOnly)
	}
	if rt.RvBypass == nil || !*rt.RvBypass {
		t.Fatalf("rv_bypass mismatch: got %v", rt.RvBypass)
	}
	// Check delay
	if rt.DelaySeconds == nil || *rt.DelaySeconds != 42 {
		t.Fatalf("delay_seconds mismatch: got %v", rt.DelaySeconds)
	}
	// Check hashes
	if rt.SvCertHash == nil || *rt.SvCertHash != hexHash {
		t.Fatalf("sv_cert_hash mismatch: got %v", rt.SvCertHash)
	}
	if rt.ClCertHash == nil || *rt.ClCertHash != hexHash {
		t.Fatalf("cl_cert_hash mismatch: got %v", rt.ClCertHash)
	}
	// Check user_input
	if rt.UserInput == nil || *rt.UserInput != "true" {
		t.Fatalf("user_input mismatch: got %v", rt.UserInput)
	}
	// Check ext_rv
	if rt.ExtRv == nil || *rt.ExtRv != extRv {
		t.Fatalf("ext_rv mismatch: got %v", rt.ExtRv)
	}
}

// TestRendezvousInfoRoundTrip_DNSOnly tests round-trip with DNS only.
func TestRendezvousInfoRoundTrip_DNSOnly(t *testing.T) {
	original := RendezvousInfo{
		{Dns: ptr("simple.example.com")},
	}

	proto, err := RendezvousInfoToProtocol(original)
	if err != nil {
		t.Fatalf("ToProtocol error: %v", err)
	}

	roundTripped, err := RendezvousInfoFromProtocol(proto)
	if err != nil {
		t.Fatalf("FromProtocol error: %v", err)
	}

	if len(roundTripped) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(roundTripped))
	}
	if roundTripped[0].Dns == nil || *roundTripped[0].Dns != "simple.example.com" {
		t.Fatalf("dns mismatch")
	}
}

// TestRendezvousInfoRoundTrip_MultipleDirectives tests round-trip with multiple directives.
func TestRendezvousInfoRoundTrip_MultipleDirectives(t *testing.T) {
	original := RendezvousInfo{
		{
			Dns:      ptr("primary.example.com"),
			Protocol: ptr(RVProtocol("https")),
		},
		{
			Dns:      ptr("fallback.example.com"),
			Protocol: ptr(RVProtocol("http")),
		},
	}

	proto, err := RendezvousInfoToProtocol(original)
	if err != nil {
		t.Fatalf("ToProtocol error: %v", err)
	}
	if len(proto) != 2 {
		t.Fatalf("expected 2 protocol groups, got %d", len(proto))
	}

	roundTripped, err := RendezvousInfoFromProtocol(proto)
	if err != nil {
		t.Fatalf("FromProtocol error: %v", err)
	}
	if len(roundTripped) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(roundTripped))
	}

	if *roundTripped[0].Dns != "primary.example.com" {
		t.Fatalf("first dns mismatch")
	}
	if *roundTripped[1].Dns != "fallback.example.com" {
		t.Fatalf("second dns mismatch")
	}
}

// TestRendezvousInfoRoundTrip_AllProtocols tests round-trip for every protocol type.
func TestRendezvousInfoRoundTrip_AllProtocols(t *testing.T) {
	protocols := []string{"rest", "http", "https", "tcp", "tls", "coap+tcp", "coap"}
	for _, p := range protocols {
		t.Run(p, func(t *testing.T) {
			original := RendezvousInfo{
				{
					Dns:      ptr("example.com"),
					Protocol: ptr(RVProtocol(p)),
				},
			}

			proto, err := RendezvousInfoToProtocol(original)
			if err != nil {
				t.Fatalf("ToProtocol error: %v", err)
			}

			roundTripped, err := RendezvousInfoFromProtocol(proto)
			if err != nil {
				t.Fatalf("FromProtocol error: %v", err)
			}

			if roundTripped[0].Protocol == nil || string(*roundTripped[0].Protocol) != p {
				t.Fatalf("protocol mismatch: expected %q, got %v", p, roundTripped[0].Protocol)
			}
		})
	}
}

// TestRendezvousInfoFromProtocol_EmptyInput tests that empty protocol input produces empty output.
func TestRendezvousInfoFromProtocol_EmptyInput(t *testing.T) {
	result, err := RendezvousInfoFromProtocol([][]protocol.RvInstruction{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 directives, got %d", len(result))
	}
}

// TestRendezvousInfoFromProtocol_NilInput tests that nil protocol input produces empty output.
func TestRendezvousInfoFromProtocol_NilInput(t *testing.T) {
	result, err := RendezvousInfoFromProtocol(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 directives, got %d", len(result))
	}
}

// TestConverterRoundTrip_OwnerOnlyFlag tests that owner_only=true round-trips correctly.
func TestConverterRoundTrip_OwnerOnlyFlag(t *testing.T) {
	original := RendezvousInfo{
		{
			Dns:       ptr("example.com"),
			OwnerOnly: ptr(true),
		},
	}

	proto, err := RendezvousInfoToProtocol(original)
	if err != nil {
		t.Fatalf("ToProtocol error: %v", err)
	}

	roundTripped, err := RendezvousInfoFromProtocol(proto)
	if err != nil {
		t.Fatalf("FromProtocol error: %v", err)
	}

	if roundTripped[0].OwnerOnly == nil || !*roundTripped[0].OwnerOnly {
		t.Fatal("owner_only should be true after round-trip")
	}
}
