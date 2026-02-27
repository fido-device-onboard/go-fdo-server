package config

import (
	"database/sql"
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
					userInfo := username + ":***REDACTED***"
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
		// Redact password value
		parts := strings.Fields(dsn)
		var redacted []string
		for _, part := range parts {
			if strings.HasPrefix(part, "password=") {
				redacted = append(redacted, "password=***REDACTED***")
			} else {
				redacted = append(redacted, part)
			}
		}
		return strings.Join(redacted, " ")
	}

	// If it's key=value format without password, return as-is
	if strings.Contains(dsn, "=") && (strings.Contains(dsn, "host=") || strings.Contains(dsn, "dbname=")) {
		return dsn
	}

	// If we can't parse it, redact the entire DSN to be safe
	return "***REDACTED***"
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
		var sqlDB *sql.DB
		if sqlDB, err = db.DB(); err == nil {
			_, _ = sqlDB.Exec("PRAGMA foreign_keys = ON")
		}
	}
	return db, nil
}
