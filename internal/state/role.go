// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role struct {
	ID          string    `gorm:"type:varchar(36);primaryKey"`
	Name        string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	Description string    `gorm:"type:text"`
	BuiltIn     bool      `gorm:"type:boolean;not null;default:false"`
	CreatedAt   time.Time `gorm:"autoCreateTime:milli"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime:milli"`
}

func (Role) TableName() string { return "roles" }

type RoleScope struct {
	RoleID  string `gorm:"type:varchar(36);not null;primaryKey"`
	Scope   string `gorm:"type:varchar(255);not null;primaryKey"`
	RoleRef *Role  `gorm:"foreignKey:RoleID;references:ID;constraint:OnDelete:CASCADE"`
}

func (RoleScope) TableName() string { return "role_scopes" }

type UserRole struct {
	UserID  string `gorm:"type:varchar(36);not null;primaryKey"`
	RoleID  string `gorm:"type:varchar(36);not null;primaryKey"`
	UserRef *User  `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
	RoleRef *Role  `gorm:"foreignKey:RoleID;references:ID;constraint:OnDelete:CASCADE"`
}

func (UserRole) TableName() string { return "user_roles" }

func CreateRole(ctx context.Context, db *gorm.DB, name, description string, builtIn bool, scopes []string) (*Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("role name must not be empty")
	}
	if len(name) > 255 {
		return nil, fmt.Errorf("role name must not exceed 255 characters")
	}
	if len(description) > 1024 {
		return nil, fmt.Errorf("role description must not exceed 1024 characters")
	}
	for _, s := range scopes {
		if !validScopePattern.MatchString(s) {
			return nil, fmt.Errorf("invalid scope %q: must match [a-z0-9][a-z0-9:_-]*", s)
		}
	}

	role := &Role{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		BuiltIn:     builtIn,
	}
	uniqueScopes := make([]string, 0, len(scopes))
	seen := make(map[string]struct{})
	for _, s := range scopes {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			uniqueScopes = append(uniqueScopes, s)
		}
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}
		if len(uniqueScopes) > 0 {
			roleScopes := make([]RoleScope, len(uniqueScopes))
			for i, scope := range uniqueScopes {
				roleScopes[i] = RoleScope{RoleID: role.ID, Scope: scope}
			}
			if err := tx.Create(&roleScopes).Error; err != nil {
				return fmt.Errorf("failed to create role scopes: %w", err)
			}
		}
		return nil
	})
	return role, err
}

func FindOrCreateRole(ctx context.Context, db *gorm.DB, name, description string, builtIn bool, scopes []string) (*Role, error) {
	var existing Role
	err := db.WithContext(ctx).Where("name = ?", name).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check for existing role %q: %w", name, err)
	}
	role, err := CreateRole(ctx, db, name, description, builtIn, scopes)
	if err != nil {
		if retryErr := db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; retryErr == nil {
			return &existing, nil
		}
		return nil, err
	}
	return role, nil
}

func AssignRoleToUser(ctx context.Context, db *gorm.DB, userID, roleID string) error {
	if err := db.WithContext(ctx).Create(&UserRole{UserID: userID, RoleID: roleID}).Error; err != nil {
		return fmt.Errorf("failed to assign role to user: %w", err)
	}
	return nil
}

func GetUserRoles(ctx context.Context, db *gorm.DB, userID string) ([]Role, error) {
	var roles []Role
	if err := db.WithContext(ctx).Select("roles.*").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	return roles, nil
}

func GetUserScopes(ctx context.Context, db *gorm.DB, userID string) ([]string, error) {
	var scopes []string
	if err := db.WithContext(ctx).Model(&RoleScope{}).
		Joins("JOIN user_roles ON user_roles.role_id = role_scopes.role_id").
		Where("user_roles.user_id = ?", userID).
		Distinct().
		Pluck("role_scopes.scope", &scopes).Error; err != nil {
		return nil, fmt.Errorf("failed to get user scopes: %w", err)
	}
	return scopes, nil
}
