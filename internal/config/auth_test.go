// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package config

import "testing"

func TestAuthConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AuthConfig
		wantErr bool
	}{
		{
			name: "disabled",
			cfg:  AuthConfig{Enabled: false},
		},
		{
			name:    "enabled no mechanisms",
			cfg:     AuthConfig{Enabled: true},
			wantErr: true,
		},
		{
			name: "enabled with api key",
			cfg: AuthConfig{
				Enabled:    true,
				Mechanisms: MechanismsConfig{APIKey: &APIKeyMechanismConfig{Enabled: true}},
			},
		},
		{
			name: "enabled api key disabled",
			cfg: AuthConfig{
				Enabled:    true,
				Mechanisms: MechanismsConfig{APIKey: &APIKeyMechanismConfig{Enabled: false}},
			},
			wantErr: true,
		},
		{
			name: "invalid excluded path missing slash",
			cfg: AuthConfig{
				Enabled:       true,
				Mechanisms:    MechanismsConfig{APIKey: &APIKeyMechanismConfig{Enabled: true}},
				ExcludedPaths: []string{"health"},
			},
			wantErr: true,
		},
		{
			name: "valid excluded path",
			cfg: AuthConfig{
				Enabled:       true,
				Mechanisms:    MechanismsConfig{APIKey: &APIKeyMechanismConfig{Enabled: true}},
				ExcludedPaths: []string{"/custom/public"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
