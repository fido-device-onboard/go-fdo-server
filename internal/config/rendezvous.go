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
		slog.Error("HTTP configuration validation failed", "err", err)
		return err
	}
	if err := rv.Rendezvous.Validate(); err != nil {
		slog.Error("rendezvous configuration validation failed", "err", err)
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
}

func (rv *RendezvousConfig) Validate() error {
	if rv.MinWaitSecs > rv.MaxWaitSecs {
		return fmt.Errorf("'to0_max_wait' (%d) must be greater or equal than 'to0_min_wait' (%d)", rv.MaxWaitSecs, rv.MinWaitSecs)
	}
	return nil
}
