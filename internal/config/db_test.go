// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package config

import (
	"strings"
	"testing"
)

// extractPasswordValue extracts the password value from a DSN fragment
// that starts right after "password=". Handles both quoted and unquoted values.
func extractPasswordValue(fragment string) string {
	if len(fragment) == 0 {
		return ""
	}

	// Check if it starts with a quote
	if fragment[0] == '\'' {
		// Find the closing quote, handling escaped quotes
		i := 1
		for i < len(fragment) {
			if fragment[i] == '\\' && i+1 < len(fragment) {
				i += 2
				continue
			}
			if fragment[i] == '\'' {
				return fragment[1:i]
			}
			i++
		}
		// No closing quote found, return everything after opening quote
		return fragment[1:]
	}

	// Unquoted value - read until space or end
	i := 0
	for i < len(fragment) && fragment[i] != ' ' && fragment[i] != '\t' {
		i++
	}
	return fragment[:i]
}

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
			expected: "postgres://myuser:********@localhost:5432/mydb",
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
			expected: "host=localhost port=5432 user=myuser password=******** dbname=mydb sslmode=disable",
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
		{
			name:     "PostgreSQL key-value format with password containing spaces",
			dbType:   "postgres",
			dsn:      "host=localhost password='my secret password' dbname=mydb",
			expected: "host=localhost password='********' dbname=mydb",
		},
		{
			name:     "PostgreSQL key-value format with multiple quoted values",
			dbType:   "postgres",
			dsn:      "host='my host' password='my password' dbname='my db'",
			expected: "host='my host' password='********' dbname='my db'",
		},
		{
			name:     "PostgreSQL key-value format with password containing escaped quotes",
			dbType:   "postgres",
			dsn:      "host=localhost password='it\\'s a secret' dbname=mydb",
			expected: "host=localhost password='********' dbname=mydb",
		},
		{
			name:     "PostgreSQL key-value format with mixed quoted and unquoted",
			dbType:   "postgres",
			dsn:      "host=localhost port=5432 password='secret pass' user=admin dbname=test",
			expected: "host=localhost port=5432 password='********' user=admin dbname=test",
		},
		{
			name:     "PostgreSQL key-value format with password at end",
			dbType:   "postgres",
			dsn:      "host=localhost dbname=mydb password='my secret'",
			expected: "host=localhost dbname=mydb password='********'",
		},
		{
			name:     "PostgreSQL key-value format with password at beginning",
			dbType:   "postgres",
			dsn:      "password='my secret' host=localhost dbname=mydb",
			expected: "password='********' host=localhost dbname=mydb",
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
				// For key=value format, extract the actual password value
				if strings.Contains(tt.dsn, "password=") {
					// Extract password value (handling both quoted and unquoted)
					parts := strings.Split(tt.dsn, "password=")
					if len(parts) > 1 {
						passwordValue := extractPasswordValue(parts[1])
						// Verify the actual password is not in the output
						if passwordValue != "" && passwordValue != "********" {
							if strings.Contains(got, passwordValue) {
								t.Errorf("RedactedDSN() still contains password %q in output: %q", passwordValue, got)
							}
						}
					}
				} else if strings.Contains(tt.dsn, ":") && strings.Contains(tt.dsn, "@") {
					// URL format - verify password is redacted
					if tt.dsn != got && !strings.Contains(got, "********") {
						t.Errorf("RedactedDSN() should contain ******** for URL with password: %q", got)
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
	if !strings.Contains(str, "********") {
		t.Errorf("String() should contain redaction marker, got: %s", str)
	}
}
