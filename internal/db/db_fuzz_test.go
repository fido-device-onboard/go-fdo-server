package db

import (
	"testing"
)

// FuzzParseHumanReadableRvJSON ensures the RVInfo parser never panics on arbitrary inputs.
func FuzzParseHumanReadableRvJSON(f *testing.F) {
	// Seed with a minimal valid example
	f.Add([]byte(`[{"dns":"example.com","device_port":"8080","owner_port":"8080","protocol":"http"}]`))
	// Seed with IP-only
	f.Add([]byte(`[{"ip":"127.0.0.1","protocol":"http"}]`))
	// Seed with both DNS and IP
	f.Add([]byte(`[{"dns":"example.com","ip":"127.0.0.1","protocol":"http","medium":"eth_all","delay_seconds":10}]`))
	// Seed with invalid IPs
	f.Add([]byte(`[{"ip":"999.999.999.999","protocol":"http"}]`))
	f.Add([]byte(`[{"ip":"abc.def.ghi.jkl","protocol":"http"}]`))
	// Seed with extra fields
	f.Add([]byte(`[{"dns":"example.com","wifi_ssid":"ssid","wifi_pw":"pw","dev_only":true,"owner_only":false,"rv_bypass":true}]`))
	// Seed with cert hashes and delay
	f.Add([]byte(`[{"dns":"example.com","sv_cert_hash":"001122","cl_cert_hash":"aabbcc","delay_seconds":5}]`))
	// Seed malformed JSON
	f.Add([]byte(`[{bad}]`))
	// Seed empty array and empty object
	f.Add([]byte(`[]`))
	f.Add([]byte(`{}`))
	// Seed large list
	f.Add([]byte(`[{"dns":"a.com"},{"ip":"10.0.0.1"},{"dns":"b.com","device_port":"1","owner_port":"2"}]`))
	// Seed all flags
	f.Add([]byte(`[{"dns":"example.com","dev_only":true,"owner_only":true,"rv_bypass":true}]`))
	// Seed bad types
	f.Add([]byte(`[{"dns":123,"protocol":false,"device_port":{},"owner_port":[],"delay_seconds":"x"}]`))
	// Seed invalid medium
	f.Add([]byte(`[{"dns":"example.com","medium":"bad"}]`))
	// Seed uppercase protocol
	f.Add([]byte(`[{"dns":"example.com","protocol":"HTTP"}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic
		_, _ = parseHumanReadableRvJSON(data)
	})
}

// FuzzParseHumanToTO2AddrsJSON ensures the TO2 addresses parser never panics on arbitrary inputs.
func FuzzParseHumanToTO2AddrsJSON(f *testing.F) {
	// Seed with a minimal valid example
	f.Add([]byte(`[{"dns":"owner.example.com","port":"1","protocol":"http"}]`))
	// Seed with IP-only
	f.Add([]byte(`[{"ip":"192.168.1.10","port":"65535","protocol":"https"}]`))
	// Seed with both DNS and IP
	f.Add([]byte(`[{"dns":"owner.example.com","ip":"10.0.0.5","port":"65535","protocol":"tls"}]`))
	// Seed with invalid IPs
	f.Add([]byte(`[{"ip":"300.300.300.300","port":"8043","protocol":"http"}]`))
	f.Add([]byte(`[{"ip":"not.an.ip","port":"8043","protocol":"http"}]`))
	// Seed malformed JSON
	f.Add([]byte(`[{bad}]`))
	// Seed with missing fields
	f.Add([]byte(`[]`))
	// Seed empty object
	f.Add([]byte(`{}`))
	// Seed bad types
	f.Add([]byte(`[{"dns":123,"ip":false,"port":{},"protocol":[]}]`))
	// Seed large list
	f.Add([]byte(`[{"dns":"a.com","port":"1"},{"dns":"b.com","ip":"10.0.0.2","port":"2"}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic
		_, _ = parseHumanToTO2AddrsJSON(data)
	})
}
