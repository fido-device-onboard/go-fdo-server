# Management API Authentication & Authorization — Phase 1 Design

**Date:** 2026-06-29
**Phase:** 1 of 3 — Auth framework + API key mechanism
**Status:** Approved

## Overview

This design introduces authentication and authorization for the FDO server management API. The existing OpenAPI specs declare five security schemes (Basic, Bearer, API Key, OIDC, OAuth2) and all endpoints reference 401/403 responses, but no authentication enforcement exists in the implementation.

Phase 1 establishes the auth framework and implements the first mechanism (API key). The framework is designed to accommodate additional mechanisms in later phases.

### Phase Roadmap

- **Phase 1** (this document): Auth framework + API key mechanism + configuration + bootstrapping
- **Phase 2**: Management API endpoints (CRUD for users, groups, roles, API keys) + CLI bootstrapping
- **Phase 3**: Additional auth mechanisms (Bearer, Basic, OIDC, OAuth2, SAML)

### Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| First auth mechanism | API key | Simplest end-to-end, no external deps, validates full pipeline |
| Authorization model | RBAC + fine-grained scopes | Scopes map to OAuth2 claims later; principle of least privilege |
| Auth data storage | Same DB as server role data | Matches existing per-server isolation; external IdP handles centralized identity |
| Role names | Shared (`admin`, `operator`) | Each server has isolated DB; prefix adds no real disambiguation |
| Management endpoints | Same API surface | Avoids second HTTP server complexity; auth-protected anyway |
| Bootstrapping | Config-file seed + CLI command | Config for automated deployments, CLI for manual setups |
| Auth mechanisms | Multiple simultaneously | Standard pattern; enables migration and M2M + human coexistence |
| Scope source of truth | OpenAPI `x-required-scopes` | Single source of truth; no drift between spec and code |
| Architecture | Middleware chain (AuthN → AuthZ) | Guarantees no handler can skip auth; clean separation of concerns |

## Core Auth Framework Architecture

### Identity Model

Every authenticated request resolves to an `Identity` — a unified representation regardless of how the user authenticated:

```go
type Identity struct {
    Subject    string            // unique identifier (always the user ID, even for API key auth)
    Name       string            // human-readable name
    AuthMethod string            // "api-key", "bearer", "basic", "oidc", etc.
    Roles      []string          // e.g., ["admin"]
    Scopes     []string          // effective scopes, resolved from roles + direct grants
    Metadata   map[string]string // auth-method-specific data
}
```

This is the contract between AuthN and AuthZ. The AuthZ middleware never needs to know how the user authenticated, only what they're authorized to do.

### Authenticator Interface

Each auth mechanism implements this interface:

```go
type Authenticator interface {
    // Name returns the mechanism name (e.g., "api-key", "oidc")
    Name() string
    // Authenticate inspects the request and returns an Identity if credentials are valid (*Identity, nil).
    // Returns (nil, error) if credentials are present but invalid.
    // Returns (nil, nil) if this authenticator doesn't match the request (no relevant credentials present).
    Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}
```

The semantic: `nil, nil` means "not my request, try the next authenticator"; `nil, error` means "credentials were present but invalid — stop and return 401".

**Error contract:** The AuthN middleware uses `errors.Is(err, ErrInvalidCredentials)` to distinguish client errors (401) from server errors (500). Authenticators MUST return `ErrInvalidCredentials` (or a wrapped form) only for credential-related failures (bad key, expired token, inactive user). For internal failures (DB unreachable, corrupt data), authenticators MUST return a different error — typically a package-level sentinel like `errInternal` — so the middleware correctly returns 500 instead of 401. Wrapping internal errors with `ErrInvalidCredentials` would mask server failures as client errors and prevent retries.

### Middleware Chain

```
Request → RateLimit → BodySize → AuthN → AuthZ → OpenAPI Validation → Handler
```

**AuthN middleware** iterates over enabled authenticators in order:

1. For each authenticator, call `Authenticate(ctx, r)`
2. If it returns an identity → set on context, proceed to AuthZ
3. If it returns `nil, nil` → try next authenticator
4. If it returns `nil, error` → classify the error:
   - `ErrInvalidCredentials` → return 401 (bad credentials, log at DEBUG)
   - Any other error (e.g., DB failure) → return 500 Internal Server Error (log at ERROR). Clients should retry, not assume their credentials are invalid.
5. If all return `nil, nil` → return 401 (no valid credentials found)
6. Skip auth entirely for excluded paths (health, docs, FDO protocol endpoints)

**AuthZ middleware** checks the resolved identity's scopes against the route's required scopes:

1. Clean the request path with `path.Clean(r.URL.Path)` to normalize double slashes and dot segments before lookup — without this, a request to `/vouchers//123` bypasses authorization because the uncleaned path fails to match the pattern `/vouchers/{guid}` due to mismatched segment counts
2. Look up required scopes for `method + cleaned path pattern` using segment-by-segment matching: split both the cleaned request path and each route pattern on `/`, reject if segment counts differ, then compare each segment — pattern segments wrapped in `{...}` (e.g., `{guid}`) match any value (wildcard), all other segments must match exactly. This is an O(n) scan over all route scope entries; acceptable given the expected map size (~17 operations across all specs)
3. If the identity's effective scopes include all required scopes → proceed
4. Otherwise → return 403

## Scopes, Roles, and Database Models

### Scope Definitions Per Server Type

Scopes follow the pattern `resource:action`.

**Manufacturer server:**

| Scope | Description |
|---|---|
| `vouchers:read` | List/get vouchers |
| `vouchers:write` | Import vouchers |
| `vouchers:delete` | Delete vouchers |
| `vouchers:extend` | Extend voucher ownership |
| `rvinfo:read` | Read rendezvous info config |
| `rvinfo:write` | Create/update rendezvous info config |
| `rvinfo:delete` | Delete rendezvous info config |

**Owner server:**

| Scope | Description |
|---|---|
| `vouchers:read` | List/get vouchers |
| `vouchers:write` | Import/verify ownership vouchers |
| `vouchers:delete` | Delete vouchers |
| `vouchers:extend` | Extend voucher ownership |
| `device-ca:read` | List/get device CA certs |
| `device-ca:write` | Add device CA certs |
| `device-ca:delete` | Delete device CA certs |
| `rvto2addr:read` | Get RVTO2 addresses |
| `rvto2addr:write` | Set RVTO2 addresses |
| `rvto2addr:delete` | Delete RVTO2 addresses |
| `devices:read` | List devices and onboarding state |

**Rendezvous server:**

| Scope | Description |
|---|---|
| `device-ca:read` | List/get device CA certs |
| `device-ca:write` | Add device CA certs |
| `device-ca:delete` | Delete device CA certs |

### Default Roles

| Role | Scopes |
|---|---|
| `admin` | All scopes for the server type + `auth:manage` (Phase 2 user/key management) |
| `operator` | All `*:read` + all `*:write` + `vouchers:extend` scopes for the server type (can create/modify, cannot delete or manage auth) |

Roles are stored in the database, not hardcoded. These are seed defaults created on first startup. Admins can create custom roles with arbitrary scope combinations via the management API (Phase 2). Built-in roles have a `BuiltIn` flag that prevents deletion.

**Phase 2 hardening note:** The seed logic derives operator scopes using suffix matching (`:read`, `:write`) plus the literal `vouchers:extend`. This is correct for Phase 1's well-known scope names, but a future scope like `audit:write-log` would be incorrectly included. Phase 2 should either enumerate operator scopes explicitly per server type or use a scope metadata registry to classify scopes by role eligibility.

### Database Models

```
┌─────────────┐       ┌──────────────────┐       ┌─────────────┐
│    User     │──M:N──│   UserRole       │──M:1──│    Role     │
├─────────────┤       ├──────────────────┤       ├─────────────┤
│ ID (UUID)   │       │ UserID           │       │ ID (UUID)   │
│ Name        │       │ RoleID           │       │ Name        │
│ Email       │       └──────────────────┘       │ Description │
│ Active      │                                  │ BuiltIn     │
│ CreatedAt   │       ┌──────────────────┐       │ CreatedAt   │
│ UpdatedAt   │       │   RoleScope      │──M:1──│ UpdatedAt   │
└─────────────┘       ├──────────────────┤       └─────────────┘
                      │ RoleID           │
                      │ Scope            │
                      └──────────────────┘

┌──────────────────┐
│    APIKey        │
├──────────────────┤
│ ID (UUID)        │
│ Prefix (6 chars) │  ← visible identifier for logs/display
│ HashedKey        │  ← SHA-256 hash of the full key
│ Name             │  ← human label ("CI pipeline key")
│ UserID (FK)      │  ← owning user (CASCADE delete)
│ Scopes           │  ← optional scope restriction (subset of user's scopes)
│ ScopeRestricted  │  ← true if key was created with explicit scope restrictions
│ ExpiresAt        │  ← optional expiration
│ Active           │
│ LastUsedAt       │
│ CreatedAt        │
│ UpdatedAt        │
└──────────────────┘
```

- API keys belong to users — a user can have multiple API keys, each optionally restricted to a subset of the user's scopes
- The full API key is shown once at creation, then only the SHA-256 hash is stored
- The prefix (first 6 chars after `fdo_`) is stored in cleartext for identification in logs and management UI
- If an API key has explicit scopes (`ScopeRestricted = true`), they are intersected with the user's role-derived scopes. If unrestricted (`ScopeRestricted = false`), the key inherits all the user's scopes. The `ScopeRestricted` boolean disambiguates "intentionally unrestricted" from "scopes field was cleared/corrupted" — when `ScopeRestricted = true` and the `Scopes` JSON is empty or corrupted, the key gets zero effective scopes (fail closed).
- All join tables (`UserRole`, `RoleScope`) and the `APIKey.UserID` foreign key use `ON DELETE CASCADE` constraints via GORM reference pointer fields (e.g., `UserRef *User \`gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"\``), ensuring that deleting a user or role automatically cleans up related records. The `constraint:OnDelete:CASCADE` tag alone (without the reference pointer) does not generate foreign key DDL in GORM's `AutoMigrate` — the reference field is required. SQLite additionally requires `PRAGMA foreign_keys = ON` for CASCADE to function (disabled by default).
- Input validation is enforced at the business logic layer: user names and emails must be non-empty with valid format, role names must be non-empty, API key names must be non-empty, and scope strings must match `[a-z0-9][a-z0-9:_-]*`.
- `FindOrCreateRole` handles concurrent calls safely by retrying the lookup if the unique constraint fails on creation (create-or-fetch pattern).
- No groups in Phase 1 — they add organizational convenience but no authorization capability that roles don't already provide. Can be added in Phase 2 if needed.

## API Key Authentication Mechanism

### Key Format

```
fdo_<random-32-bytes-base62-encoded>
```

Example: `fdo_a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5P6q7R8s9T0u1V2`

- Prefix `fdo_` makes keys identifiable (grep-able in logs, scannable by secret detection tools)
- 32 bytes of cryptographic randomness, base62-encoded to ~43 alphanumeric characters (256 bits of entropy). The encoding is left-padded with `0` if shorter than 43 characters to ensure consistent minimum length
- Base62 encoding (alphanumeric, no special chars) avoids shell escaping issues
- First 6 characters after `fdo_` stored as the `Prefix` field for display/identification

### Storage and Validation

- On creation: generate the key, return the full key to the user once, store only `sha256(full_key)`
- On request: extract `X-API-Key` header → query by prefix (first 6 chars after `fdo_`) → `sha256(provided_key)` + constant-time comparison to verify
- SHA-256 is appropriate here because API keys have 256 bits of cryptographic randomness, making them immune to offline dictionary attacks. bcrypt's intentional slowness would add 50-100ms per request and create a CPU-exhaustion DoS vector
- Prefix-based lookup avoids full-table scan — the prefix is a non-unique index (~56 billion combinations with 6 base62 chars)

### Scope Resolution Logic

```
User roles → expand to role scopes → union = user's full scope set
ScopeRestricted = false → user's full scope set = effective scopes
ScopeRestricted = true, valid scopes → intersect with user's full scope set = effective scopes
ScopeRestricted = true, empty scopes → zero effective scopes (fail closed)
ScopeRestricted = true, corrupted JSON → error, reject authentication (fail closed)
```

An API key can never exceed the user's permissions, even if the user's roles change after key creation.

**`ScopeRestricted` flag:** The `APIKey.ScopeRestricted` boolean disambiguates between "key was created without scope restrictions" (`false` — inherits all user scopes) and "key was created with explicit scope restrictions" (`true` — only the scopes listed in the `Scopes` JSON column apply). This prevents a privilege escalation where a truncated or cleared `Scopes` field silently grants maximum permissions: when `ScopeRestricted = true` and the `Scopes` JSON is empty, the key gets zero effective scopes.

**Fail-closed on corrupted scopes:** The `APIKey.ScopesList()` method returns `([]string, error)`. If the JSON-encoded `Scopes` column is corrupted and cannot be unmarshalled, the method returns an error. The authenticator treats this as an authentication failure — it never falls through to "empty scopes = all user scopes."

### Authentication Flow

1. Extract `X-API-Key` header — if absent, return `nil, nil` (try next authenticator)
2. Validate format (`fdo_` prefix, minimum 10 characters total — 4-char prefix + 6-char identifier minimum) — if invalid, log at DEBUG level and return `nil, error`
3. Extract prefix (6 chars after `fdo_`)
4. Query API key candidates by prefix where `APIKey.Active = true`
5. Compute `sha256(provided_key)` and constant-time compare against each candidate's `HashedKey`
6. Check expiration — if expired, log at DEBUG level and return `nil, error` (same error as invalid credentials to avoid leaking key validity; checked before user lookup to avoid a DB roundtrip on expired keys)
7. Verify `User.Active = true` for the key's owning user — if user is inactive, log at WARN level and return `nil, error`
8. Update `last_used_at` asynchronously (background goroutine with `context.WithoutCancel` to survive request cancellation, then `context.WithTimeout` for a 5-second deadline; tracked by `sync.WaitGroup` for graceful shutdown draining; failures are logged at WARN level but do not block authentication). The `APIKeyAuthenticator` exposes a `Wait()` method that blocks until all pending `last_used_at` updates complete. The server startup code must retain a reference to the authenticator and call `Wait()` during graceful shutdown — creating the authenticator inside `Handler()` without exposing it would make the WaitGroup unreachable.
9. Resolve effective scopes and return `Identity`
10. If no match, log at DEBUG level and return `nil, error`

## OpenAPI Spec Changes and Scope Parsing

### Custom Extension: `x-required-scopes`

Each operation in the OpenAPI spec gets an `x-required-scopes` array declaring required scopes:

```yaml
paths:
  /vouchers:
    get:
      operationId: ListOwnershipVouchers
      x-required-scopes:
        - vouchers:read
    post:
      operationId: ImportOwnershipVouchers
      x-required-scopes:
        - vouchers:write
  /vouchers/{guid}:
    get:
      operationId: GetOwnershipVoucherByGuid
      x-required-scopes:
        - vouchers:read
    delete:
      operationId: DeleteOwnershipVoucher
      x-required-scopes:
        - vouchers:delete
  /vouchers/{guid}/extend:
    post:
      operationId: ExtendOwnershipVoucher
      x-required-scopes:
        - vouchers:extend
  /vouchers/verify-ownership:
    post:
      operationId: VerifyOwnership
      x-required-scopes:
        - vouchers:write
```

Operations with no `x-required-scopes` (like `/health`) are treated as public.

When an operation lists multiple scopes, ALL are required (AND logic). For example, `x-required-scopes: [vouchers:read, vouchers:write]` requires the identity to have both scopes.

### Scope Parsing at Startup

The project already merges per-resource OpenAPI specs into a combined spec for each server role. The scope parser hooks into the same loaded spec:

```go
func ParseRouteScopes(specJSON []byte) (map[string][]string, error)
func CollectAllScopes(routeScopes map[string][]string) []string
```

`ParseRouteScopes` runs once at startup. The returned map is passed to the AuthZ middleware. `CollectAllScopes` extracts all unique scopes from the route map — this is used by the seed logic and CLI `init-admin` command to derive the server's scope set from the OpenAPI spec rather than hardcoding scope lists. This keeps the OpenAPI spec as the single source of truth for scopes.

**Extension type note:** kin-openapi v0.135.0 populates `Operation.Extensions` via `json.Unmarshal` into `map[string]any`. For `x-required-scopes`, this produces `[]interface{}` with `string` elements (standard Go JSON-to-interface{} rules). The parser uses `ext.([]interface{})` type assertion — no `json.RawMessage` handling needed. If the kin-openapi dependency is updated, verify that extensions are still decoded as `map[string]any` rather than stored as `json.RawMessage`.

### Startup Validation

1. **Missing scopes warning**: Operation has `security` schemes but no `x-required-scopes` → log warning
2. **Unknown scopes warning**: `x-required-scopes` references a scope not in any role definition → log warning (not a fatal error, because scopes like `auth:manage` are defined for Phase 2 endpoints that don't exist yet)
3. **Unprotected endpoints info**: Log which endpoints have no `x-required-scopes`

### Files Requiring `x-required-scopes` Annotations

| File | Operations |
|---|---|
| `api/v2/voucher/openapi.yaml` | 6 operations |
| `api/v2/deviceca/openapi.yaml` | 4 operations |
| `api/v2/rvinfo/openapi.yaml` | 3 operations |
| `api/v2/rvto2addr/openapi.yaml` | 3 operations |
| `api/v2/device/openapi.yaml` | 1 operation |
| `api/v2/health/openapi.yaml` | 0 operations (public) |

## Configuration Schema

### Auth Configuration Block

Added to each server's YAML config under a new `auth` key:

```yaml
auth:
  # When false or omitted, all endpoints are accessible without authentication.
  # When true, at least one mechanism must be enabled.
  enabled: true

  # Additional paths excluded from authentication.
  # Health check, docs, OpenAPI spec, and FDO protocol endpoints are always excluded.
  # excluded_paths:
  #   - /custom/public/endpoint

  # Authentication mechanisms — multiple can be enabled simultaneously.
  mechanisms:
    api_key:
      enabled: true

    # Future mechanisms (Phase 3):
    # basic:
    #   enabled: true
    # bearer:
    #   enabled: true
    #   jwt_secret: "..."
    #   jwt_issuer: "..."
    # oidc:
    #   enabled: true
    #   issuer_url: "https://auth.example.com/realms/fdo"
    #   client_id: "fdo-owner"
    # oauth2:
    #   enabled: true
    #   token_introspection_url: "..."
    #   client_id: "..."
    #   client_secret: "..."

  # Bootstrap: seed an initial admin user + API key on first startup.
  # Ignored if any users already exist in the database.
  seed:
    admin:
      name: "admin"
      email: "admin@example.com"
      # Optional: pre-set API key for automated deployments.
      # If omitted, a random key is generated and logged at WARN level.
      # api_key: "fdo_..."
```

### Config Struct

```go
type AuthConfig struct {
    Enabled       bool              `mapstructure:"enabled"`
    ExcludedPaths []string          `mapstructure:"excluded_paths"`
    Mechanisms    MechanismsConfig  `mapstructure:"mechanisms"`
    Seed          *SeedConfig       `mapstructure:"seed"`
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
```

### Validation Rules

- `auth.enabled: true` requires at least one mechanism enabled — otherwise startup fails
- `auth.enabled: false` or omitted → auth middleware not installed, backward-compatible
- `auth.excluded_paths` entries must start with `/` — startup fails if any entry is empty or missing the leading slash
- `seed.admin` processed only when auth is enabled AND no users exist in the database
- Pre-set `seed.admin.api_key` validated for format (`fdo_` prefix, minimum length) — validation happens at seed time in `createPresetAPIKey`, not in `AuthConfig.Validate()`, because the seed block is only processed when auth is enabled and no users exist. A config-level check would reject valid configs on restarts when users already exist and the seed is ignored.
- Generated API key logged at `WARN` level with message to save it
- `--force` flag on CLI `init-admin` allows creating additional admin users when users already exist. If the provided `--email` conflicts with an existing user's unique email, the seed returns a clear error message wrapping the constraint violation rather than a raw GORM error — the `CreateUser` function already wraps DB errors with `fmt.Errorf("failed to create user: %w", err)`, so the caller sees the context.

### CLI Bootstrap Command

```
go-fdo-server <role> init-admin --name "admin" --email "admin@example.com"
```

- Creates admin user with `admin` role and generates an API key
- Prints the API key to stdout
- Returns a non-zero exit code with a clear error message if users already exist (use `--force` to create additional admin)
- Uses the same config file for DB connection (`--config` flag)

## Package Layout

```
internal/
├── auth/                          # Core auth framework
│   ├── identity.go                # Identity type, context helpers
│   ├── authenticator.go           # Authenticator interface
│   ├── middleware.go              # AuthN and AuthZ middleware
│   ├── scopes.go                  # Scope parser (OpenAPI x-required-scopes)
│   └── apikey/                    # API key authenticator
│       └── authenticator.go       # APIKeyAuthenticator implementation
├── config/
│   ├── auth.go                    # AuthConfig structs + validation (NEW)
│   └── server.go                  # Updated: add Auth field to ServerConfig
├── state/
│   ├── user.go                    # User GORM model + DB operations (NEW)
│   ├── role.go                    # Role, RoleScope GORM models + DB operations (NEW)
│   ├── apikey.go                  # APIKey GORM model + DB operations + key generation (NEW)
│   └── seed.go                    # Bootstrap seeding logic (NEW)
├── server/
│   ├── manufacturing.go           # Updated: wire auth middleware
│   ├── owner.go                   # Updated: wire auth middleware
│   └── rendezvous.go              # Updated: wire auth middleware
```

### Server Startup Sequence (Updated)

1. Load config (existing)
2. Initialize DB (existing)
3. If `auth.enabled`: auto-migrate auth tables (NEW) — skipped when auth is disabled to avoid DDL operations that could fail with restricted DB permissions
4. If `auth.enabled`: seed admin if needed (NEW)
5. If `auth.enabled`: parse route scopes from OpenAPI spec (NEW)
6. If `auth.enabled`: build authenticator chain from config (NEW)
7. Create handler with middleware chain (MODIFIED — insert AuthN + AuthZ when auth enabled)
8. Start HTTP server (existing)

### Middleware Chain Assembly

```go
// Before:
handler = rateLimit(bodySize(openapiValidation(mux)))

// After:
handler = rateLimit(bodySize(authN(authZ(openapiValidation(mux)))))
```

The `Handler()` method signature gains three new parameters (`authCfg`, `authenticators`, `routeScopes`) but keeps returning `http.Handler` — no error return needed because `AuthNMiddleware` and `AuthZMiddleware` are pure closures that cannot fail at construction time. The authenticator chain is built in the server startup code and passed in, so `Handler()` does not create authenticators itself. When auth is disabled (`authCfg.Enabled == false`), pass a zero-value `AuthConfig{}`, `nil` authenticators, and `nil` route scopes to preserve the existing no-auth behavior.

### Graceful Shutdown

The server startup code retains a reference to `APIKeyAuthenticator` (separate from the `[]Authenticator` slice passed to `Handler()`) and calls `Wait()` during graceful shutdown — after `httpServer.Shutdown(ctx)` returns and before the process exits. This drains any in-flight background `last_used_at` goroutines. The `Wait()` call is a no-op if no API key authentication occurred.

### Dependencies

- `crypto/sha256` + `crypto/subtle` (standard library) — API key hashing and constant-time comparison
- No external auth frameworks

## Backward Compatibility

### Zero-Breaking-Change Guarantee

Default behavior when `auth` is not configured is identical to current behavior — all endpoints are open:

- Existing deployments continue working without config changes
- Existing integration tests pass without modification
- FDO protocol endpoints are never affected by management API auth

### Migration Path

1. **Upgrade binary** — No config changes needed. No schema changes, no auth middleware installed. Fully backward-compatible.
2. **Enable auth** — Add `auth` section to config. On restart: auth tables created, seed admin created, API key logged, auth middleware installed.
3. **Distribute keys** — Admin uses seeded key to create additional users/keys via CLI (Phase 1) or management API (Phase 2).

### FDO Protocol Endpoint Exclusion

Always excluded from management API auth (hardcoded):

```
/fdo/101/msg/         # FDO v1.1 protocol messages (prefix match)
/fdo/200/msg/         # FDO v2.0 protocol messages (prefix match)
/health               # Health check (exact match)
/api/docs/            # Swagger UI (prefix match)
/api/openapi.json     # OpenAPI spec (exact match)
```

The `auth.excluded_paths` config option allows adding custom exclusions on top of these defaults.

**Path matching semantics:** The request path is cleaned with `path.Clean` before matching to normalize double slashes and dot segments. Excluded paths use exact match for leaf paths (e.g., `/health` matches only `/health`, not `/health-check` or `/healthy`). Paths ending with `/` are treated as prefix matches: the cleaned path must either equal the entry without its trailing slash, or start with the full entry (including the trailing slash). This two-part check prevents both `path.Clean`'s trailing-slash stripping from breaking legitimate matches (e.g., `/api/docs/` cleaned to `/api/docs` still matches) and unintended bypasses through path manipulation (e.g., `/api/docsextra` does NOT match `/api/docs/` because `"/api/docsextra"` is not `"/api/docs"` and does not start with `"/api/docs/"`).

### V1 API Protection

The legacy V1 management API (`/api/v1/`) supports voucher, rendezvous info, device CA, device, and redirect operations. When auth is enabled, V1 endpoints MUST be wrapped with both `AuthN` and `AuthZ` middleware — not just `AuthN`. Without `AuthZ`, any authenticated user (even one with a restricted `vouchers:read`-only key) could execute all V1 operations including writes.

#### V1 Route Inventory

The following tables list all V1 routes per server type (paths shown after `http.StripPrefix("/api/v1", ...)` removes the version prefix). Routes that already match the V2-derived `routeScopes` map are marked ✓; routes that require additional V1-specific scope entries are marked ✗.

**Manufacturer V1** (6 routes):

| Method | Path (after StripPrefix) | Handler | Matches V2 scope map? |
|---|---|---|---|
| `GET` | `/vouchers` | ListVouchers | ✓ |
| `POST` | `/vouchers` | InsertVoucher | ✓ |
| `GET` | `/vouchers/{guid}` | GetVoucherByGUID | ✓ |
| `GET` | `/rvinfo` | GetRendezvousInfo | ✓ |
| `POST` | `/rvinfo` | CreateRendezvousInfo | ✗ — V2 uses `PUT` only |
| `PUT` | `/rvinfo` | UpdateRendezvousInfo | ✓ |

The manufacturer V1 has one method mismatch: V1 registers `POST /rvinfo` (CreateRendezvousInfo) in addition to `PUT /rvinfo` (UpdateRendezvousInfo). The V2-derived scope map only has `PUT /rvinfo` → `rvinfo:write`, so `POST /rvinfo` would bypass AuthZ. The manufacturer handler must add a V1-specific scope override: `POST /rvinfo` → `rvinfo:write`.

Note: V1 voucher routes have no `DELETE` endpoint — V1 does not support voucher deletion on the manufacturer server.

**Owner V1** (12 routes):

| Method | Path (after StripPrefix) | Handler | Matches V2 scope map? |
|---|---|---|---|
| `GET` | `/owner/vouchers` | ListVouchers | ✗ — `/owner/` prefix |
| `POST` | `/owner/vouchers` | InsertVoucher | ✗ — `/owner/` prefix |
| `GET` | `/owner/vouchers/{guid}` | GetVoucherByGUID | ✗ — `/owner/` prefix |
| `POST` | `/owner/resell/{guid}` | ResellVoucher | ✗ — `/owner/` prefix |
| `GET` | `/owner/devices` | ListDevices | ✗ — `/owner/` prefix |
| `GET` | `/owner/redirect` | GetRVTO2Addr | ✗ — `/owner/` prefix |
| `POST` | `/owner/redirect` | CreateRVTO2Addr | ✗ — `/owner/` prefix |
| `PUT` | `/owner/redirect` | UpdateRVTO2Addr | ✗ — `/owner/` prefix |
| `GET` | `/device-ca` | ListTrustedDeviceCACerts | ✓ |
| `POST` | `/device-ca` | ImportTrustedDeviceCACerts | ✓ |
| `DELETE` | `/device-ca/{fingerprint}` | DeleteTrustedDeviceCACert | ✓ |
| `GET` | `/device-ca/{fingerprint}` | GetTrustedDeviceCACertByFingerprint | ✓ |

The owner V1 routes register under an `/owner/` prefix (via `HandlerWithOptions` with `BaseURL: "/owner"`) or use different path structures, so after `StripPrefix` these 8 paths don't match the V2-derived `routeScopes` keys and would bypass AuthZ silently (`lookupRequiredScopes` returns nil → no scope check → proceed). The `device-ca` routes use `HandlerFromMux` without a `BaseURL`, so they match V2 patterns directly.

Note: V1 voucher routes have no `DELETE` endpoint — V1 does not support voucher deletion on the owner server. The V1 `resell` endpoint maps to the V2 `extend` operation.

To fix this, the owner handler must define additional V1-specific scope entries that map these paths to the same scopes as their V2 equivalents:

| V1 path (after StripPrefix) | Scope | V2 equivalent |
|---|---|---|
| `GET /owner/vouchers` | `vouchers:read` | `GET /vouchers` |
| `POST /owner/vouchers` | `vouchers:write` | `POST /vouchers` |
| `GET /owner/vouchers/{guid}` | `vouchers:read` | `GET /vouchers/{guid}` |
| `POST /owner/resell/{guid}` | `vouchers:extend` | `POST /vouchers/{guid}/extend` |
| `GET /owner/devices` | `devices:read` | `GET /devices` |
| `GET /owner/redirect` | `rvto2addr:read` | `GET /rvto2addr` |
| `POST /owner/redirect` | `rvto2addr:write` | `POST /rvto2addr` |
| `PUT /owner/redirect` | `rvto2addr:write` | `PUT /rvto2addr` |

These entries are merged into the same `routeScopes` map before passing it to `AuthZMiddleware`.

**Rendezvous V1** (4 routes):

| Method | Path (after StripPrefix) | Handler | Matches V2 scope map? |
|---|---|---|---|
| `GET` | `/device-ca` | ListTrustedDeviceCACerts | ✓ |
| `POST` | `/device-ca` | ImportTrustedDeviceCACerts | ✓ |
| `DELETE` | `/device-ca/{fingerprint}` | DeleteTrustedDeviceCACert | ✓ |
| `GET` | `/device-ca/{fingerprint}` | GetTrustedDeviceCACertByFingerprint | ✓ |

For the rendezvous server, all V1 routes match V2 scope map patterns directly — no additional scope entries needed.

### Database Migration

Auth tables added via GORM `AutoMigrate` — same pattern as existing models. New tables only, no modifications to existing tables, safe to run repeatedly. Migration is only triggered when `auth.enabled: true` — when auth is disabled, no DDL operations are performed, avoiding failures in environments where the database user has restricted permissions (e.g., DML-only).

## Future Considerations: API Key Lifecycle

### Expired Key Cleanup

Phase 1 does not clean up expired API keys — they remain in the database but are rejected during authentication. Over time, expired keys accumulate as dead rows. Phase 2 should provide a cleanup mechanism:

- A management API endpoint (e.g., `DELETE /api/v2/apikeys?expired=true`) for admin-initiated cleanup
- Optionally, a periodic background job that deletes keys expired beyond a configurable retention period (e.g., 30 days)

### Batched `last_used_at` Updates

If authentication throughput becomes a concern, the per-request `UpdateAPIKeyLastUsed` goroutine can be replaced with a batching pattern: collect key IDs in a channel and flush to the database in bulk every N seconds. Not needed for Phase 1 given expected load.

## Future Considerations: Multi-Tenancy on the Manufacturer Server

This section documents a planned evolution of the authorization model that goes beyond Phase 1. It is captured here to confirm that the Phase 1 framework does not block this path.

### Use Case

The manufacturer server allows multiple customers (device owners) to register accounts. Each customer:

- Registers their own owner public keys via a management API
- Can self-register (public registration endpoint)
- Can view only the vouchers that were extended to their keys
- Cannot see other customers' vouchers or keys

The manufacturer admin retains full access to all resources and can use any registered owner key to extend vouchers at sell time.

### New Role: `customer`

| Role | Scopes | Resource access |
|---|---|---|
| `admin` | All scopes + `auth:manage` | All resources (no ownership filter) |
| `operator` | `*:read` + `*:write` + `vouchers:extend` | All resources |
| `customer` | `vouchers:read`, `owner-keys:read`, `owner-keys:write` | Only own resources |

The key difference: `customer` and `admin` may share the same scope (`vouchers:read`) but the effective query is different — admin sees all, customer sees only theirs. This is **resource-level authorization** handled at the handler level using the `Identity.Subject` (user ID) from the request context, not in the scope middleware.

### What Changes Are Required

None of these require changes to the Phase 1 auth framework:

1. **New `OwnerKey` database model** — stores customer-uploaded owner public keys with a `UserID` foreign key
2. **New management API endpoints** — CRUD for owner keys, self-registration
3. **`OwnerID` foreign key on Voucher** — tracks which customer's key was used to extend each voucher
4. **Handler-level ownership filtering** — voucher list/get handlers check the identity's role; if `customer`, filter by `owner_id = identity.Subject`
5. **New `customer` role** — database insert with appropriate scopes

### Why Phase 1 Already Supports This

- The `Identity` struct carries `Subject` (user ID) and `Roles` — handlers can use these for resource filtering
- Adding a `customer` role is a database insert, not a code change
- New scopes (`owner-keys:read`, `owner-keys:write`) just need `x-required-scopes` on new endpoints
- The `Authenticator` interface and middleware chain are unaffected
- The scope intersection logic on API keys works identically for customer-scoped keys
