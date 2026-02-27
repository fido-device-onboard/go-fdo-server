package config

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DatabaseConfig configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

func (dc *DatabaseConfig) GetDB() (*gorm.DB, error) {
	dsn := dc.DSN
	dialect := strings.ToLower(dc.Type)
	slog.Debug("Initializing database", "type", dialect, "dsn", dsn)
	if dsn == "" {
		slog.Error("Database DSN is required but not provided")
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	slog.Debug("Validating database type", "type", dialect)
	if dc.Type != "sqlite" && dialect != "postgres" {
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

func (dc *DatabaseConfig) getState() (*db.State, error) {
	slog.Debug("Initializing database state", "type", dc.Type, "dsn", dc.DSN)
	if dc.DSN == "" {
		slog.Error("Database DSN is required but not provided")
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	dc.Type = strings.ToLower(dc.Type)
	slog.Debug("Validating database type", "type", dc.Type)
	if dc.Type != "sqlite" && dc.Type != "postgres" {
		slog.Error("Unsupported database type", "type", dc.Type, "supported", []string{"sqlite", "postgres"})
		return nil, fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dc.Type)
	}

	slog.Debug("Calling db.InitDb", "type", dc.Type)
	state, err := db.InitDb(dc.Type, dc.DSN)
	if err != nil {
		slog.Error("Failed to initialize database", "type", dc.Type, "dsn", dc.DSN, "err", err)
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Debug("Database state initialized successfully", "type", dc.Type)
	return state, nil
}
