package db

import (
	"strings"
	"testing"
)

func TestParseHumanReadableRvJSON_Cases(t *testing.T) {
	cases := []struct {
		name      string
		jsonBody  string
		wantError bool
		errSubstr string
	}{
		{
			name:     "valid_dns_only",
			jsonBody: `[{"dns":"example.com","protocol":"http"}]`,
		},
		{
			name:     "valid_ip_only",
			jsonBody: `[{"ip":"127.0.0.1","protocol":"http"}]`,
		},
		{
			name:     "valid_both_dns_ip",
			jsonBody: `[{"dns":"example.com","ip":"127.0.0.1","protocol":"https"}]`,
		},
		{
			name:     "valid_with_ports_and_medium",
			jsonBody: `[{"dns":"example.com","device_port":"8080","owner_port":"8043","protocol":"http","medium":"eth_all"}]`,
		},
		{
			name:     "valid_with_delay_seconds",
			jsonBody: `[{"dns":"example.com","delay_seconds":10}]`,
		},
		{
			name:      "invalid_missing_dns_ip",
			jsonBody:  `[{}]`,
			wantError: true,
			errSubstr: "at least one of dns or ip",
		},
		{
			name:      "invalid_protocol",
			jsonBody:  `[{"dns":"example.com","protocol":"gopher"}]`,
			wantError: true,
			errSubstr: "unsupported protocol",
		},
		{
			name:      "invalid_device_port",
			jsonBody:  `[{"dns":"example.com","device_port":"eighty"}]`,
			wantError: true,
			errSubstr: "device_port",
		},
		{
			name:      "invalid_owner_port",
			jsonBody:  `[{"dns":"example.com","owner_port":"oops"}]`,
			wantError: true,
			errSubstr: "owner_port",
		},
		{
			name:      "invalid_delay_seconds_string",
			jsonBody:  `[{"dns":"example.com","delay_seconds":"10"}]`,
			wantError: true,
			errSubstr: "json: cannot unmarshal",
		},
		{
			name:      "invalid_delay_seconds_negative",
			jsonBody:  `[{"dns":"example.com","delay_seconds":-1}]`,
			wantError: true,
			errSubstr: "cannot unmarshal",
		},
		{
			name:      "invalid_delay_seconds_too_large",
			jsonBody:  `[{"dns":"example.com","delay_seconds":4294967296}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_medium",
			jsonBody:  `[{"dns":"example.com","medium":"wifi_invalid"}]`,
			wantError: true,
			errSubstr: "unsupported medium",
		},
		{
			name:      "invalid_medium_type_number",
			jsonBody:  `[{"dns":"example.com","medium":1}]`,
			wantError: true,
			errSubstr: "json: cannot unmarshal",
		},
		{
			name:      "invalid_protocol_case",
			jsonBody:  `[{"dns":"example.com","protocol":"HTTP"}]`,
			wantError: true,
			errSubstr: "unsupported protocol",
		},
		{
			name:      "invalid_protocol_type_number",
			jsonBody:  `[{"dns":"example.com","protocol":1}]`,
			wantError: true,
			errSubstr: "json: cannot unmarshal",
		},
		{
			name:      "invalid_sv_cert_hash_hex",
			jsonBody:  `[{"dns":"example.com","sv_cert_hash":"xyz"}]`,
			wantError: true,
			errSubstr: "hex",
		},
		{
			name:      "invalid_cl_cert_hash_hex",
			jsonBody:  `[{"dns":"example.com","cl_cert_hash":"12345"}]`,
			wantError: true,
			errSubstr: "hex",
		},
		{
			name:      "invalid_wifi_ssid_type_number",
			jsonBody:  `[{"dns":"example.com","wifi_ssid":123}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_device_port_number",
			jsonBody:  `[{"dns":"example.com","device_port":8080}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:     "owner_port_empty_is_ignored",
			jsonBody: `[{"dns":"example.com","owner_port":""}]`,
		},
		{
			name:      "invalid_json_malformed",
			jsonBody:  `[{bad}]`,
			wantError: true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid_top_level_object",
			jsonBody:  `{}`,
			wantError: true,
			errSubstr: "cannot unmarshal object",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseHumanReadableRvJSON([]byte(tc.jsonBody))
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseHumanToTO2AddrsJSON_Cases(t *testing.T) {
	cases := []struct {
		name      string
		jsonBody  string
		wantError bool
		errSubstr string
	}{
		{
			name:     "valid_dns_only",
			jsonBody: `[{"dns":"owner.example.com","port":"8043","protocol":"http"}]`,
		},
		{
			name:     "valid_ip_only",
			jsonBody: `[{"ip":"10.0.0.5","port":"8043","protocol":"https"}]`,
		},
		{
			name:     "valid_both",
			jsonBody: `[{"dns":"owner.example.com","ip":"10.0.0.5","port":"8043","protocol":"tls"}]`,
		},
		{
			name:      "invalid_missing_dns_ip",
			jsonBody:  `[{}]`,
			wantError: true,
			errSubstr: "at least one of dns or ip",
		},
		{
			name:      "invalid_protocol",
			jsonBody:  `[{"dns":"owner.example.com","port":"8043","protocol":"bogus"}]`,
			wantError: true,
			errSubstr: "unsupported transport protocol",
		},
		{
			name:      "invalid_port_non_numeric",
			jsonBody:  `[{"dns":"owner.example.com","port":"eightythree","protocol":"http"}]`,
			wantError: true,
			errSubstr: "port:",
		},
		{
			name:      "invalid_transport_protocol_case",
			jsonBody:  `[{"dns":"owner.example.com","port":"8043","protocol":"HTTP"}]`,
			wantError: true,
			errSubstr: "unsupported transport protocol",
		},
		{
			name:      "invalid_json_malformed",
			jsonBody:  `[{bad}]`,
			wantError: true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid_top_level_object",
			jsonBody:  `{}`,
			wantError: true,
			errSubstr: "cannot unmarshal object",
		},
		{
			name:      "invalid_protocol_type_number",
			jsonBody:  `[{"dns":"owner.example.com","port":"8043","protocol":1}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:     "port_empty_is_ignored",
			jsonBody: `[{"dns":"owner.example.com","port":"","protocol":"http"}]`,
		},
		{
			name:      "invalid_port_float",
			jsonBody:  `[{"dns":"owner.example.com","port":8043.5,"protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_dns_type_number",
			jsonBody:  `[{"dns":123,"port":"8043","protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_ip_type_number",
			jsonBody:  `[{"ip":123,"port":"8043","protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseHumanToTO2AddrsJSON([]byte(tc.jsonBody))
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
