// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type AuthConfig struct {
	Enabled       bool             `mapstructure:"enabled"`
	ExcludedPaths []string         `mapstructure:"excluded_paths"`
	Mechanisms    MechanismsConfig `mapstructure:"mechanisms"`
	Seed          *SeedConfig      `mapstructure:"seed"`
}

type MechanismsConfig struct {
	APIKey *APIKeyMechanismConfig `mapstructure:"api_key"`
}

type APIKeyMechanismConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type SeedConfig struct {
	Admin *SeedAdminConfig `mapstructure:"admin"`
}

type SeedAdminConfig struct {
	Name   string `mapstructure:"name"`
	Email  string `mapstructure:"email"`
	APIKey string `mapstructure:"api_key"`
}

func (a *AuthConfig) Validate() error {
	if !a.Enabled {
		slog.Debug("Auth is disabled")
		return nil
	}

	if !a.HasEnabledMechanism() {
		return errors.New("auth is enabled but no authentication mechanism is configured")
	}

	for _, p := range a.ExcludedPaths {
		if p == "" || !strings.HasPrefix(p, "/") {
			return fmt.Errorf("invalid excluded path %q: must start with '/'", p)
		}
	}

	slog.Debug("Auth configuration validated", "mechanisms", a.enabledMechanismNames())
	return nil
}

func (a *AuthConfig) HasEnabledMechanism() bool {
	if a.Mechanisms.APIKey != nil && a.Mechanisms.APIKey.Enabled {
		return true
	}
	return false
}

func (a *AuthConfig) enabledMechanismNames() []string {
	var names []string
	if a.Mechanisms.APIKey != nil && a.Mechanisms.APIKey.Enabled {
		names = append(names, "api-key")
	}
	return names
}
