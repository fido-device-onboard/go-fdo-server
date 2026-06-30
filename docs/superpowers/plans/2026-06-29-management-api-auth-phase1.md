# Management API Auth Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add authentication (API key) and authorization (RBAC + scopes) to the FDO server management API, with a pluggable framework for future auth mechanisms.

**Architecture:** Middleware chain (AuthN → AuthZ) inserted between rate limiter and OpenAPI validation. Scopes parsed from `x-required-scopes` OpenAPI extensions at startup. API key authenticator as the first pluggable mechanism. Auth disabled by default for backward compatibility.

**Tech Stack:** Go 1.26, GORM (SQLite/PostgreSQL), `crypto/sha256` + `crypto/subtle` (stdlib), `github.com/getkin/kin-openapi`, Cobra/Viper, oapi-codegen

## Global Constraints

- Module path: `github.com/fido-device-onboard/go-fdo-server`
- Existing GORM model pattern: models in `internal/state/`, init functions return `(*State, error)`, `AutoMigrate` called in init
- Existing middleware pattern: `func(http.Handler) http.Handler` closures in `internal/middleware/`
- Existing config pattern: structs with `mapstructure` tags in `internal/config/`, validated via `.Validate()` methods
- Existing CLI pattern: Cobra commands in `cmd/`, Viper for config binding
- OpenAPI specs: per-resource YAML in `api/v2/<resource>/openapi.yaml`, merged into `openapi.json` via `go:generate npx openapi-format`
- FDO protocol endpoints (`/fdo/101/msg/*`, `/fdo/200/msg/*`), `/health`, `/api/docs/*`, `/api/openapi.json` are NEVER subject to management API auth
- Default behavior when `auth` is not configured: all endpoints open (backward compatible)
- SPDX header: `// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.` + `// SPDX-License-Identifier: Apache 2.0`
- Error responses: `{"message": "..."}` JSON format, matching existing `errorResponse` struct pattern

---

### Task 1: Database Models for Users, Roles, and API Keys

**Files:**
- Create: `internal/state/user.go`
- Create: `internal/state/role.go`
- Create: `internal/state/apikey.go`
- Create: `internal/state/auth.go`
- Create: `internal/state/user_test.go`
- Create: `internal/state/role_test.go`
- Create: `internal/state/auth_test.go`
- Create: `internal/state/apikey_test.go`

**Interfaces:**
- Consumes: `gorm.io/gorm` (existing dependency)
- Produces:
  - `type User struct` — GORM model with ID (UUID string), Name, Email, Active, CreatedAt, UpdatedAt
  - `type Role struct` — GORM model with ID (UUID string), Name, Description, BuiltIn (bool), CreatedAt, UpdatedAt
  - `type UserRole struct` — join table with UserID, RoleID
  - `type RoleScope struct` — GORM model with RoleID, Scope (string)
  - `type APIKey struct` — GORM model with ID (UUID string), Prefix (6 chars), HashedKey ([]byte), Name, UserID (FK), Scopes (JSON string slice), ExpiresAt (*time.Time), Active (bool), LastUsedAt (*time.Time), CreatedAt, UpdatedAt
  - `func InitAuthDB(ctx context.Context, db *gorm.DB) error` — runs AutoMigrate for all auth models
  - `func CreateUser(ctx context.Context, db *gorm.DB, name, email string) (*User, error)`
  - `func GetUserByID(ctx context.Context, db *gorm.DB, id string) (*User, error)`
  - `func GetUserRoles(ctx context.Context, db *gorm.DB, userID string) ([]Role, error)`
  - `func GetUserScopes(ctx context.Context, db *gorm.DB, userID string) ([]string, error)`
  - `func CreateRole(ctx context.Context, db *gorm.DB, name, description string, builtIn bool, scopes []string) (*Role, error)`
  - `func AssignRoleToUser(ctx context.Context, db *gorm.DB, userID, roleID string) error`
  - `func CreateAPIKey(ctx context.Context, db *gorm.DB, name, userID string, scopes []string, expiresAt *time.Time) (*APIKey, string, error)` — returns model + cleartext key
  - `func FindAPIKeysByPrefix(ctx context.Context, db *gorm.DB, prefix string) ([]APIKey, error)`
  - `func UpdateAPIKeyLastUsed(ctx context.Context, db *gorm.DB, id string)`
  - `func UserCount(ctx context.Context, db *gorm.DB) (int64, error)`

- [ ] **Step 1: Write test for User CRUD**

Create `internal/state/user_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if err := InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("Failed to initialize auth database: %v", err)
	}
	return db
}

func TestCreateUser(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.ID == "" {
		t.Error("expected non-empty ID")
	}
	if user.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", user.Name)
	}
	if user.Email != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got %q", user.Email)
	}
	if !user.Active {
		t.Error("expected user to be active")
	}
}

func TestGetUserByID(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	created, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	found, err := GetUserByID(ctx, db, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
	if found.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", found.Name)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	db := setupAuthTestDB(t)
	_, err := GetUserByID(t.Context(), db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestUserCount(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	if _, err := CreateUser(ctx, db, "admin", "admin@example.com"); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	count, err = UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestCreateUser_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name  string
		uname string
		email string
	}{
		{"empty name", "", "test@example.com"},
		{"empty email", "admin", ""},
		{"invalid email", "admin", "not-an-email"},
		{"display name form", "admin", "Admin <admin@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateUser(ctx, db, tt.uname, tt.email)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -run "TestCreateUser|TestGetUserByID|TestUserCount" -v`
Expected: FAIL — `InitAuthDB`, `CreateUser`, `GetUserByID`, `UserCount` not defined

- [ ] **Step 3: Implement User model and operations**

Create `internal/state/user.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID        string    `gorm:"type:varchar(36);primaryKey"`
	Name      string    `gorm:"type:varchar(255);not null"`
	Email     string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	Active    bool      `gorm:"type:boolean;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime:milli"`
	UpdatedAt time.Time `gorm:"autoUpdateTime:milli"`
}

func (User) TableName() string { return "users" }

func CreateUser(ctx context.Context, db *gorm.DB, name, email string) (*User, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" {
		return nil, fmt.Errorf("user name must not be empty")
	}
	if len(name) > 255 {
		return nil, fmt.Errorf("user name must not exceed 255 characters")
	}
	if len(email) > 255 {
		return nil, fmt.Errorf("email must not exceed 255 characters")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return nil, fmt.Errorf("invalid email address: %q", email)
	}

	user := &User{
		ID:     uuid.New().String(),
		Name:   name,
		Email:  email,
		Active: true,
	}
	if err := db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return user, nil
}

func GetUserByID(ctx context.Context, db *gorm.DB, id string) (*User, error) {
	var user User
	if err := db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

func UserCount(ctx context.Context, db *gorm.DB) (int64, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}
```

- [ ] **Step 4: Run user tests to verify they pass**

Run: `go test ./internal/state/ -run "TestCreateUser|TestGetUserByID|TestUserCount" -v`
Expected: PASS

- [ ] **Step 5: Write test for Role CRUD**

Create `internal/state/role_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"slices"
	"testing"
)

func TestCreateRole(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if role.ID == "" {
		t.Error("expected non-empty ID")
	}
	if role.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", role.Name)
	}
	if !role.BuiltIn {
		t.Error("expected BuiltIn to be true")
	}
}

func TestAssignRoleAndGetScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	expectedScopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete"}
	role, err := CreateRole(ctx, db, "admin", "Full access", true, expectedScopes)
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	user, err := CreateUser(ctx, db, "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	roles, err := GetUserRoles(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("GetUserRoles failed: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
	if roles[0].Name != "admin" {
		t.Errorf("expected role name 'admin', got %q", roles[0].Name)
	}

	scopes, err := GetUserScopes(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("GetUserScopes failed: %v", err)
	}
	slices.Sort(scopes)
	slices.Sort(expectedScopes)
	if !slices.Equal(scopes, expectedScopes) {
		t.Errorf("expected scopes %v, got %v", expectedScopes, scopes)
	}
}

func TestFindOrCreateRole(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	role1, err := FindOrCreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("FindOrCreateRole (create) failed: %v", err)
	}

	role2, err := FindOrCreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("FindOrCreateRole (find) failed: %v", err)
	}

	if role1.ID != role2.ID {
		t.Errorf("expected same role ID, got %q and %q", role1.ID, role2.ID)
	}
}

func TestCreateRole_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	_, err := CreateRole(ctx, db, "", "description", false, nil)
	if err == nil {
		t.Error("expected validation error for empty role name")
	}

	_, err = CreateRole(ctx, db, "admin", "description", false, []string{"INVALID:SCOPE"})
	if err == nil {
		t.Error("expected validation error for invalid scope format")
	}
}
```

- [ ] **Step 6: Implement Role model and operations**

Create `internal/state/role.go`:

```go
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
```

- [ ] **Step 7: Run role tests**

Run: `go test ./internal/state/ -run "TestCreateRole|TestFindOrCreateRole|TestAssignRole" -v`
Expected: PASS

- [ ] **Step 8: Write test for APIKey CRUD**

Create `internal/state/apikey_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func createTestUser(t *testing.T, db *gorm.DB) *User {
	t.Helper()
	user, err := CreateUser(t.Context(), db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	return user
}

func TestCreateAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	apiKey, cleartext, err := CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if apiKey.ID == "" {
		t.Error("expected non-empty ID")
	}
	if apiKey.Name != "test-key" {
		t.Errorf("expected name 'test-key', got %q", apiKey.Name)
	}
	if apiKey.UserID != user.ID {
		t.Errorf("expected UserID %q, got %q", user.ID, apiKey.UserID)
	}
	if !apiKey.Active {
		t.Error("expected key to be active")
	}
	if !strings.HasPrefix(cleartext, "fdo_") {
		t.Errorf("expected cleartext to start with 'fdo_', got %q", cleartext)
	}
	if len(apiKey.Prefix) != 6 {
		t.Errorf("expected prefix length 6, got %d", len(apiKey.Prefix))
	}
	if cleartext[4:10] != apiKey.Prefix {
		t.Errorf("expected prefix %q, got %q", cleartext[4:10], apiKey.Prefix)
	}
}

func TestFindAPIKeysByPrefix(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	_, cleartext, err := CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	prefix := cleartext[4:10]
	keys, err := FindAPIKeysByPrefix(ctx, db, prefix)
	if err != nil {
		t.Fatalf("FindAPIKeysByPrefix failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

func TestCreateAPIKeyWithExpiration(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	expiry := time.Now().Add(24 * time.Hour)
	apiKey, _, err := CreateAPIKey(ctx, db, "expiring-key", user.ID, nil, &expiry)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if apiKey.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestCreateAPIKeyWithScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	scopes := []string{"vouchers:read"}
	apiKey, _, err := CreateAPIKey(ctx, db, "limited-key", user.ID, scopes, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	got, err := apiKey.ScopesList()
	if err != nil {
		t.Fatalf("ScopesList failed: %v", err)
	}
	if len(got) != 1 || got[0] != "vouchers:read" {
		t.Errorf("expected scopes [vouchers:read], got %v", got)
	}
}

func TestScopesList_Corrupted(t *testing.T) {
	key := &APIKey{Scopes: "not valid json"}
	_, err := key.ScopesList()
	if err == nil {
		t.Error("expected error for corrupted scopes JSON")
	}
}

func TestScopesList_Empty(t *testing.T) {
	key := &APIKey{Scopes: ""}
	got, err := key.ScopesList()
	if err != nil {
		t.Fatalf("ScopesList failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCreateAPIKey_Validation(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	tests := []struct {
		name   string
		kname  string
		scopes []string
	}{
		{"empty name", "", nil},
		{"invalid scope chars", "key", []string{"INVALID:SCOPE"}},
		{"scope with spaces", "key", []string{"has space"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CreateAPIKey(ctx, db, tt.kname, user.ID, tt.scopes, nil)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestCreateAPIKeyWithScopes_ScopeRestricted(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	user := createTestUser(t, db)

	restricted, _, err := CreateAPIKey(ctx, db, "restricted", user.ID, []string{"vouchers:read"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if !restricted.ScopeRestricted {
		t.Error("expected ScopeRestricted to be true for key with scopes")
	}

	unrestricted, _, err := CreateAPIKey(ctx, db, "unrestricted", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if unrestricted.ScopeRestricted {
		t.Error("expected ScopeRestricted to be false for key without scopes")
	}
}
```

- [ ] **Step 9: Implement APIKey model and operations**

Create `internal/state/apikey.go`:

```go
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
```

- [ ] **Step 10: Implement InitAuthDB**

Create `internal/state/auth.go`:

```go
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
```

- [ ] **Step 11: Write test for InitAuthDB**

Create `internal/state/auth_test.go`:

Note: `setupAuthTestDB` is defined in `user_test.go` (same package). The existing `rvblob_test.go` already defines a `setupTestDB` in this package, so the auth tests use a distinct name to avoid collision. These tests reuse `setupAuthTestDB` directly — no redefinition needed.

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import "testing"

func TestInitAuthDB(t *testing.T) {
	db := setupAuthTestDB(t)

	for _, table := range []string{"users", "roles", "role_scopes", "user_roles", "api_keys"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestInitAuthDB_Idempotent(t *testing.T) {
	db := setupAuthTestDB(t)

	if err := InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("InitAuthDB (second call) failed: %v", err)
	}
}
```

- [ ] **Step 12: Tidy module dependencies**

Run: `go mod tidy`

Note: `github.com/google/uuid` is already an indirect dependency in `go.mod`. The new direct imports will promote it to a direct dependency automatically; `go mod tidy` handles this.

- [ ] **Step 13: Run all auth state tests**

Run: `go test ./internal/state/ -run "TestCreateUser|TestGetUser|TestUserCount|TestCreateRole|TestFindOrCreate|TestAssignRole|TestCreateAPIKey|TestFindAPIKeys|TestInitAuthDB" -v`
Expected: ALL PASS

- [ ] **Step 14: Write cascade tests**

Add to `internal/state/auth_test.go` (after `TestInitAuthDB_Idempotent`):

```go
func TestCascadeDeleteUser_RemovesAPIKeys(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, _, err := CreateAPIKey(ctx, db, "key1", user.ID, nil, nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, _, err := CreateAPIKey(ctx, db, "key2", user.ID, nil, nil); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	var count int64
	db.Model(&APIKey{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 API keys, got %d", count)
	}

	if err := db.Delete(&User{}, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

	db.Model(&APIKey{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 API keys after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteUser_RemovesUserRoles(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser: %v", err)
	}

	var count int64
	db.Model(&UserRole{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 user_role, got %d", count)
	}

	if err := db.Delete(&User{}, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}

	db.Model(&UserRole{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 user_roles after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteRole_RemovesRoleScopes(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	role, err := CreateRole(ctx, db, "admin", "Full access", true, []string{"vouchers:read", "vouchers:write"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	var count int64
	db.Model(&RoleScope{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 role_scopes, got %d", count)
	}

	if err := db.Delete(&Role{}, "id = ?", role.ID).Error; err != nil {
		t.Fatalf("delete role: %v", err)
	}

	db.Model(&RoleScope{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 role_scopes after cascade delete, got %d", count)
	}
}

func TestCascadeDeleteRole_RemovesUserRoles(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	user, err := CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	role, err := CreateRole(ctx, db, "operator", "Ops access", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser: %v", err)
	}

	var count int64
	db.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 user_role, got %d", count)
	}

	if err := db.Delete(&Role{}, "id = ?", role.ID).Error; err != nil {
		t.Fatalf("delete role: %v", err)
	}

	db.Model(&UserRole{}).Where("role_id = ?", role.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 user_roles after cascade delete of role, got %d", count)
	}
}
```

- [ ] **Step 15: Run cascade tests**

Run: `go test ./internal/state/ -run "TestCascadeDelete" -v`
Expected: ALL PASS

- [ ] **Step 16: Commit**

```bash
git add internal/state/user.go internal/state/role.go internal/state/apikey.go internal/state/auth.go internal/state/user_test.go internal/state/role_test.go internal/state/apikey_test.go internal/state/auth_test.go
git commit -m "feat: add database models for users, roles, and API keys"
```

---

### Task 2: Auth Identity and Authenticator Interface

**Files:**
- Create: `internal/auth/identity.go`
- Create: `internal/auth/authenticator.go`
- Create: `internal/auth/identity_test.go`

**Interfaces:**
- Consumes: nothing (standalone types)
- Produces:
  - `type Identity struct` — unexported fields (subject, name, authMethod, roles, scopes, metadata) with copy-returning accessor methods
  - `func NewIdentity(subject, name, authMethod string, roles, scopes []string, metadata map[string]string) *Identity` — constructor that deep-copies slices and maps
  - `func (i *Identity) Subject() string`, `Name() string`, `AuthMethod() string`, `Roles() []string`, `Scopes() []string`, `Metadata() map[string]string` — copy-returning accessors
  - `func (i *Identity) HasAllScopes(required []string) bool`
  - `func IdentityFromContext(ctx context.Context) (*Identity, bool)` — comma-ok pattern
  - `func ContextWithIdentity(ctx context.Context, id *Identity) context.Context`
  - `type Authenticator interface` — `Name() string`, `Authenticate(ctx, r) (*Identity, error)` (documented return value semantics)
  - `var ErrInvalidCredentials` — sentinel error (expired keys also return this to avoid leaking key validity)

- [ ] **Step 1: Write test for Identity scope checking**

Create `internal/auth/identity_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"testing"
)

func TestHasAllScopes(t *testing.T) {
	id := NewIdentity("", "", "", nil, []string{"vouchers:read", "vouchers:write", "vouchers:delete", "device-ca:read"}, nil)

	tests := []struct {
		name     string
		required []string
		want     bool
	}{
		{"single present scope", []string{"vouchers:read"}, true},
		{"multiple present scopes", []string{"vouchers:read", "vouchers:write"}, true},
		{"all present scopes", []string{"vouchers:delete"}, true},
		{"missing scope", []string{"vouchers:extend"}, false},
		{"one present one missing", []string{"vouchers:read", "vouchers:extend"}, false},
		{"nil required", nil, true},
		{"empty required", []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := id.HasAllScopes(tt.required)
			if got != tt.want {
				t.Errorf("HasAllScopes(%v) = %v, want %v", tt.required, got, tt.want)
			}
		})
	}
}

func TestIdentityContext(t *testing.T) {
	id := NewIdentity("user-123", "admin", "", nil, nil, nil)
	ctx := ContextWithIdentity(context.Background(), id)

	found, ok := IdentityFromContext(ctx)
	if !ok || found == nil {
		t.Fatal("expected identity in context, got nil")
	}
	if found.Subject() != "user-123" {
		t.Errorf("expected subject 'user-123', got %q", found.Subject())
	}

	empty, ok := IdentityFromContext(context.Background())
	if ok || empty != nil {
		t.Errorf("expected no identity from empty context, got %v", empty)
	}

	nilCtx := ContextWithIdentity(context.Background(), nil)
	nilID, nilOK := IdentityFromContext(nilCtx)
	if nilOK || nilID != nil {
		t.Errorf("expected (nil, false) after storing nil identity, got (%v, %v)", nilID, nilOK)
	}
}
```

- [ ] **Step 2: Implement Identity and Authenticator**

Create `internal/auth/identity.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"errors"
)

type contextKey struct{}

var ErrInvalidCredentials = errors.New("invalid credentials")

type Identity struct {
	subject    string
	name       string
	authMethod string
	roles      []string
	scopes     []string
	metadata   map[string]string
}

func NewIdentity(subject, name, authMethod string, roles, scopes []string, metadata map[string]string) *Identity {
	id := &Identity{
		subject:    subject,
		name:       name,
		authMethod: authMethod,
	}
	if roles != nil {
		id.roles = make([]string, len(roles))
		copy(id.roles, roles)
	}
	if scopes != nil {
		id.scopes = make([]string, len(scopes))
		copy(id.scopes, scopes)
	}
	if metadata != nil {
		id.metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			id.metadata[k] = v
		}
	}
	return id
}

func (i *Identity) Subject() string    { return i.subject }
func (i *Identity) Name() string       { return i.name }
func (i *Identity) AuthMethod() string { return i.authMethod }

func (i *Identity) Roles() []string {
	out := make([]string, len(i.roles))
	copy(out, i.roles)
	return out
}

func (i *Identity) Scopes() []string {
	out := make([]string, len(i.scopes))
	copy(out, i.scopes)
	return out
}

func (i *Identity) Metadata() map[string]string {
	out := make(map[string]string, len(i.metadata))
	for k, v := range i.metadata {
		out[k] = v
	}
	return out
}

func (i *Identity) HasAllScopes(required []string) bool {
	if len(required) == 0 {
		return true
	}
	if i == nil || len(i.scopes) < len(required) {
		return false
	}
	have := make(map[string]struct{}, len(i.scopes))
	for _, s := range i.scopes {
		have[s] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			return false
		}
	}
	return true
}

func ContextWithIdentity(ctx context.Context, id *Identity) context.Context {
	if id == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, id)
}

// IdentityFromContext retrieves the authenticated identity from the context.
// Returns (nil, false) when no identity is present.
func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(contextKey{}).(*Identity)
	return id, ok
}
```

Create `internal/auth/authenticator.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"net/http"
)

// Authenticator extracts and validates credentials from an HTTP request.
//
// Authenticate returns:
//   - (*Identity, nil) on successful authentication
//   - (nil, nil) when no credentials are present (the request is unauthenticated
//     but another authenticator may handle it)
//   - (nil, error) when credentials are present but invalid or an internal error occurs
type Authenticator interface {
	Name() string
	Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/identity.go internal/auth/authenticator.go internal/auth/identity_test.go
git commit -m "feat: add auth Identity model and Authenticator interface"
```

---

### Task 3: API Key Authenticator

**Files:**
- Create: `internal/auth/apikey/authenticator.go`
- Create: `internal/auth/apikey/authenticator_test.go`

**Interfaces:**
- Consumes: `auth.Authenticator` interface, `state.FindAPIKeysByPrefix`, `state.GetUserRoles`, `state.GetUserScopes`, `state.UpdateAPIKeyLastUsed`
- Produces: `type APIKeyAuthenticator struct` implementing `auth.Authenticator`

**Logging levels for authentication flow errors** (per design spec):
- Invalid format (bad prefix/length): `DEBUG` — routine rejection, expected in normal operation
- Inactive user: `WARN` — may indicate a revoked user still attempting access, worth operator attention
- Expired key: `DEBUG` — routine rejection, expected in normal operation
- `last_used_at` update failure: `WARN` — server-side DB write failure on an otherwise successful auth path, worth operator attention but does not block the request
- No matching key: `DEBUG` — routine rejection, expected in normal operation

- [ ] **Step 1: Write test for API key authenticator**

Create `internal/auth/apikey/authenticator_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package apikey

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if err := state.InitAuthDB(t.Context(), db); err != nil {
		t.Fatalf("Failed to initialize auth database: %v", err)
	}
	return db
}

func TestAuthenticate_NoHeader(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	id, err := authn.Authenticate(r.Context(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_EmptyHeader(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "")
	id, err := authn.Authenticate(r.Context(), r)
	if id != nil {
		t.Errorf("expected nil identity for empty header, got %+v", id)
	}
	if err != nil {
		t.Errorf("expected nil error for empty header (skip to next authenticator), got %v", err)
	}
}

func TestAuthenticate_InvalidFormat(t *testing.T) {
	db := setupAuthTestDB(t)
	authn := New(db)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "bad_key")
	id, err := authn.Authenticate(r.Context(), r)
	if err == nil {
		t.Fatal("expected error for invalid key format")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_ValidKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if id.Subject() != user.ID {
		t.Errorf("expected subject %q, got %q", user.ID, id.Subject())
	}
	if id.AuthMethod() != "api-key" {
		t.Errorf("expected auth method 'api-key', got %q", id.AuthMethod())
	}
	expectedScopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete"}
	gotScopes := id.Scopes()
	slices.Sort(gotScopes)
	slices.Sort(expectedScopes)
	if !slices.Equal(gotScopes, expectedScopes) {
		t.Errorf("expected scopes %v, got %v", expectedScopes, gotScopes)
	}

	authn.Wait()
}

func TestAuthenticate_ExpiredKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	expired := time.Now().Add(-1 * time.Hour)
	_, cleartext, err := state.CreateAPIKey(ctx, db, "expired-key", user.ID, nil, &expired)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr == nil {
		t.Fatal("expected error for expired key")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_InactiveUser(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if err := db.WithContext(ctx).Model(&state.User{}).Where("id = ?", user.ID).Update("active", false).Error; err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr == nil {
		t.Fatal("expected error for inactive user")
	}
	if id != nil {
		t.Errorf("expected nil identity, got %v", id)
	}
}

func TestAuthenticate_ScopedKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "admin", "Admin", true, []string{"vouchers:read", "vouchers:write", "vouchers:delete", "device-ca:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "limited-key", user.ID, []string{"vouchers:read"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	gotScopes := id.Scopes()
	if len(gotScopes) != 1 || gotScopes[0] != "vouchers:read" {
		t.Errorf("expected scopes [vouchers:read], got %v", gotScopes)
	}

	authn.Wait()
}

func TestAuthenticate_InactiveKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "test-key", user.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if err := db.WithContext(ctx).Model(&state.APIKey{}).Where("prefix = ?", cleartext[4:10]).Update("active", false).Error; err != nil {
		t.Fatalf("Failed to deactivate key: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if id != nil {
		t.Errorf("expected nil identity for inactive key, got %+v", id)
	}
	if authErr == nil {
		t.Error("expected error for inactive key")
	}
}

func TestAuthenticate_ScopedKeyNoIntersection(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	authn := New(db)

	user, err := state.CreateUser(ctx, db, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	role, err := state.CreateRole(ctx, db, "limited", "Limited", false, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, user.ID, role.ID); err != nil {
		t.Fatalf("AssignRoleToUser failed: %v", err)
	}

	_, cleartext, err := state.CreateAPIKey(ctx, db, "no-overlap-key", user.ID, []string{"admin:special"}, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", cleartext)
	id, authErr := authn.Authenticate(r.Context(), r)
	if authErr != nil {
		t.Fatalf("Authenticate failed: %v", authErr)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if len(id.Scopes()) != 0 {
		t.Errorf("expected empty scopes (no intersection), got %v", id.Scopes())
	}

	authn.Wait()
}
```

- [ ] **Step 2: Implement API key authenticator**

Create `internal/auth/apikey/authenticator.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package apikey

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

var errInternal = errors.New("internal authentication error")

const (
	headerName     = "X-API-Key"
	keyPrefix      = "fdo_"
	minKeyLen      = 10
	prefixStartIdx = 4
	prefixEndIdx   = 10
)

var _ auth.Authenticator = (*APIKeyAuthenticator)(nil)

type APIKeyAuthenticator struct {
	db *gorm.DB
	wg sync.WaitGroup
}

func New(db *gorm.DB) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{db: db}
}

func (a *APIKeyAuthenticator) Wait() {
	a.wg.Wait()
}

func (a *APIKeyAuthenticator) Name() string { return "api-key" }

func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*auth.Identity, error) {
	key := r.Header.Get(headerName)
	if key == "" {
		return nil, nil
	}

	if !strings.HasPrefix(key, keyPrefix) || len(key) < minKeyLen {
		slog.Debug("API key format invalid")
		return nil, auth.ErrInvalidCredentials
	}

	prefix := key[prefixStartIdx:prefixEndIdx]

	candidates, err := state.FindAPIKeysByPrefix(ctx, a.db, prefix)
	if err != nil {
		slog.Error("Failed to look up API keys by prefix", "prefix", prefix, "error", err)
		return nil, errInternal
	}

	hash := sha256.Sum256([]byte(key))

	for _, candidate := range candidates {
		if len(candidate.HashedKey) != len(hash) || subtle.ConstantTimeCompare(candidate.HashedKey, hash[:]) != 1 {
			continue
		}

		if candidate.ExpiresAt != nil && candidate.ExpiresAt.Before(time.Now()) {
			slog.Debug("API key expired", "prefix", prefix)
			return nil, auth.ErrInvalidCredentials
		}

		user, err := state.GetUserByID(ctx, a.db, candidate.UserID)
		if err != nil {
			slog.Error("Failed to look up API key owner", "user_id", candidate.UserID, "error", err)
			return nil, errInternal
		}
		if !user.Active {
			slog.Warn("API key authentication rejected: user inactive", "user_id", candidate.UserID, "prefix", prefix)
			return nil, auth.ErrInvalidCredentials
		}

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			state.UpdateAPIKeyLastUsed(bgCtx, a.db, candidate.ID)
		}()

		scopes, err := a.resolveScopes(ctx, &candidate)
		if err != nil {
			slog.Error("Failed to resolve scopes for API key", "api_key_id", candidate.ID, "error", err)
			return nil, errInternal
		}

		roles, err := state.GetUserRoles(ctx, a.db, candidate.UserID)
		if err != nil {
			slog.Error("Failed to look up user roles", "user_id", candidate.UserID, "error", err)
			return nil, errInternal
		}
		roleNames := make([]string, 0, len(roles))
		for _, r := range roles {
			roleNames = append(roleNames, r.Name)
		}

		return auth.NewIdentity(
			candidate.UserID,
			user.Name,
			"api-key",
			roleNames,
			scopes,
			map[string]string{"api_key_prefix": prefix, "api_key_name": candidate.Name},
		), nil
	}

	slog.Debug("API key authentication failed: no matching key", "prefix", prefix)
	return nil, auth.ErrInvalidCredentials
}

func (a *APIKeyAuthenticator) resolveScopes(ctx context.Context, apiKey *state.APIKey) ([]string, error) {
	userScopes, err := state.GetUserScopes(ctx, a.db, apiKey.UserID)
	if err != nil {
		return nil, err
	}

	if !apiKey.ScopeRestricted {
		return userScopes, nil
	}

	keyScopes, err := apiKey.ScopesList()
	if err != nil {
		return nil, err
	}
	if len(keyScopes) == 0 {
		return nil, nil
	}

	userScopeSet := make(map[string]struct{}, len(userScopes))
	for _, s := range userScopes {
		userScopeSet[s] = struct{}{}
	}

	effective := make([]string, 0, len(keyScopes))
	for _, s := range keyScopes {
		if _, ok := userScopeSet[s]; ok {
			effective = append(effective, s)
		}
	}
	return effective, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/apikey/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/apikey/authenticator.go internal/auth/apikey/authenticator_test.go
git commit -m "feat: add API key authenticator"
```

---

### Task 4: AuthN and AuthZ Middleware

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/middleware_test.go`

**Interfaces:**
- Consumes: `auth.Authenticator` interface, `auth.IdentityFromContext`, `auth.ContextWithIdentity`
- Produces:
  - `func AuthNMiddleware(authenticators []Authenticator, excludedPaths []string) func(http.Handler) http.Handler`
  - `func AuthZMiddleware(routeScopes map[string][]string) func(http.Handler) http.Handler`

- [ ] **Step 1: Write tests for AuthN middleware**

Create `internal/auth/middleware_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockAuthenticator struct {
	name     string
	identity *Identity
	err      error
}

func (m *mockAuthenticator) Name() string { return m.name }
func (m *mockAuthenticator) Authenticate(_ context.Context, _ *http.Request) (*Identity, error) {
	return m.identity, m.err
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d", w.Code, want)
	}
}

func TestAuthNMiddleware_ExcludedPath(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/health"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathExactMatch(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/health"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/health-check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_ExcludedPathPrefixMatch(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docs/index.html", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathTrailingSlash(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docs/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathNoFalsePrefix(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docsextra", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_NoAuthenticators(t *testing.T) {
	mw := AuthNMiddleware(nil, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_SuccessfulAuth(t *testing.T) {
	id := NewIdentity("user-1", "", "", nil, []string{"vouchers:read"}, nil)
	authn := &mockAuthenticator{name: "mock", identity: id}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_InvalidCredentials(t *testing.T) {
	authn := &mockAuthenticator{name: "mock", err: ErrInvalidCredentials}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_InternalError(t *testing.T) {
	authn := &mockAuthenticator{name: "mock", err: errors.New("database connection failed")}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestAuthNMiddleware_FallThrough(t *testing.T) {
	skip := &mockAuthenticator{name: "skip", identity: nil, err: nil}
	id := NewIdentity("user-1", "", "", nil, nil, nil)
	match := &mockAuthenticator{name: "match", identity: id}
	mw := AuthNMiddleware([]Authenticator{skip, match}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_Allowed(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers": {"vouchers:read"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{"vouchers:read", "vouchers:write", "vouchers:delete"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_Forbidden(t *testing.T) {
	routeScopes := map[string][]string{
		"DELETE /vouchers/{guid}": {"vouchers:delete"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{"vouchers:read"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodDelete, "/vouchers/abc123", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusForbidden)
}

func TestAuthZMiddleware_NoScopesRequired(t *testing.T) {
	mw := AuthZMiddleware(map[string][]string{})

	id := NewIdentity("", "", "", nil, []string{}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/unprotected", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_DoubleSlashNormalized(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers/{guid}": {"vouchers:read"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers//abc123", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusForbidden)
}
```

- [ ] **Step 2: Implement AuthN and AuthZ middleware**

Create `internal/auth/middleware.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

var defaultExcludedPaths = []string{
	"/fdo/101/msg/",   // prefix match (trailing /)
	"/fdo/200/msg/",   // prefix match (trailing /)
	"/health",         // exact match
	"/api/docs/",      // prefix match (trailing /)
	"/api/openapi.json", // exact match
}

func AuthNMiddleware(authenticators []Authenticator, excludedPaths []string) func(http.Handler) http.Handler {
	allExcluded := make([]string, 0, len(defaultExcludedPaths)+len(excludedPaths))
	allExcluded = append(allExcluded, defaultExcludedPaths...)
	allExcluded = append(allExcluded, excludedPaths...)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExcluded(path.Clean(r.URL.Path), allExcluded) {
				next.ServeHTTP(w, r)
				return
			}

			for _, authn := range authenticators {
				identity, err := authn.Authenticate(r.Context(), r)
				if err != nil {
					if errors.Is(err, ErrInvalidCredentials) {
						slog.Debug("Authentication failed", "authenticator", authn.Name(), "error", err)
						writeAuthError(w, http.StatusUnauthorized, "authentication failed")
					} else {
						slog.Error("Internal authentication error", "authenticator", authn.Name(), "error", err)
						writeAuthError(w, http.StatusInternalServerError, "internal server error")
					}
					return
				}
				if identity != nil {
					ctx := ContextWithIdentity(r.Context(), identity)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			writeAuthError(w, http.StatusUnauthorized, "authentication required")
		})
	}
}

func AuthZMiddleware(routeScopes map[string][]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, _ := IdentityFromContext(r.Context())
			if identity == nil {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			required := lookupRequiredScopes(r.Method, path.Clean(r.URL.Path), routeScopes)
			if len(required) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if !identity.HasAllScopes(required) {
				slog.Debug("Authorization denied",
					"subject", identity.Subject(),
					"required", required,
					"have", identity.Scopes(),
					"path", r.URL.Path,
				)
				writeAuthError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isExcluded(cleanedPath string, excluded []string) bool {
	for _, entry := range excluded {
		if strings.HasSuffix(entry, "/") {
			trimmed := strings.TrimSuffix(entry, "/")
			if cleanedPath == trimmed || strings.HasPrefix(cleanedPath, entry) {
				return true
			}
		} else {
			if cleanedPath == entry {
				return true
			}
		}
	}
	return false
}

func lookupRequiredScopes(method, path string, routeScopes map[string][]string) []string {
	for pattern, scopes := range routeScopes {
		if matchRoute(method, path, pattern) {
			return scopes
		}
	}
	return nil
}

func matchRoute(method, path, pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return false
	}
	patternMethod, patternPath := parts[0], parts[1]

	if method != patternMethod {
		return false
	}

	return matchPath(path, patternPath)
}

func matchPath(path, pattern string) bool {
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(pathParts) != len(patternParts) {
		return false
	}

	for i, pp := range patternParts {
		if strings.HasPrefix(pp, "{") && strings.HasSuffix(pp, "}") {
			continue
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}

type authErrorResponse struct {
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(authErrorResponse{Message: message})
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat: add AuthN and AuthZ middleware"
```

---

### Task 5: OpenAPI Scope Parser

**Files:**
- Create: `internal/auth/scopes.go`
- Create: `internal/auth/scopes_test.go`

**Interfaces:**
- Consumes: `github.com/getkin/kin-openapi/openapi3` (existing dependency)
- Produces: `func ParseRouteScopes(specJSON []byte) (map[string][]string, error)`

- [ ] **Step 1: Write tests for scope parsing**

Create `internal/auth/scopes_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"slices"
	"testing"
)

func TestParseRouteScopes(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/vouchers": {
				"get": {
					"operationId": "ListVouchers",
					"x-required-scopes": ["vouchers:read"],
					"responses": {"200": {"description": "OK"}}
				},
				"post": {
					"operationId": "ImportVouchers",
					"x-required-scopes": ["vouchers:write"],
					"responses": {"201": {"description": "Created"}}
				}
			},
			"/api/v2/vouchers/{guid}": {
				"delete": {
					"operationId": "DeleteVoucher",
					"x-required-scopes": ["vouchers:delete"],
					"responses": {"204": {"description": "Deleted"}}
				}
			},
			"/health": {
				"get": {
					"operationId": "Health",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}

	tests := []struct {
		key  string
		want []string
	}{
		{"GET /vouchers", []string{"vouchers:read"}},
		{"POST /vouchers", []string{"vouchers:write"}},
		{"DELETE /vouchers/{guid}", []string{"vouchers:delete"}},
	}
	for _, tt := range tests {
		got, ok := scopes[tt.key]
		if !ok {
			t.Errorf("missing key %q", tt.key)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("scopes[%q] = %v, want %v", tt.key, got, tt.want)
		}
	}
	if _, ok := scopes["GET /health"]; ok {
		t.Error("health endpoint should not have scopes")
	}
}

func TestParseRouteScopes_NoExtension(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/resource": {
				"get": {
					"operationId": "GetResource",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}
	if len(scopes) != 0 {
		t.Errorf("got %d scope entries, want 0", len(scopes))
	}
}

func TestParseRouteScopes_MultipleScopes(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/admin": {
				"post": {
					"operationId": "AdminAction",
					"x-required-scopes": ["vouchers:read", "vouchers:delete"],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}
	got := scopes["POST /admin"]
	want := []string{"vouchers:read", "vouchers:delete"}
	if !slices.Equal(got, want) {
		t.Errorf("scopes[POST /admin] = %v, want %v", got, want)
	}
}

func TestCollectAllScopes(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers":          {"vouchers:read"},
		"POST /vouchers":         {"vouchers:write"},
		"DELETE /vouchers/{guid}": {"vouchers:delete"},
		"GET /devices":           {"devices:read"},
	}
	got := CollectAllScopes(routeScopes)
	want := []string{"devices:read", "vouchers:delete", "vouchers:read", "vouchers:write"}
	if !slices.Equal(got, want) {
		t.Errorf("CollectAllScopes = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Implement scope parser**

Create `internal/auth/scopes.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const requiredScopesExtension = "x-required-scopes"

func ParseRouteScopes(specJSON []byte) (map[string][]string, error) {
	spec, err := openapi3.NewLoader().LoadFromData(specJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	routeScopes := make(map[string][]string)

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			ext, ok := op.Extensions[requiredScopesExtension]
			if !ok {
				continue
			}

			scopeSlice, err := parseStringSliceExtension(ext)
			if err != nil {
				return nil, fmt.Errorf("invalid %s on %s %s: %w", requiredScopesExtension, method, path, err)
			}

			if len(scopeSlice) == 0 {
				continue
			}

			normalizedPath := path
			if stripped, ok := strings.CutPrefix(path, "/api/v2"); ok {
				normalizedPath = stripped
			}

			key := method + " " + normalizedPath
			routeScopes[key] = scopeSlice
			slog.Debug("Registered route scope", "route", key, "scopes", scopeSlice)
		}
	}

	return routeScopes, nil
}

func CollectAllScopes(routeScopes map[string][]string) []string {
	seen := make(map[string]struct{})
	for _, scopes := range routeScopes {
		for _, s := range scopes {
			seen[s] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	slices.Sort(result)
	return result
}

func parseStringSliceExtension(ext interface{}) ([]string, error) {
	raw, ok := ext.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", ext)
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string in array, got %T", v)
		}
		result = append(result, s)
	}
	return result, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/ -run "TestParseRouteScopes" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/scopes.go internal/auth/scopes_test.go
git commit -m "feat: add OpenAPI x-required-scopes parser"
```

---

### Task 6: Auth Configuration and Seed Logic

**Files:**
- Create: `internal/config/auth.go`
- Create: `internal/config/auth_test.go`
- Create: `internal/state/seed.go`
- Create: `internal/state/seed_test.go`
- Modify: `internal/config/server.go` — add `Auth AuthConfig` field

**Interfaces:**
- Consumes: `state.CreateUser`, `state.CreateRole`, `state.AssignRoleToUser`, `state.CreateAPIKey`, `state.UserCount`
- Produces:
  - `type AuthConfig struct` — Enabled, ExcludedPaths, Mechanisms, Seed
  - `func (a *AuthConfig) Validate() error`
  - `func SeedAuth(ctx context.Context, db *gorm.DB, cfg config.SeedConfig, serverScopes []string, force bool) (string, error)` — returns API key cleartext; all operations are atomic (single transaction)

- [ ] **Step 1: Write test for AuthConfig validation**

Create `internal/config/auth_test.go`:

```go
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
```

- [ ] **Step 2: Implement AuthConfig**

Create `internal/config/auth.go`:

```go
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
```

- [ ] **Step 3: Update ServerConfig to include Auth**

Modify `internal/config/server.go` — add `Auth AuthConfig` field (keep existing SPDX year):

```go
type ServerConfig struct {
	Log  LogConfig      `mapstructure:"log"`
	DB   DatabaseConfig `mapstructure:"db"`
	HTTP HTTPConfig     `mapstructure:"http"`
	Auth AuthConfig     `mapstructure:"auth"`
}
```

- [ ] **Step 4: Write test for seed logic**

Create `internal/state/seed_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/config"
)

func TestSeedAuth_CreatesAdminUserAndKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	scopes := []string{"vouchers:read", "vouchers:write", "vouchers:delete", "vouchers:extend"}
	cfg := config.SeedConfig{
		Admin: &config.SeedAdminConfig{
			Name:  "admin",
			Email: "admin@example.com",
		},
	}

	apiKey, err := SeedAuth(ctx, db, cfg, scopes, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if !strings.HasPrefix(apiKey, "fdo_") {
		t.Errorf("apiKey = %q, want fdo_ prefix", apiKey)
	}

	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}
}

func TestSeedAuth_SkipsIfUsersExist(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	if _, err := CreateUser(ctx, db, "existing", "existing@example.com"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cfg := config.SeedConfig{
		Admin: &config.SeedAdminConfig{
			Name:  "admin",
			Email: "admin@example.com",
		},
	}

	apiKey, err := SeedAuth(ctx, db, cfg, nil, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if apiKey != "" {
		t.Errorf("expected empty apiKey, got %q", apiKey)
	}
}

func TestSeedAuth_ForceCreatesWhenUsersExist(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	scopes := []string{"vouchers:read"}

	cfg1 := config.SeedConfig{
		Admin: &config.SeedAdminConfig{Name: "admin", Email: "admin@example.com"},
	}
	if _, err := SeedAuth(ctx, db, cfg1, scopes, false); err != nil {
		t.Fatalf("SeedAuth (first): %v", err)
	}

	cfg2 := config.SeedConfig{
		Admin: &config.SeedAdminConfig{Name: "admin2", Email: "admin2@example.com"},
	}
	apiKey, err := SeedAuth(ctx, db, cfg2, scopes, true)
	if err != nil {
		t.Fatalf("SeedAuth (force): %v", err)
	}
	if apiKey == "" {
		t.Error("expected non-empty apiKey with --force")
	}

	count, err := UserCount(ctx, db)
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 2 {
		t.Errorf("user count = %d, want 2", count)
	}
}

func TestSeedAuth_PresetAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()
	cfg := config.SeedConfig{
		Admin: &config.SeedAdminConfig{
			Name:   "admin",
			Email:  "admin@example.com",
			APIKey: "fdo_a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5P6",
		},
	}

	apiKey, err := SeedAuth(ctx, db, cfg, []string{"vouchers:read"}, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}
	if apiKey != cfg.Admin.APIKey {
		t.Errorf("apiKey = %q, want %q", apiKey, cfg.Admin.APIKey)
	}
}

func TestSeedAuth_InvalidPresetAPIKey(t *testing.T) {
	db := setupAuthTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name string
		key  string
	}{
		{"missing prefix", "bad_a1B2c3D4e5F6"},
		{"too short", "fdo_abc"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.SeedConfig{
				Admin: &config.SeedAdminConfig{
					Name:   "admin",
					Email:  "admin@example.com",
					APIKey: tt.key,
				},
			}
			_, err := SeedAuth(ctx, db, cfg, []string{"vouchers:read"}, true)
			if err == nil {
				t.Error("expected error for invalid preset API key")
			}
		})
	}
}
```

- [ ] **Step 5: Implement seed logic**

Create `internal/state/seed.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package state

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func SeedAuth(ctx context.Context, db *gorm.DB, cfg config.SeedConfig, serverScopes []string, force bool) (string, error) {
	if cfg.Admin == nil {
		return "", nil
	}

	if !force {
		count, err := UserCount(ctx, db)
		if err != nil {
			return "", err
		}
		if count > 0 {
			slog.Info("Users already exist, skipping auth seed")
			return "", nil
		}
	}

	slog.Info("Seeding auth database with initial admin user")

	// All seed operations run in a single transaction for atomicity.
	// FindOrCreateRole → CreateRole uses a nested transaction (GORM savepoint),
	// which SQLite supports. The outer transaction is the commit boundary.
	var cleartext string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		adminScopes := append(serverScopes[:len(serverScopes):len(serverScopes)], "auth:manage")
		adminRole, err := FindOrCreateRole(ctx, tx, "admin", "Full administrative access", true, adminScopes)
		if err != nil {
			return fmt.Errorf("failed to create admin role: %w", err)
		}

		var operatorScopes []string
		for _, s := range serverScopes {
			if strings.HasSuffix(s, ":read") || strings.HasSuffix(s, ":write") || s == "vouchers:extend" {
				operatorScopes = append(operatorScopes, s)
			}
		}
		if _, err := FindOrCreateRole(ctx, tx, "operator", "Operational access (create/modify, no delete or auth management)", true, operatorScopes); err != nil {
			return fmt.Errorf("failed to create operator role: %w", err)
		}

		user, err := CreateUser(ctx, tx, cfg.Admin.Name, cfg.Admin.Email)
		if err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}

		if err := AssignRoleToUser(ctx, tx, user.ID, adminRole.ID); err != nil {
			return fmt.Errorf("failed to assign admin role: %w", err)
		}

		if cfg.Admin.APIKey != "" {
			if err := createPresetAPIKey(ctx, tx, user.ID, cfg.Admin.APIKey); err != nil {
				return err
			}
			cleartext = cfg.Admin.APIKey
			return nil
		}

		_, key, err := CreateAPIKey(ctx, tx, "admin-seed-key", user.ID, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to create admin API key: %w", err)
		}
		cleartext = key
		return nil
	})
	if err != nil {
		return "", err
	}

	return cleartext, nil
}

// createPresetAPIKey must be called within the SeedAuth transaction after CreateUser,
// so the userID foreign key is guaranteed to exist.
func createPresetAPIKey(ctx context.Context, db *gorm.DB, userID, cleartext string) error {
	if !strings.HasPrefix(cleartext, "fdo_") || len(cleartext) < 10 {
		return fmt.Errorf("invalid preset API key: must start with 'fdo_' and be at least 10 characters")
	}

	hash := sha256.Sum256([]byte(cleartext))
	hashed := hash[:]

	prefix := cleartext[4:10]
	apiKey := &APIKey{
		ID:        uuid.New().String(),
		Prefix:    prefix,
		HashedKey: hashed,
		Name:      "admin-seed-key",
		UserID:    userID,
		Active:    true,
	}

	if err := db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return fmt.Errorf("failed to create preset API key: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Run all config and seed tests**

Run: `go test ./internal/config/ -run "TestAuthConfig" -v && go test ./internal/state/ -run "TestSeedAuth" -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/config/auth.go internal/config/auth_test.go internal/config/server.go internal/state/seed.go internal/state/seed_test.go
git commit -m "feat: add auth configuration and database seed logic"
```

---

### Task 7: OpenAPI `x-required-scopes` Annotations

**Files:**
- Modify: `api/v2/voucher/openapi.yaml` — add `x-required-scopes` to 6 operations
- Modify: `api/v2/deviceca/openapi.yaml` — add `x-required-scopes` to 4 operations
- Modify: `api/v2/rvinfo/openapi.yaml` — add `x-required-scopes` to 3 operations (read, write, delete)
- Modify: `api/v2/rvto2addr/openapi.yaml` — add `x-required-scopes` to 3 operations
- Modify: `api/v2/device/openapi.yaml` — add `x-required-scopes` to 1 operation

**Interfaces:**
- Consumes: Existing OpenAPI YAML files
- Produces: Same files with `x-required-scopes` annotations on each operation

- [ ] **Step 1: Add `x-required-scopes` to voucher API**

In `api/v2/voucher/openapi.yaml`, add to each operation right after `operationId`:

- `ListOwnershipVouchers` → `x-required-scopes: [vouchers:read]`
- `ImportOwnershipVouchers` → `x-required-scopes: [vouchers:write]`
- `GetOwnershipVoucherByGuid` → `x-required-scopes: [vouchers:read]`
- `DeleteOwnershipVoucher` → `x-required-scopes: [vouchers:delete]`
- `ExtendOwnershipVoucher` → `x-required-scopes: [vouchers:extend]`
- `VerifyOwnership` → `x-required-scopes: [vouchers:write]`

- [ ] **Step 2: Add `x-required-scopes` to deviceca API**

In `api/v2/deviceca/openapi.yaml`:

- `ListTrustedDeviceCACerts` → `x-required-scopes: [device-ca:read]`
- `ImportTrustedDeviceCACerts` → `x-required-scopes: [device-ca:write]`
- `GetTrustedDeviceCACertByFingerprint` → `x-required-scopes: [device-ca:read]`
- `DeleteTrustedDeviceCACert` → `x-required-scopes: [device-ca:delete]`

- [ ] **Step 3: Add `x-required-scopes` to rvinfo API**

In `api/v2/rvinfo/openapi.yaml`:

- `GetRendezvousInfo` → `x-required-scopes: [rvinfo:read]`
- `UpdateRendezvousInfo` → `x-required-scopes: [rvinfo:write]`
- `DeleteRendezvousInfo` → `x-required-scopes: [rvinfo:delete]`

- [ ] **Step 4: Add `x-required-scopes` to rvto2addr API**

In `api/v2/rvto2addr/openapi.yaml`:

- `GetRVTO2Addr` → `x-required-scopes: [rvto2addr:read]`
- `UpdateRVTO2Addr` → `x-required-scopes: [rvto2addr:write]`
- `DeleteRVTO2Addr` → `x-required-scopes: [rvto2addr:delete]`

- [ ] **Step 5: Add `x-required-scopes` to device API**

In `api/v2/device/openapi.yaml`:

- `ListDevices` → `x-required-scopes: [devices:read]`

- [ ] **Step 6: Regenerate merged OpenAPI JSON specs**

Run: `go generate ./api/v2/...`

- [ ] **Step 7: Commit**

```bash
git add api/v2/
git commit -m "feat: add x-required-scopes annotations to OpenAPI specs"
```

---

### Task 8: Wire Auth into Server Startup

**Files:**
- Modify: `api/v2/manufacturer/handler.go` — add auth middleware wiring
- Modify: `api/v2/owner/handler.go` — add auth middleware wiring
- Modify: `api/v2/rendezvous/handler.go` — add auth middleware wiring
- Modify: `api/v2/owner/backward_compatibility_test.go` — update `Handler()` call sites
- Create: `api/v2/manufacturer/auth_integration_test.go` — end-to-end auth tests
- Modify: `internal/server/manufacturing.go` — call `InitAuthDB`, `SeedAuth`, pass auth config, graceful shutdown
- Modify: `internal/server/owner.go` — same
- Modify: `internal/server/rendezvous.go` — same

**Interfaces:**
- Consumes: `auth.AuthNMiddleware`, `auth.AuthZMiddleware`, `auth.ParseRouteScopes`, `apikey.New`, `state.InitAuthDB`, `state.SeedAuth`, `config.AuthConfig`
- Produces: Complete end-to-end auth pipeline wired into all three server types

- [ ] **Step 1: Define scope registries per server type**

Each handler package's embedded `openAPISpecJSON` must be exported (renamed to `OpenAPISpecJSON`) so the CLI `init-admin` command and server startup code can access it cross-package via `auth.ParseRouteScopes`. In each handler file (`api/v2/manufacturer/handler.go`, `api/v2/owner/handler.go`, `api/v2/rendezvous/handler.go`):

1. Rename `var openAPISpecJSON []byte` to `var OpenAPISpecJSON []byte`
2. Update all internal references within the same file (e.g., `OpenAPIValidationMiddleware(OpenAPISpecJSON)`, `ServeOpenAPI(..., OpenAPISpecJSON)`)
3. In server startup code (`internal/server/*.go`), reference as `manufacturer.OpenAPISpecJSON`, `owner.OpenAPISpecJSON`, `rendezvous.OpenAPISpecJSON`

For the manufacturer handler (`api/v2/manufacturer/handler.go`), also add the auth middleware wrapping for the V2 management API mux:

```go
import (
	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/auth/apikey"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
)
```

The `Handler()` method accepts pre-built authenticators and route scopes from the server startup code (rather than creating them internally). This ensures the server retains a reference to authenticators like `APIKeyAuthenticator` so it can call `Wait()` during graceful shutdown to drain background `last_used_at` updates.

Update the `Handler()` method signature (keeps returning `http.Handler` — no error return needed because `AuthNMiddleware` and `AuthZMiddleware` are pure closures that cannot fail at construction time):

```go
func (m *Manufacturer) Handler(authCfg config.AuthConfig, authenticators []auth.Authenticator, routeScopes map[string][]string) http.Handler {
```

Inside `Handler()`, after building `mgmtAPIServeMuxV2` and before wrapping with validation middleware, add:

```go
var mgmtHandlerV2 http.Handler
if authCfg.Enabled {
    authN := auth.AuthNMiddleware(authenticators, authCfg.ExcludedPaths)
    authZ := auth.AuthZMiddleware(routeScopes)

    mgmtHandlerV2 = middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
        middleware.BodySizeMiddleware(10<<20,
            authN(authZ(validationMiddleware(mgmtAPIServeMuxV2)))))
} else {
    mgmtHandlerV2 = middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
        middleware.BodySizeMiddleware(10<<20,
            validationMiddleware(mgmtAPIServeMuxV2)))
}
```

Apply the same pattern to `api/v2/owner/handler.go` and `api/v2/rendezvous/handler.go`.

**Security-critical:** The V1 management API (`/api/v1/`) MUST also be wrapped with both `authN` and `authZ` middleware when auth is enabled. Without `authZ`, any authenticated user — even one with a restricted `vouchers:read`-only key — could execute all V1 operations including writes and deletes.

This works with minimal additional scope mapping because V1 routes are served behind `http.StripPrefix("/api/v1", ...)`. By the time AuthZ sees the request, most paths match the same patterns as V2 (e.g., `GET /vouchers`, `PUT /rvinfo`). The manufacturer needs one V1-specific override (`POST /rvinfo`), the owner needs 8 overrides (for `/owner/` prefix paths), and the rendezvous needs none.

The existing V1 handler already wraps the mux with `RateLimitMiddleware(BodySizeMiddleware(...))`. When auth is enabled, insert `authN(authZ(...))` inside the existing chain — do not replace it:

```go
var mgmtHandlerV1 http.Handler
if authCfg.Enabled {
    mgmtHandlerV1 = middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
        middleware.BodySizeMiddleware(10<<20,
            authN(authZ(mgmtAPIServeMuxV1))))
} else {
    mgmtHandlerV1 = middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
        middleware.BodySizeMiddleware(10<<20, mgmtAPIServeMuxV1))
}
```

- [ ] **Step 2: Update server startup to init auth DB and seed**

In `internal/server/manufacturing.go`, update `NewManufacturingServer` after DB init:

```go
var routeScopes map[string][]string
if config.Auth.Enabled {
    if err := state.InitAuthDB(ctx, gormDB); err != nil {
        return nil, fmt.Errorf("failed to initialize auth database: %w", err)
    }

    var err error
    routeScopes, err = auth.ParseRouteScopes(manufacturer.OpenAPISpecJSON)
    if err != nil {
        return nil, fmt.Errorf("failed to parse route scopes: %w", err)
    }

    if config.Auth.Seed != nil {
        serverScopes := auth.CollectAllScopes(routeScopes)
        apiKey, err := state.SeedAuth(ctx, gormDB, *config.Auth.Seed, serverScopes, false)
        if err != nil {
            return nil, fmt.Errorf("failed to seed auth: %w", err)
        }
        if apiKey != "" {
            fmt.Fprintf(os.Stderr, "WARNING: Initial admin API key generated — save this key, it will not be shown again:\n%s\n", apiKey)
        }
    }
}
```

Build the authenticator chain at the server level and pass it to `Handler()`. Retain a reference to the `APIKeyAuthenticator` so its `Wait()` can be called during graceful shutdown:

```go
var authenticators []auth.Authenticator
var apikeyAuthn *apikey.APIKeyAuthenticator
if config.Auth.Enabled {
    if config.Auth.Mechanisms.APIKey != nil && config.Auth.Mechanisms.APIKey.Enabled {
        apikeyAuthn = apikey.New(gormDB)
        authenticators = append(authenticators, apikeyAuthn)
    }
}

httpHandler := mfg.Handler(config.Auth, authenticators, routeScopes)
```

During graceful shutdown (after `httpServer.Shutdown(ctx)` returns), drain background auth work:

```go
if apikeyAuthn != nil {
    apikeyAuthn.Wait()
}
```

Apply the same pattern to owner and rendezvous.

The V1-specific scope overrides below are added in the **server startup code** (same files as above), after `ParseRouteScopes` returns and before passing `routeScopes` to `Handler()`.

For the **manufacturer** server (`internal/server/manufacturing.go`), add a V1-specific scope override. V1 registers both `POST /rvinfo` (CreateRendezvousInfo) and `PUT /rvinfo` (UpdateRendezvousInfo), but V2 only uses `PUT /rvinfo`. The V2-derived scope map already covers `PUT /rvinfo` → `rvinfo:write`, but `POST /rvinfo` has no entry and would bypass AuthZ. The remaining manufacturer V1 routes (`GET/POST /vouchers`, `GET /vouchers/{guid}`, `GET /rvinfo`) already match V2 patterns. Note: V1 has no `DELETE` endpoint for vouchers on the manufacturer server.

```go
if config.Auth.Enabled {
    routeScopes["POST /rvinfo"] = []string{"rvinfo:write"}
}
```

For the **owner** server (`internal/server/owner.go`), after parsing route scopes, add V1-specific entries because the owner's V1 routes register under an `/owner/` prefix (e.g., `/owner/vouchers`, `/owner/resell/{guid}`). After `StripPrefix("/api/v1")`, these 8 paths don't match the V2-derived scope map keys. Note: V1 does not have a DELETE endpoint for vouchers on the owner server, so no `DELETE /owner/vouchers/{guid}` entry is needed. The V1 `device-ca` routes (4 routes) already match V2 patterns directly — no overrides needed. Add:

```go
if config.Auth.Enabled {
    ownerV1Scopes := map[string][]string{
        "GET /owner/vouchers":           {"vouchers:read"},
        "POST /owner/vouchers":          {"vouchers:write"},
        "GET /owner/vouchers/{guid}":    {"vouchers:read"},
        "POST /owner/resell/{guid}":     {"vouchers:extend"},
        "GET /owner/devices":            {"devices:read"},
        "GET /owner/redirect":           {"rvto2addr:read"},
        "POST /owner/redirect":          {"rvto2addr:write"},
        "PUT /owner/redirect":           {"rvto2addr:write"},
    }
    for k, v := range ownerV1Scopes {
        routeScopes[k] = v
    }
}
```

- [ ] **Step 3: Add auth validation to manufacturing config Validate()**

In `internal/config/manufacturer.go`, add at the end of `Validate()`:

```go
if err := m.ServerConfig.Auth.Validate(); err != nil {
    return err
}
```

Do the same in `internal/config/owner.go` and `internal/config/rendezvous.go`.

- [ ] **Step 4: Update Handler() signature and all call sites**

The `Handler()` signature gains three new parameters (`authCfg`, `authenticators`, `routeScopes`) but keeps returning `http.Handler` — no error return needed because `AuthNMiddleware` and `AuthZMiddleware` are pure closures that cannot fail at construction time.

All 5 existing call sites must be updated:

| File | Current call | Updated call |
|---|---|---|
| `internal/server/manufacturing.go:78` | `httpHandler := mfg.Handler()` | `httpHandler := mfg.Handler(config.Auth, authenticators, routeScopes)` |
| `internal/server/owner.go:178` | `Handler: s.handler.Handler()` | `Handler: s.handler.Handler(config.Auth, authenticators, routeScopes)` |
| `internal/server/rendezvous.go:53` | `handler := rv.Handler()` | `handler := rv.Handler(config.Auth, authenticators, routeScopes)` |
| `api/v2/owner/backward_compatibility_test.go:50` | `mux := handler.Handler()` | `mux := handler.Handler(config.AuthConfig{}, nil, nil)` |
| `api/v2/owner/backward_compatibility_test.go:163` | `mux := handler.Handler()` | `mux := handler.Handler(config.AuthConfig{}, nil, nil)` |

For test call sites, pass a zero-value `config.AuthConfig{}`, `nil` authenticators, and `nil` route scopes (which produces the existing no-auth behavior).

- [ ] **Step 5: Run existing tests to verify backward compatibility**

Run: `go test ./... 2>&1 | tail -30`
Expected: ALL existing tests PASS (auth disabled by default)

- [ ] **Step 6: Verify graceful shutdown wiring**

Check the existing shutdown flow in `internal/server/manufacturing.go` (and owner/rendezvous). Identify where `httpServer.Shutdown(ctx)` is called and insert `apikeyAuthn.Wait()` immediately after. The exact insertion point depends on whether the server uses a signal handler goroutine, a `defer`, or a blocking shutdown sequence. Verify by reading the existing code — do not assume the pattern.

The `Wait()` call must happen after `Shutdown(ctx)` returns (so no new requests arrive) but before the process exits (so in-flight `last_used_at` goroutines complete). If the server uses `defer` for cleanup, add `apikeyAuthn.Wait()` as an earlier defer (defers execute LIFO, so it runs after shutdown).

- [ ] **Step 7: Write integration test for auth pipeline**

Create `api/v2/manufacturer/auth_integration_test.go`:

```go
// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package manufacturer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/auth"
	"github.com/fido-device-onboard/go-fdo-server/internal/auth/apikey"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
	return db
}

func TestAuthIntegration_ManufacturerHandler(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	ctx := t.Context()

	// Initialize domain tables (vouchers, rvinfo, etc.) as needed for the handler
	// This mirrors what NewManufacturingServer does for the DB.
	// Skipped here for brevity — the handler doesn't need populated domain data
	// to test auth middleware responses (401/403 are returned before reaching handlers).

	// Initialize auth tables
	if err := state.InitAuthDB(ctx, db); err != nil {
		t.Fatalf("InitAuthDB: %v", err)
	}

	// Parse scopes from the manufacturer's embedded OpenAPI spec
	routeScopes, err := auth.ParseRouteScopes(OpenAPISpecJSON)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}

	// Seed admin user + key with full scopes
	serverScopes := auth.CollectAllScopes(routeScopes)
	seedCfg := config.SeedConfig{
		Admin: &config.SeedAdminConfig{Name: "admin", Email: "admin@test.com"},
	}
	adminKey, err := state.SeedAuth(ctx, db, seedCfg, serverScopes, false)
	if err != nil {
		t.Fatalf("SeedAuth: %v", err)
	}

	// Create a read-only user with a scoped key
	readUser, err := state.CreateUser(ctx, db, "reader", "reader@test.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	readRole, err := state.CreateRole(ctx, db, "reader", "Read only", false, []string{"vouchers:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := state.AssignRoleToUser(ctx, db, readUser.ID, readRole.ID); err != nil {
		t.Fatalf("AssignRoleToUser: %v", err)
	}
	_, readKey, err := state.CreateAPIKey(ctx, db, "read-key", readUser.ID, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Build authenticator chain
	apikeyAuthn := apikey.New(db)
	defer apikeyAuthn.Wait()
	authenticators := []auth.Authenticator{apikeyAuthn}

	authCfg := config.AuthConfig{
		Enabled:    true,
		Mechanisms: config.MechanismsConfig{APIKey: &config.APIKeyMechanismConfig{Enabled: true}},
	}

	// Build the handler — note: this will fail to construct the full handler
	// without valid manufacturing keys/certs. Instead, test auth middleware
	// directly by composing it the same way Handler() does.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /vouchers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("DELETE /vouchers/{guid}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authN := auth.AuthNMiddleware(authenticators, authCfg.ExcludedPaths)
	authZ := auth.AuthZMiddleware(routeScopes)
	handler := authN(authZ(mux))

	tests := []struct {
		name       string
		method     string
		path       string
		apiKey     string
		wantStatus int
	}{
		{"no credentials", http.MethodGet, "/vouchers", "", http.StatusUnauthorized},
		{"invalid key", http.MethodGet, "/vouchers", "fdo_invalidkey12345678901234567890", http.StatusUnauthorized},
		{"admin reads vouchers", http.MethodGet, "/vouchers", adminKey, http.StatusOK},
		{"admin deletes voucher", http.MethodDelete, "/vouchers/some-guid", adminKey, http.StatusOK},
		{"reader reads vouchers", http.MethodGet, "/vouchers", readKey, http.StatusOK},
		{"reader deletes voucher — forbidden", http.MethodDelete, "/vouchers/some-guid", readKey, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.apiKey != "" {
				r.Header.Set("X-API-Key", tt.apiKey)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestAuthIntegration_ExcludedPaths(t *testing.T) {
	authN := auth.AuthNMiddleware(nil, nil) // no authenticators, just default exclusions
	handler := authN(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"health excluded", "/health", http.StatusOK},
		{"FDO v1.1 protocol excluded", "/fdo/101/msg/10", http.StatusOK},
		{"FDO v2.0 protocol excluded", "/fdo/200/msg/10", http.StatusOK},
		{"swagger docs excluded", "/api/docs/index.html", http.StatusOK},
		{"openapi spec excluded", "/api/openapi.json", http.StatusOK},
		{"management API not excluded", "/vouchers", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuthIntegration_V1Protected(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	ctx := t.Context()

	if err := state.InitAuthDB(ctx, db); err != nil {
		t.Fatalf("InitAuthDB: %v", err)
	}

	routeScopes, err := auth.ParseRouteScopes(OpenAPISpecJSON)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}
	// Add manufacturer V1 override
	routeScopes["POST /rvinfo"] = []string{"rvinfo:write"}

	apikeyAuthn := apikey.New(db)
	defer apikeyAuthn.Wait()
	authenticators := []auth.Authenticator{apikeyAuthn}

	// V1 mux (simulated — after StripPrefix("/api/v1"))
	v1Mux := http.NewServeMux()
	v1Mux.HandleFunc("POST /rvinfo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authN := auth.AuthNMiddleware(authenticators, nil)
	authZ := auth.AuthZMiddleware(routeScopes)
	handler := authN(authZ(v1Mux))

	// Without auth, V1 POST /rvinfo should be rejected
	r := httptest.NewRequest(http.MethodPost, "/rvinfo", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("V1 POST /rvinfo without auth: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
```

- [ ] **Step 8: Run integration tests**

Run: `go test ./api/v2/manufacturer/ -run "TestAuthIntegration" -v`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add api/v2/manufacturer/handler.go api/v2/manufacturer/auth_integration_test.go
git add api/v2/owner/handler.go api/v2/rendezvous/handler.go
git add api/v2/owner/backward_compatibility_test.go
git add internal/server/manufacturing.go internal/server/owner.go internal/server/rendezvous.go
git add internal/config/manufacturer.go internal/config/owner.go internal/config/rendezvous.go
git commit -m "feat: wire auth middleware into server startup"
```

---

### Task 9: CLI `init-admin` Command

**Files:**
- Modify: `cmd/manufacturing.go` — add `init-admin` subcommand
- Modify: `cmd/owner.go` — add `init-admin` subcommand
- Modify: `cmd/rendezvous.go` — add `init-admin` subcommand

**Interfaces:**
- Consumes: `config.*ServerConfig`, `state.InitAuthDB`, `state.SeedAuth`
- Produces: `init-admin` subcommand for each server role

- [ ] **Step 1: Add init-admin command for manufacturing**

In `cmd/manufacturing.go`, add after `manufacturingCmd` definition:

```go
var manufacturingInitAdminCmd = &cobra.Command{
	Use:   "init-admin",
	Short: "Create initial admin user and API key",
	Long:  `Creates an admin user with full permissions and generates an API key. The API key is printed to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var mfgConfig config.ManufacturingServerConfig
		if err := viper.Unmarshal(&mfgConfig); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}

		gormDB, err := mfgConfig.DB.GetDB()
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		ctx := cmd.Context()

		if err := state.InitAuthDB(ctx, gormDB); err != nil {
			return fmt.Errorf("failed to initialize auth database: %w", err)
		}

		name, _ := cmd.Flags().GetString("name")
		email, _ := cmd.Flags().GetString("email")
		force, _ := cmd.Flags().GetBool("force")

		routeScopes, err := auth.ParseRouteScopes(manufacturer.OpenAPISpecJSON)
		if err != nil {
			return fmt.Errorf("failed to parse route scopes: %w", err)
		}
		serverScopes := auth.CollectAllScopes(routeScopes)

		seedCfg := config.SeedConfig{
			Admin: &config.SeedAdminConfig{Name: name, Email: email},
		}
		apiKey, err := state.SeedAuth(ctx, gormDB, seedCfg, serverScopes, force)
		if err != nil {
			return err
		}

		if apiKey == "" {
			return fmt.Errorf("auth database already seeded; use --force to create an additional admin user")
		}

		fmt.Println(apiKey)
		return nil
	},
}
```

In `manufacturingCmdInit()`, add:

```go
manufacturingCmd.AddCommand(manufacturingInitAdminCmd)
manufacturingInitAdminCmd.Flags().String("name", "admin", "Admin user name")
manufacturingInitAdminCmd.Flags().String("email", "admin@example.com", "Admin user email")
manufacturingInitAdminCmd.Flags().Bool("force", false, "Create admin even if users exist")
```

- [ ] **Step 2: Add same command for owner and rendezvous**

Apply the same pattern to `cmd/owner.go` and `cmd/rendezvous.go`. Each uses `auth.ParseRouteScopes` with its own server role's merged OpenAPI spec, then `auth.CollectAllScopes` to derive scopes — no hardcoded scope lists needed.

- [ ] **Step 3: Run the command to verify it works**

Run: `go build -o /tmp/go-fdo-server . && /tmp/go-fdo-server manufacturing init-admin --config configs/manufacturing.yaml --name admin --email admin@test.com`
Expected: Prints an API key starting with `fdo_`

- [ ] **Step 4: Commit**

```bash
git add cmd/manufacturing.go cmd/owner.go cmd/rendezvous.go
git commit -m "feat: add init-admin CLI command for all server roles"
```

---

### Task 10: Update Config File Templates

**Files:**
- Modify: `configs/manufacturing.yaml` — add commented `auth` section
- Modify: `configs/owner.yaml` — add commented `auth` section
- Modify: `configs/rendezvous.yaml` — add commented `auth` section

**Interfaces:**
- Consumes: nothing
- Produces: Config files with auth section documentation

- [ ] **Step 1: Add auth section to all config templates**

Append to each config file:

```yaml

# Authentication and authorization configuration.
# When enabled, management API endpoints require valid credentials.
# FDO protocol endpoints (/fdo/101/msg/*, /fdo/200/msg/*) are never affected.
#auth:
#  enabled: true
#  mechanisms:
#    api_key:
#      enabled: true
#  seed:
#    admin:
#      name: "admin"
#      email: "admin@example.com"
```

- [ ] **Step 2: Commit**

```bash
git add configs/
git commit -m "docs: add auth configuration examples to config templates"
```
