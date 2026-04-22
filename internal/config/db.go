package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DatabaseConfig configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

// String returns a redacted string representation of the DatabaseConfig
// that masks sensitive information in the DSN
func (dc DatabaseConfig) String() string {
	redactedDSN := dc.RedactedDSN()
	return fmt.Sprintf("DatabaseConfig{Type: %q, DSN: %q}", dc.Type, redactedDSN)
}

// RedactedDSN returns the DSN with sensitive information (passwords) redacted
func (dc *DatabaseConfig) RedactedDSN() string {
	if dc.DSN == "" {
		return ""
	}

	// For SQLite, just show the path (no sensitive data typically)
	if strings.ToLower(dc.Type) == "sqlite" {
		return dc.DSN
	}

	// For PostgreSQL, redact password from connection string
	// Format: postgres://user:password@host:port/database?params
	// or: host=localhost port=5432 user=myuser password=mypass dbname=mydb
	dsn := dc.DSN

	// Try parsing as URL first
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		if u, err := url.Parse(dsn); err == nil {
			if u.User != nil {
				username := u.User.Username()
				_, hasPassword := u.User.Password()
				if hasPassword {
					// Build redacted URL manually to avoid URL encoding of asterisks
					userInfo := username + ":********"
					redactedURL := u.Scheme + "://" + userInfo + "@" + u.Host + u.Path
					if u.RawQuery != "" {
						redactedURL += "?" + u.RawQuery
					}
					if u.Fragment != "" {
						redactedURL += "#" + u.Fragment
					}
					return redactedURL
				}
				// No password in URL, return as-is
				return dsn
			}
			// No user info at all
			return dsn
		}
	}

	// Handle key=value format (libpq connection string)
	if strings.Contains(dsn, "password=") {
		return redactLibpqPassword(dsn)
	}

	// If it's key=value format without password, return as-is
	if strings.Contains(dsn, "=") && (strings.Contains(dsn, "host=") || strings.Contains(dsn, "dbname=")) {
		return dsn
	}

	// If we can't parse it, redact the entire DSN to be safe
	return "********"
}

// redactLibpqPassword redacts the password from a libpq-style connection string
// Handles quoted values properly, including passwords with spaces
func redactLibpqPassword(dsn string) string {
	var result strings.Builder
	i := 0

	for i < len(dsn) {
		// Skip whitespace
		for i < len(dsn) && (dsn[i] == ' ' || dsn[i] == '\t') {
			result.WriteByte(dsn[i])
			i++
		}

		if i >= len(dsn) {
			break
		}

		// Read key
		keyStart := i
		for i < len(dsn) && dsn[i] != '=' && dsn[i] != ' ' && dsn[i] != '\t' {
			i++
		}

		if i >= len(dsn) || dsn[i] != '=' {
			// No equals sign, just copy rest and break
			result.WriteString(dsn[keyStart:])
			break
		}

		key := dsn[keyStart:i]
		result.WriteString(key)
		result.WriteByte('=') // Write the '='
		i++                   // Skip the '='

		// Read value (handling quotes)
		if i < len(dsn) && dsn[i] == '\'' {
			// Quoted value
			i++ // Skip opening quote
			valueStart := i

			// Find closing quote, handling escaped quotes
			for i < len(dsn) {
				if dsn[i] == '\\' && i+1 < len(dsn) {
					// Skip escaped character
					i += 2
					continue
				}
				if dsn[i] == '\'' {
					break
				}
				i++
			}

			if key == "password" {
				result.WriteString("'********'")
			} else {
				// Copy the quoted value
				result.WriteByte('\'')
				result.WriteString(dsn[valueStart:i])
				result.WriteByte('\'')
			}

			if i < len(dsn) {
				i++ // Skip closing quote
			}
		} else {
			// Unquoted value
			valueStart := i
			for i < len(dsn) && dsn[i] != ' ' && dsn[i] != '\t' {
				i++
			}

			if key == "password" {
				result.WriteString("********")
			} else {
				result.WriteString(dsn[valueStart:i])
			}
		}
	}

	return result.String()
}

func (dc *DatabaseConfig) GetDB() (*gorm.DB, error) {
	dsn := dc.DSN
	dialect := strings.ToLower(dc.Type)
	slog.Debug("Initializing database", "type", dialect, "dsn", dc.RedactedDSN())
	if dsn == "" {
		slog.Error("Database DSN is required but not provided")
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	slog.Debug("Validating database type", "type", dialect)
	if dialect != "sqlite" && dialect != "postgres" {
		slog.Error("Unsupported database type", "type", dialect, "supported", []string{"sqlite", "postgres"})
		return nil, fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dialect)
	}

	var dialector gorm.Dialector

	switch dialect {
	case "sqlite":
		dialector = sqlite.Open(dc.DSN)
	case "postgres":
		dialector = postgres.Open(dc.DSN)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dialect)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Enable foreign keys for SQLite
	if dialect == "sqlite" {
		sqlDB, err := db.DB()
		if err != nil {
			slog.Warn("Failed to get underlying SQL DB for foreign key setup", "error", err)
		} else if _, err = sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
			slog.Warn("Failed to enable SQLite foreign keys", "error", err)
		}
	}
	return db, nil
}
