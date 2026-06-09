# Optional Owner Certificate & Ownership-Aware Voucher Handling

**Date:** 2026-06-03
**Status:** Approved

## Problem

The manufacturer server currently requires an owner certificate at startup.
This certificate is used to automatically extend every voucher during Device
Initialization (DI), which means:

1. The owner must be known at manufacturing time.
2. The voucher extension API endpoint fails with 500 because the manufacturer
   no longer owns the voucher after DI.
3. The owner server's TO0 scheduler blindly attempts TO0 for all pending
   vouchers, including ones the server doesn't own.

## Goals

- Make the owner certificate optional in the manufacturer configuration.
- Allow any valid voucher to be imported regardless of ownership.
- Only start TO0 for vouchers the server actually owns.
- Only allow extending vouchers the server owns.
- Return proper HTTP status codes (403 instead of 500) when extension is
  not possible due to ownership.

## Design

### 1. Manufacturer: optional owner certificate

**Config** (`internal/config/manufacturer.go`):

- Remove the `validateCertFile` call for the owner certificate in `Validate()`.
- `GetOwnerCertificate()` returns `(nil, nil)` when the path is empty.

**Server init** (`internal/server/manufacturing.go`):

- Treat `nil` owner certificate as valid. Only error when the path is set but
  the file is unreadable or unparsable.

**DI handler** (`api/v2/manufacturer/handler.go`):

- `BeforeVoucherPersist` hook (which calls `fdo.ExtendVoucher`): only
  registered when `OwnerCert != nil`. When nil, vouchers are stored with only
  the manufacturer key (zero ownership entries).
- V1 voucher API: pass empty `OwnerPKeys` when `OwnerCert == nil`. V1 import
  will reject vouchers in this mode — the optional-owner-cert workflow is V2
  only.
- V2 voucher `Server`: always pass an `OwnerKeyState` built from the
  manufacturer signing key. The extend endpoint uses the ownership check
  (section 4) to determine whether extension is allowed — no special
  config-based gating needed.

### 2. Voucher DB: `ownership_verified` column

**Schema** (`internal/state/voucher.go`):

- Add `OwnershipVerified bool` column to the `Voucher` struct,
  default `false`.
- GORM AutoMigrate handles the column addition.
- On first startup after upgrade, `InitVoucherDB` detects the new column
  (via `db.Migrator().HasColumn`) and sets `NeedsOwnershipMigration = true`.
  The server then calls `MigrateOwnershipVerified` which scans all existing
  vouchers, compares each owner key against the server's key, and batch-updates
  the flag. This ensures the TO0 scheduler continues processing previously-owned
  vouchers after upgrade.

**When it's set:**

| Event | Value | Reason |
|-------|-------|--------|
| Import (owner key matches) | `true` | Server owns the voucher |
| Import (owner key doesn't match or no key configured) | `false` | Server doesn't own it |
| Extend (voucher transferred to new owner) | `false` | Server no longer owns it |
| Auto-extend at DI (manufacturer with owner cert) | `false` | Manufacturer no longer owns it after extending to owner cert |

### 3. Import: accept any valid voucher

**V2 import** (`api/v2/voucher/handler.go`):

- Accept any voucher that passes `VerifyOwnershipVoucher` (header field
  validation and cryptographic integrity checks).
- Vouchers that fail parsing or verification are counted as `failed`.
- Vouchers that already exist (duplicate GUID) are counted as `skipped`.
- After storing, set `ownership_verified` by comparing the voucher's owner
  public key against the server's key (if `OwnerKeyState` is available).
  When `OwnerKeyState` is nil, `ownership_verified` defaults to `false`.

**Response status codes:**

| Condition | Status | Meaning |
|-----------|--------|---------|
| `imported ≥ 1` | **201** | At least one voucher was imported |
| `detected ≥ 1`, `imported = 0` | **200** | Vouchers detected but all skipped or failed |
| `detected = 0` | **400** | No valid ownership voucher PEM blocks found |

### 4. Extend: ownership-gated

**V2 extend** (`api/v2/voucher/handler.go`):

The extend endpoint enforces two checks:

1. `OwnerKeyState == nil` → **403 Forbidden**
   ("no signing key configured") — checked in the handler before parsing.
2. Voucher's owner public key doesn't match the server's key →
   **403 Forbidden** ("server does not own this voucher") — checked
   atomically inside `ExtendVoucher`'s database transaction to prevent
   TOCTOU races.

The request body accepts either a `PUBLIC KEY` or `CERTIFICATE` PEM block.
For certificates, the public key is extracted via `x509.ParseCertificate`.

On success:

- Extend the voucher using the server's signing key.
- Update the stored voucher with the extended version.
- Clear `ownership_verified = false` inside the `ExtendVoucher` transaction
  (atomic with the voucher replacement — no separate handler-level call).
- Return the extended voucher as PEM.

This handles both manufacturer and owner servers uniformly:

- **Manufacturer with owner cert**: vouchers are auto-extended at DI, so the
  manufacturer's key is no longer the current owner → extend returns 403.
- **Manufacturer without owner cert**: vouchers still have the manufacturer
  key as owner → extend works using the manufacturer's signing key.
- **Owner server**: extend works for owned vouchers, returns 403 for others.

### 5. TO0 scheduler: ownership-filtered

**DB query** (`internal/state/voucher.go`):

- `ListPendingTO0Vouchers` adds `AND vouchers.ownership_verified = true` to
  its WHERE clause. Only vouchers the server owns are considered for TO0.

**Scheduler** (`internal/server/owner.go`):

- No changes to the scheduler loop itself — the filtering happens at the
  query level.

### 6. Manual re-verification endpoint

**V2 API** (`api/v2/voucher/handler.go`):

- `POST /api/v2/vouchers/verify-ownership` calls `MigrateOwnershipVerified`
  and returns `{ total, owned, unowned }`.
- Returns **403 Forbidden** when no signing key is configured
  (`OwnerKeyState == nil`).
- Use case: key rotation, interrupted startup migration, or operator-triggered
  re-evaluation of ownership state.

## Files affected

This work was implemented alongside the V2 API scaffolding and shared infrastructure. The full list of modified/created files:

| Area | Files | Change |
|------|-------|--------|
| **Core feature** | `internal/config/manufacturer.go` | Make owner cert optional, return key type from `GetDeviceCAKey` |
| | `internal/server/manufacturing.go` | Handle `nil` owner cert from config |
| | `api/v2/manufacturer/handler.go` | Conditional `BeforeVoucherPersist` (column default handles ownership_verified=false for new vouchers), V1 `OwnerPKeys`, mfg key as `OwnerKeyState` |
| | `internal/state/voucher.go` | Add `OwnershipVerified` column, `SetOwnershipVerified`, `MigrateOwnershipVerified`, `NeedsOwnershipMigration` flag, `ListPendingTO0Vouchers` ownership filter, `ExtendVoucher` with atomic ownership check, typed `DeviceFilter`, paginated `ListDevices` |
| | `api/v2/voucher/handler.go` | Accept any voucher on import, set `ownership_verified` after import, ownership-gated extend (403) accepting PUBLIC KEY or CERTIFICATE PEM, verify-ownership endpoint, content negotiation |
| **V2 API scaffolding** | `api/v2/device/` | New V2 device listing API with pagination |
| | `api/v2/owner/` | New V2 owner handler (replaces `api/v1/owner/`) |
| | `api/v2/rvto2addr/` | New V2 rvto2addr API with corrected transport protocols |
| | `api/v2/components/openapi.yaml` | Shared error responses (BadRequest, Forbidden, NotFound, InternalServerError, etc.) |
| **Infrastructure** | `internal/middleware/validation.go` | OpenAPI request validation middleware |
| | `internal/middleware/openapi.go` | Shared OpenAPI spec serving helper |
| | `internal/utils/voucher.go` | Extracted shared voucher verification functions |
| | `internal/server/owner.go` | Wire V2 APIs into owner server |
| | `internal/state/ownerkey.go` | Defensive chain copy, doc comments |
| | `internal/state/deviceca.go` | Add `CertPool()`, export `IsDuplicateError`, LIKE escape fix |
| | `internal/state/manufacturing.go` | Add `DeviceCA` field |
| | `internal/state/rvto2addr.go` | Use exported `IsDuplicateError` |
| **V1 cleanup** | `api/v1/owner/` | Removed (functionality moved to `api/v2/owner/`) |
| | `api/v1/voucher/handler.go` | Use shared `utils.VerifyVoucherOwnership`, set `ownership_verified` on import |
| | `api/v1/resell/handler.go` | Updated for new `ExtendVoucher` signature |
| **Documentation** | `REVERSE_PROXY.md`, `SECURITY.md` | Updated for V2 API endpoints |
| | `docs/user-guide/*.md` | Revised certificate setup and quick-start instructions |
| | `docs/openapi-code-generation.md` | Updated code generation docs |
| | `docs/device-ca-content-negotiation.md` | Updated content negotiation docs |

## Non-goals

- V1 API changes beyond adjusting `OwnerPKeys` when owner cert is absent.
- Changing the TO0 scheduler loop logic — filtering is DB-level only.
