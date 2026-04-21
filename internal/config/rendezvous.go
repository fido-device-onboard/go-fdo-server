package config

import (
	"fmt"
	"log/slog"
)

const (
	// DefaultMinWaitSecs Default minimum wait time in seconds for TO0 rendezvous entries (requests below this are rejected)
	// Default: 0 (no minimum)
	DefaultMinWaitSecs uint32 = 0
	// DefaultMaxWaitSecs Default maximum wait time in seconds for TO0 rendezvous entries (requests above this are capped)
	// Default: 86400 (24h)
	DefaultMaxWaitSecs uint32 = 86400
	// DefaultCleanupIntervalSecs Default interval in seconds for cleaning up expired rendezvous blobs and sessions.
	// Set to 0 to disable.
	DefaultCleanupIntervalSecs uint32 = 3600
	// DefaultSessionMaxAgeSecs Default maximum age in seconds for sessions before cleanup.
	DefaultSessionMaxAgeSecs uint32 = 3600
	// DefaultInitialCleanupDelaySecs Default delay in seconds before the first cleanup run after startup.
	DefaultInitialCleanupDelaySecs uint32 = 300
)

// RendezvousServerConfig server configuration file structure
type RendezvousServerConfig struct {
	ServerConfig `mapstructure:",squash"`
	Rendezvous   RendezvousConfig `mapstructure:"rendezvous"`
}

// String returns a string representation of RendezvousServerConfig
func (rv RendezvousServerConfig) String() string {
	return fmt.Sprintf("RendezvousServerConfig{DB: %s, HTTP: %+v, Rendezvous: %+v, Log: %+v}",
		rv.DB.String(), rv.HTTP, rv.Rendezvous, rv.Log)
}

// Validate checks that required configuration is present
func (rv *RendezvousServerConfig) Validate() error {
	slog.Debug("Validating rendezvous server configuration")
	if err := rv.HTTP.Validate(); err != nil {
		slog.Error("HTTP configuration validation failed", "error", err)
		return err
	}
	if err := rv.Rendezvous.Validate(); err != nil {
		slog.Error("rendezvous configuration validation failed", "error", err)
		return err
	}
	slog.Debug("Rendezvous server configuration validated successfully")

	return nil
}

// RendezvousConfig server configuration
type RendezvousConfig struct {
	// MinWaitSecs is the minimum time in seconds the rendezvous server will accept
	// to maintain a rendezvous blob registered in the database.
	// If an owner server requests a wait time lower than this value during TO0,
	// the request will be rejected.
	// Default: 0 (no minimum)
	MinWaitSecs uint32 `mapstructure:"to0_min_wait"`

	// MaxWaitSecs is the maximum time in seconds the rendezvous server will accept
	// to maintain a rendezvous blob registered in the database.
	// If an owner server requests a wait time higher than this value during TO0,
	// the request will be accepted but capped at this maximum value.
	// Default: 86400 (24h)
	MaxWaitSecs uint32 `mapstructure:"to0_max_wait"`

	// CleanupIntervalSecs is the interval in seconds at which the server automatically
	// purges expired rendezvous blobs and old sessions from the database.
	// Set to 0 to disable automatic cleanup.
	// Default: 3600 (1 hour)
	CleanupIntervalSecs uint32 `mapstructure:"cleanup_interval"`

	// SessionMaxAgeSecs is the maximum age in seconds for sessions before they are
	// considered expired and removed during cleanup.
	// Default: 3600 (1 hour)
	SessionMaxAgeSecs uint32 `mapstructure:"session_timeout"`

	// InitialCleanupDelaySecs is the delay in seconds before the first cleanup runs
	// after startup, to avoid a spike when restarting with large amounts of expired data.
	// Default: 300 (5 minutes)
	InitialCleanupDelaySecs uint32 `mapstructure:"initial_cleanup_delay"`
}

func (rv *RendezvousConfig) Validate() error {
	if rv.MinWaitSecs > rv.MaxWaitSecs {
		return fmt.Errorf("'to0_max_wait' (%d) must be greater or equal than 'to0_min_wait' (%d)", rv.MaxWaitSecs, rv.MinWaitSecs)
	}
	return nil
}
