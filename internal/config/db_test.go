// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package config

import (
	"strings"
	"testing"
)

func TestDatabaseConfig_RedactedDSN(t *testing.T) {
	tests := []struct {
		name     string
		dbType   string
		dsn      string
		expected string
	}{
		{
			name:     "SQLite file path - no redaction needed",
			dbType:   "sqlite",
			dsn:      "/var/lib/fdo/fdo.db",
			expected: "/var/lib/fdo/fdo.db",
		},
		{
			name:     "SQLite memory - no redaction needed",
			dbType:   "sqlite",
			dsn:      ":memory:",
			expected: ":memory:",
		},
		{
			name:     "PostgreSQL URL format with password",
			dbType:   "postgres",
			dsn:      "postgres://myuser:mypassword@localhost:5432/mydb",
			expected: "postgres://myuser:***REDACTED***@localhost:5432/mydb",
		},
		{
			name:     "PostgreSQL URL format without password",
			dbType:   "postgres",
			dsn:      "postgres://myuser@localhost:5432/mydb",
			expected: "postgres://myuser@localhost:5432/mydb",
		},
		{
			name:     "PostgreSQL key-value format with password",
			dbType:   "postgres",
			dsn:      "host=localhost port=5432 user=myuser password=secret dbname=mydb sslmode=disable",
			expected: "host=localhost port=5432 user=myuser password=***REDACTED*** dbname=mydb sslmode=disable",
		},
		{
			name:     "PostgreSQL key-value format without password",
			dbType:   "postgres",
			dsn:      "host=localhost port=5432 user=myuser dbname=mydb",
			expected: "host=localhost port=5432 user=myuser dbname=mydb",
		},
		{
			name:     "Empty DSN",
			dbType:   "postgres",
			dsn:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &DatabaseConfig{
				Type: tt.dbType,
				DSN:  tt.dsn,
			}

			got := dc.RedactedDSN()
			if got != tt.expected {
				t.Errorf("RedactedDSN() = %q, want %q", got, tt.expected)
			}

			// Verify that the original password is NOT in the redacted output
			if tt.dsn != "" && strings.Contains(tt.dsn, "password") {
				// Extract password value from original DSN
				if strings.Contains(tt.dsn, "password=") {
					parts := strings.Split(tt.dsn, "password=")
					if len(parts) > 1 {
						passwordPart := strings.Fields(parts[1])[0]
						if passwordPart != "" && passwordPart != "***REDACTED***" {
							if strings.Contains(got, passwordPart) {
								t.Errorf("RedactedDSN() still contains password: %q", got)
							}
						}
					}
				} else if strings.Contains(tt.dsn, ":") && strings.Contains(tt.dsn, "@") {
					// URL format - check password is redacted
					if strings.Contains(got, tt.dsn) && tt.dsn != got {
						// Password should have been redacted
						if !strings.Contains(got, "***REDACTED***") {
							t.Errorf("RedactedDSN() should contain ***REDACTED***: %q", got)
						}
					}
				}
			}
		})
	}
}

func TestDatabaseConfig_String(t *testing.T) {
	dc := DatabaseConfig{
		Type: "postgres",
		DSN:  "postgres://user:secret@localhost/db",
	}

	str := dc.String()

	// Should contain the type
	if !strings.Contains(str, "postgres") {
		t.Errorf("String() should contain database type, got: %s", str)
	}

	// Should NOT contain the actual password
	if strings.Contains(str, "secret") {
		t.Errorf("String() should not contain the actual password, got: %s", str)
	}

	// Should contain the redaction marker
	if !strings.Contains(str, "***REDACTED***") {
		t.Errorf("String() should contain redaction marker, got: %s", str)
	}
}
