package manufacturing

import (
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

func TestParseOpenAPIRvJSON_Valid(t *testing.T) {
	cases := []struct {
		name     string
		jsonBody string
		wantVars []protocol.RvVar // Expected variables in first directive
	}{
		{
			name: "simple_http_dns",
			jsonBody: `[
				[
					{"dns": "rendezvous.example.com"},
					{"protocol": "http"},
					{"owner_port": 8080}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVProtocol, protocol.RVOwnerPort},
		},
		{
			name: "ip_instead_of_dns",
			jsonBody: `[
				[
					{"ip": "192.168.1.100"},
					{"protocol": "https"},
					{"device_port": 8041},
					{"owner_port": 8443}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVIPAddress, protocol.RVProtocol, protocol.RVDevPort, protocol.RVOwnerPort},
		},
		{
			name: "with_rv_bypass",
			jsonBody: `[
				[
					{"dns": "owner.example.com"},
					{"protocol": "https"},
					{"owner_port": 8443},
					{"rv_bypass": true}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVProtocol, protocol.RVOwnerPort, protocol.RVBypass},
		},
		{
			name: "with_delay_seconds",
			jsonBody: `[
				[
					{"dns": "rv.example.com"},
					{"delay_seconds": 30}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVDelaysec},
		},
		{
			name: "multiple_directives",
			jsonBody: `[
				[
					{"dns": "rv-primary.example.com"},
					{"protocol": "https"},
					{"owner_port": 8443}
				],
				[
					{"dns": "rv-fallback.example.com"},
					{"protocol": "http"},
					{"owner_port": 8080}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVProtocol, protocol.RVOwnerPort},
		},
		{
			name: "with_wifi",
			jsonBody: `[
				[
					{"medium": "wifi_all"},
					{"wifi_ssid": "FDO-Network"},
					{"wifi_pw": "SecurePassword123"},
					{"dns": "rv.local.network"},
					{"protocol": "http"}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVMedium, protocol.RVWifiSsid, protocol.RVWifiPw, protocol.RVDns, protocol.RVProtocol},
		},
		{
			name: "with_user_input",
			jsonBody: `[
				[
					{"dns": "rv.example.com"},
					{"user_input": true}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVUserInput},
		},
		{
			name: "with_ext_rv",
			jsonBody: `[
				[
					{"dns": "rv.example.com"},
					{"ext_rv": ["custom-instruction-1", "custom-instruction-2"]}
				]
			]`,
			wantVars: []protocol.RvVar{protocol.RVDns, protocol.RVExtRV},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseOpenAPIRvJSON([]byte(tc.jsonBody))
			if err != nil {
				t.Fatalf("ParseOpenAPIRvJSON failed: %v", err)
			}

			if len(result) == 0 {
				t.Fatalf("expected at least one directive, got 0")
			}

			// Check first directive
			firstDirective := result[0]
			if len(firstDirective) != len(tc.wantVars) {
				t.Errorf("expected %d instructions, got %d", len(tc.wantVars), len(firstDirective))
			}

			// Verify all expected variables are present
			foundVars := make(map[protocol.RvVar]bool)
			for _, instr := range firstDirective {
				foundVars[instr.Variable] = true
			}

			for _, wantVar := range tc.wantVars {
				if !foundVars[wantVar] {
					t.Errorf("expected variable %d not found in result", wantVar)
				}
			}
		})
	}
}

func TestParseOpenAPIRvJSON_Invalid(t *testing.T) {
	cases := []struct {
		name      string
		jsonBody  string
		errSubstr string
	}{
		{
			name:      "not_array_of_arrays",
			jsonBody:  `[{"dns": "rv.com"}]`,
			errSubstr: "expected array of arrays",
		},
		{
			name: "multi_key_object",
			jsonBody: `[
				[
					{"dns": "rv.com", "protocol": "http"}
				]
			]`,
			errSubstr: "must be a single-key object",
		},
		{
			name: "missing_dns_and_ip",
			jsonBody: `[
				[
					{"protocol": "http"}
				]
			]`,
			errSubstr: "at least one of dns or ip must be specified",
		},
		{
			name: "unknown_key",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"invalid_key": "value"}
				]
			]`,
			errSubstr: "unknown instruction key",
		},
		{
			name: "invalid_protocol",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"protocol": "gopher"}
				]
			]`,
			errSubstr: "unsupported protocol",
		},
		{
			name: "invalid_port_type",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"owner_port": "not-a-number"}
				]
			]`,
			errSubstr: "port must be an integer",
		},
		{
			name: "invalid_user_input_type",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"user_input": "true"}
				]
			]`,
			errSubstr: "user_input must be a boolean",
		},
		{
			name: "invalid_ext_rv_type",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"ext_rv": "not-an-array"}
				]
			]`,
			errSubstr: "ext_rv must be an array",
		},
		{
			name: "invalid_ext_rv_element_type",
			jsonBody: `[
				[
					{"dns": "rv.com"},
					{"ext_rv": ["valid", 123]}
				]
			]`,
			errSubstr: "ext_rv[1] must be a string",
		},
		{
			name:      "malformed_json",
			jsonBody:  `[[[{bad}]]]`,
			errSubstr: "invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseOpenAPIRvJSON([]byte(tc.jsonBody))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Errorf("expected error containing %q, got: %v", tc.errSubstr, err)
			}
		})
	}
}

func TestParseOpenAPIRvJSON_ProtocolValues(t *testing.T) {
	// Test that protocol strings are converted to correct codes
	protocols := map[string]uint8{
		"http":     protocol.RVProtHTTP,
		"https":    protocol.RVProtHTTPS,
		"tcp":      protocol.RVProtTCP,
		"tls":      protocol.RVProtTLS,
		"coap+tcp": protocol.RVProtCoapTCP,
		"coap":     protocol.RVProtCoapUDP,
	}

	for protoStr, expectedCode := range protocols {
		t.Run("protocol_"+protoStr, func(t *testing.T) {
			jsonBody := `[[{"dns":"rv.com"}, {"protocol":"` + protoStr + `"}]]`
			result, err := ParseOpenAPIRvJSON([]byte(jsonBody))
			if err != nil {
				t.Fatalf("ParseOpenAPIRvJSON failed: %v", err)
			}

			// Find protocol instruction
			var found bool
			for _, instr := range result[0] {
				if instr.Variable == protocol.RVProtocol {
					found = true
					// Decode the CBOR value
					var code uint8
					if err := cbor.Unmarshal(instr.Value, &code); err != nil {
						t.Fatalf("failed to unmarshal protocol value: %v", err)
					}
					if code != expectedCode {
						t.Errorf("expected protocol code %d, got %d", expectedCode, code)
					}
					break
				}
			}
			if !found {
				t.Error("protocol instruction not found in result")
			}
		})
	}
}
