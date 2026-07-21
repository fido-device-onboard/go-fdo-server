// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	apiKeyPrefix    = "fdo_"
	apiKeyRandBytes = 32
	apiKeyPrefixLen = 6
	base62MinLen    = 43
)

type APIKey struct {
	ID              string     `gorm:"type:varchar(36);primaryKey"`
	Prefix          string     `gorm:"type:varchar(6);not null;index"`
	HashedKey       []byte     `gorm:"not null"`
	Name            string     `gorm:"type:varchar(255);not null"`
	UserID          string     `gorm:"type:varchar(36);not null;index"`
	UserRef         *User      `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	Scopes          string     `gorm:"type:text"`
	ScopeRestricted bool       `gorm:"type:boolean;not null;default:false"`
	ExpiresAt       *time.Time `gorm:"index"`
	Active          bool       `gorm:"type:boolean;not null"`
	LastUsedAt      *time.Time
	CreatedAt       time.Time `gorm:"autoCreateTime:milli"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime:milli"`
}

func (APIKey) TableName() string { return "api_keys" }

func (a *APIKey) ScopesList() ([]string, error) {
	if a.Scopes == "" {
		return nil, nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(a.Scopes), &scopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
	}
	return scopes, nil
}

func generateAPIKey() (string, error) {
	bytes := make([]byte, apiKeyRandBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	var num big.Int
	num.SetBytes(bytes)
	val := num.Text(62)
	if len(val) < base62MinLen {
		val = strings.Repeat("0", base62MinLen-len(val)) + val
	}
	return apiKeyPrefix + val, nil
}

func CreateAPIKey(ctx context.Context, db *gorm.DB, name, userID string, scopes []string, expiresAt *time.Time) (*APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("API key name must not be empty")
	}
	if len(name) > 255 {
		return nil, "", fmt.Errorf("API key name must not exceed 255 characters")
	}
	for _, s := range scopes {
		if !validScopePattern.MatchString(s) {
			return nil, "", fmt.Errorf("invalid scope %q: must match [a-z0-9][a-z0-9:_-]*", s)
		}
	}

	cleartext, err := generateAPIKey()
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256([]byte(cleartext))
	hashed := hash[:]

	var scopesJSON string
	if len(scopes) > 0 {
		b, err := json.Marshal(scopes)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal scopes: %w", err)
		}
		scopesJSON = string(b)
	}

	apiKey := &APIKey{
		ID:              uuid.New().String(),
		Prefix:          cleartext[len(apiKeyPrefix) : len(apiKeyPrefix)+apiKeyPrefixLen],
		HashedKey:       hashed,
		Name:            name,
		UserID:          userID,
		Scopes:          scopesJSON,
		ScopeRestricted: len(scopes) > 0,
		ExpiresAt:       expiresAt,
		Active:          true,
	}

	if err := db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}
	return apiKey, cleartext, nil
}

func FindAPIKeysByPrefix(ctx context.Context, db *gorm.DB, prefix string) ([]APIKey, error) {
	var keys []APIKey
	if err := db.WithContext(ctx).Where("prefix = ? AND active = ?", prefix, true).Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to find API keys by prefix: %w", err)
	}
	return keys, nil
}

func UpdateAPIKeyLastUsed(ctx context.Context, db *gorm.DB, id string) {
	now := time.Now()
	if err := db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", id).UpdateColumn("last_used_at", now).Error; err != nil {
		slog.Warn("Failed to update API key last_used_at", "id", id, "error", err)
	}
}
