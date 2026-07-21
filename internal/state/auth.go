// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"gorm.io/gorm"
)

var validScopePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9:_-]*$`)

func InitAuthDB(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&User{}, &Role{}, &RoleScope{}, &UserRole{}, &APIKey{}); err != nil {
		return fmt.Errorf("failed to migrate auth database schema: %w", err)
	}
	slog.Info("Auth database initialized successfully")
	return nil
}
