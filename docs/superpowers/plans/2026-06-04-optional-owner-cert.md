# Optional Owner Certificate & Ownership-Aware Vouchers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the manufacturer's owner certificate optional and add ownership-aware voucher handling (import any voucher, extend (403)/TO0 only owned ones).

**Architecture:** Add an `ownership_verified` boolean column to the vouchers table. Import accepts any valid voucher and sets the flag based on key comparison. The extend endpoint checks the flag (403 if not owned). The TO0 scheduler query filters on the flag. The manufacturer config makes the owner cert optional, conditionally registering the DI auto-extend hook.

**Tech Stack:** Go, GORM (AutoMigrate), oapi-codegen (OpenAPI code generation)

**Spec:** `docs/superpowers/specs/2026-06-03-optional-owner-cert-design.md`

---

### Task 1: Add `ownership_verified` column and update TO0 query

**Files:**
- Modify: `internal/state/voucher.go:52-58` (Voucher struct)
- Modify: `internal/state/voucher.go:326-339` (ListPendingTO0Vouchers)
- Modify: `internal/state/voucher.go:370-421` (ExtendVoucher)
- Create: `internal/state/voucher_ownership_test.go`

- [ ] **Step 1: Write the failing test for SetOwnershipVerified**

Create `internal/state/voucher_ownership_test.go`:

```go
package state

import (
	"context"
	"testing"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestVoucherDB(t *testing.T) *VoucherPersistentState {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	state, err := InitVoucherDB(db)
	if err != nil {
		t.Fatalf("Failed to init voucher DB: %v", err)
	}
	return state
}

func TestSetOwnershipVerified(t *testing.T) {
	state := setupTestVoucherDB(t)
	ctx := context.Background()

	// Insert a raw voucher row
	guid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	row := Voucher{GUID: guid, CBOR: []byte("test"), DeviceInfo: "test"}
	if err := state.DB.Create(&row).Error; err != nil {
		t.Fatalf("Failed to create test voucher: %v", err)
	}

	// Default should be false
	var v Voucher
	state.DB.Where("guid = ?", guid).First(&v)
	if v.OwnershipVerified {
		t.Fatal("Expected ownership_verified to default to false")
	}

	// Set to true
	var g protocol.GUID
	copy(g[:], guid)
	if err := state.SetOwnershipVerified(ctx, g, true); err != nil {
		t.Fatalf("SetOwnershipVerified(true) failed: %v", err)
	}
	state.DB.Where("guid = ?", guid).First(&v)
	if !v.OwnershipVerified {
		t.Fatal("Expected ownership_verified to be true after SetOwnershipVerified(true)")
	}

	// Set back to false
	if err := state.SetOwnershipVerified(ctx, g, false); err != nil {
		t.Fatalf("SetOwnershipVerified(false) failed: %v", err)
	}
	state.DB.Where("guid = ?", guid).First(&v)
	if v.OwnershipVerified {
		t.Fatal("Expected ownership_verified to be false after SetOwnershipVerified(false)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -run TestSetOwnershipVerified -v`
Expected: FAIL — `OwnershipVerified` field and `SetOwnershipVerified` method don't exist.

- [ ] **Step 3: Add `OwnershipVerified` field and `SetOwnershipVerified` method**

In `internal/state/voucher.go`, add the field to the `Voucher` struct:

```go
type Voucher struct {
	GUID                GUID      `json:"guid" gorm:"primaryKey"`
	CBOR                []byte    `json:"cbor,omitempty"`
	DeviceInfo          string    `json:"device_info" gorm:"type:text"`
	OwnershipVerified   bool      `json:"ownership_verified" gorm:"type:boolean;not null;default:false"`
	CreatedAt           time.Time `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime:milli"`
}
```

Add the method:

```go
// SetOwnershipVerified updates the ownership_verified flag for a voucher.
func (s *VoucherPersistentState) SetOwnershipVerified(ctx context.Context, guid protocol.GUID, verified bool) error {
	return s.DB.WithContext(ctx).Model(&Voucher{}).Where("guid = ?", guid[:]).Update("ownership_verified", verified).Error
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/state/ -run TestSetOwnershipVerified -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for ListPendingTO0Vouchers ownership filter**

Append to `internal/state/voucher_ownership_test.go`:

```go
func TestListPendingTO0Vouchers_OnlyOwned(t *testing.T) {
	state := setupTestVoucherDB(t)
	ctx := context.Background()

	// Insert two voucher rows with onboarding records (to2_completed=false)
	guid1 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	guid2 := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	state.DB.Create(&Voucher{GUID: guid1, CBOR: []byte("v1"), DeviceInfo: "dev1", OwnershipVerified: true})
	state.DB.Create(&Voucher{GUID: guid2, CBOR: []byte("v2"), DeviceInfo: "dev2", OwnershipVerified: false})
	state.DB.Create(&DeviceOnboarding{GUID: guid1})
	state.DB.Create(&DeviceOnboarding{GUID: guid2})

	vouchers, err := state.ListPendingTO0Vouchers(ctx)
	if err != nil {
		t.Fatalf("ListPendingTO0Vouchers failed: %v", err)
	}

	// Only guid1 (ownership_verified=true) should be returned
	if len(vouchers) != 1 {
		t.Fatalf("Expected 1 voucher, got %d", len(vouchers))
	}
	if string(vouchers[0].GUID) != string(guid1) {
		t.Fatalf("Expected guid1, got %x", vouchers[0].GUID)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/state/ -run TestListPendingTO0Vouchers_OnlyOwned -v`
Expected: FAIL — both vouchers are returned (no ownership_verified filter yet).

- [ ] **Step 7: Update `ListPendingTO0Vouchers` to filter by `ownership_verified`**

In `internal/state/voucher.go`, update `ListPendingTO0Vouchers`:

```go
func (s *VoucherPersistentState) ListPendingTO0Vouchers(ctx context.Context) ([]Voucher, error) {
	var vouchers []Voucher

	err := s.DB.WithContext(ctx).Model(&Voucher{}).
		Joins("JOIN device_onboarding ON device_onboarding.guid = vouchers.guid").
		Where("device_onboarding.to2_completed = ?", false).
		Where("vouchers.ownership_verified = ?", true).
		Find(&vouchers).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list pending TO0 vouchers: %w", err)
	}

	return vouchers, nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/state/ -run TestListPendingTO0Vouchers_OnlyOwned -v`
Expected: PASS

- [ ] **Step 9: Update `ExtendVoucher` to clear `ownership_verified`**

In `internal/state/voucher.go`, in the `ExtendVoucher` method, after `replaceVoucherInTxRaw`, add:

```go
		if err := replaceVoucherInTxRaw(tx, guid, extended, extendedCBOR, false); err != nil {
			return err
		}
		return tx.Model(&Voucher{}).Where("guid = ?", extended.Header.Val.GUID[:]).Update("ownership_verified", false).Error
```

- [ ] **Step 10: Run all state tests**

Run: `go test ./internal/state/ -v`
Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add internal/state/voucher.go internal/state/voucher_ownership_test.go
git commit --signoff -m "feat: add ownership_verified column to vouchers table

Add a boolean ownership_verified column (default false) that tracks
whether the server holds the signing key for the voucher's current
owner. ListPendingTO0Vouchers now filters on this flag so only owned
vouchers are considered for TO0. ExtendVoucher clears the flag when
a voucher is transferred to a new owner."
```

---

### Task 2: Use 403 Forbidden for ownership-gated extend responses

**Note:** The extend endpoint uses 403 Forbidden (not 405 Method Not Allowed)
for ownership failures. The 403 response is already defined in the shared
components and referenced from the extend endpoint. No additional response
types are needed — the existing `ExtendOwnershipVoucher403JSONResponse`
(generated from the Forbidden response ref) is used for both "no signing key
configured" and "server does not own this voucher" cases.

---

### Task 3: Update V2 import to accept any voucher and set `ownership_verified`

**Files:**
- Modify: `api/v2/voucher/handler.go:163-270` (ImportOwnershipVouchers)
- Modify: `api/v2/voucher/import_test.go`

- [ ] **Step 1: Write a failing test for importing a non-owned voucher**

In `api/v2/voucher/import_test.go`, add a test that creates a voucher owned by a different key and verifies import succeeds (currently it's rejected). The exact test structure depends on the existing test helpers — follow the pattern already used in the file. The test should:

1. Generate a manufacturer key and a separate "other owner" key.
2. Create and extend a voucher to the "other owner" key.
3. Import it — assert 201 with `imported: 1` (currently fails with `skipped: 1`).
4. Check that `ownership_verified` is `false` in the DB.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/v2/voucher/ -run TestImportNonOwnedVoucher -v`
Expected: FAIL — voucher is skipped due to ownership check.

- [ ] **Step 3: Update import handler to remove ownership gate and set flag**

In `api/v2/voucher/handler.go`, in `ImportOwnershipVouchers`:

1. Remove the `VerifyVoucherOwnership` block (lines ~224-232) that skips vouchers not owned by this server.

2. After a successful `AddVoucher` call, determine and set `ownership_verified`:

```go
		err := s.VoucherState.AddVoucher(ctx, p.voucher)
		if err != nil {
			// ... existing error handling ...
			continue
		}

		// Determine ownership: compare voucher's owner key to server's key
		owned := false
		if s.OwnerKeyState != nil {
			ownerPubKey, pubKeyErr := p.voucher.OwnerPublicKey()
			if pubKeyErr == nil {
				owned = utils.PublicKeysEqual(ownerPubKey, s.OwnerKeyState.Signer().Public())
			}
		}
		if err := s.VoucherState.SetOwnershipVerified(ctx, p.voucher.Header.Val.GUID, owned); err != nil {
			slog.Error("Failed to set ownership_verified", "guid", guid, "error", err)
		}
```

3. Keep the existing `VerifyOwnershipVoucher` integrity check — that stays.

- [ ] **Step 4: Run import tests to verify they pass**

Run: `go test ./api/v2/voucher/ -run TestImport -v`
Expected: PASS — both owned and non-owned vouchers are imported.

- [ ] **Step 5: Write a test verifying `ownership_verified` is true for owned imports**

Add a test that imports a voucher owned by the server's key and checks the DB flag is `true`.

- [ ] **Step 6: Run and verify**

Run: `go test ./api/v2/voucher/ -run TestImport -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add api/v2/voucher/handler.go api/v2/voucher/import_test.go
git commit --signoff -m "feat: accept any valid voucher on import and track ownership

Remove the ownership gate from V2 voucher import — any voucher that
passes integrity verification is now accepted. After storing, the
handler compares the voucher's owner public key against the server's
key and sets ownership_verified accordingly."
```

---

### Task 4: Update V2 extend to enforce ownership check with 403

**Files:**
- Modify: `api/v2/voucher/handler.go` (ExtendOwnershipVoucher)
- Modify: `api/v2/voucher/voucher_test.go`

The extend endpoint uses `ExtendOwnershipVoucher403JSONResponse` (Forbidden)
for two cases:
1. `OwnerKeyState == nil` → "No signing key configured"
2. Voucher's owner key doesn't match server's key → "Server does not own this voucher"

The request body accepts both `PUBLIC KEY` and `CERTIFICATE` PEM blocks.
For certificates, the public key is extracted via `x509.ParseCertificate`.

The `ownership_verified` flag is cleared inside the `ExtendVoucher` transaction
(no separate handler-level call needed).

---

### Task 5: Make owner certificate optional in manufacturer config

**Files:**
- Modify: `internal/config/manufacturer.go:58-83` (Validate, GetOwnerCertificate)
- Create: `internal/config/manufacturer_test.go`

- [ ] **Step 1: Write a failing test for Validate with empty owner cert**

Create `internal/config/manufacturer_test.go`:

```go
package config

import (
	"os"
	"testing"
)

func TestManufacturerConfig_Validate_OwnerCertOptional(t *testing.T) {
	// Create temp files for required certs/keys
	mfgKeyFile, _ := os.CreateTemp("", "mfg-key-*.pem")
	defer os.Remove(mfgKeyFile.Name())
	mfgKeyFile.Close()

	deviceCAKeyFile, _ := os.CreateTemp("", "device-ca-key-*.pem")
	defer os.Remove(deviceCAKeyFile.Name())
	deviceCAKeyFile.Close()

	deviceCACertFile, _ := os.CreateTemp("", "device-ca-cert-*.pem")
	defer os.Remove(deviceCACertFile.Name())
	deviceCACertFile.Close()

	cfg := ManufacturingServerConfig{
		Manufacturer: ManufacturingConfig{ManufacturerKeyPath: mfgKeyFile.Name()},
		DeviceCA:     DeviceCAConfig{KeyPath: deviceCAKeyFile.Name(), CertPath: deviceCACertFile.Name()},
		Owner:        OwnerConfig{OwnerCertificate: ""}, // empty — should be valid
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() should succeed with empty owner cert, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestManufacturerConfig_Validate_OwnerCertOptional -v`
Expected: FAIL — `validateCertFile` rejects the empty path.

- [ ] **Step 3: Update Validate and GetOwnerCertificate**

In `internal/config/manufacturer.go`, in `Validate()`, remove the owner cert validation block (lines ~76-78):

```go
	// Owner certificate is optional — when absent, vouchers are not
	// auto-extended during DI and must be extended via the API.
```

In `GetOwnerCertificate()`, add an early return for empty path:

```go
func (m *ManufacturingServerConfig) GetOwnerCertificate() (*x509.Certificate, error) {
	if m.Owner.OwnerCertificate == "" {
		return nil, nil
	}
	slog.Debug("Loading owner certificate", "path", m.Owner.OwnerCertificate)
	// ... rest unchanged ...
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestManufacturerConfig_Validate_OwnerCertOptional -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/manufacturer.go internal/config/manufacturer_test.go
git commit --signoff -m "feat: make owner certificate optional in manufacturer config

Remove the validateCertFile check for the owner certificate in
Validate(). GetOwnerCertificate() now returns (nil, nil) when the
path is empty. When no owner cert is configured, vouchers are not
auto-extended during DI."
```

---

### Task 6: Wire optional owner cert through manufacturer handler and server

**Files:**
- Modify: `internal/server/manufacturing.go:56-59` (handle nil owner cert)
- Modify: `api/v2/manufacturer/handler.go:49-57` (NewManufacturer signature)
- Modify: `api/v2/manufacturer/handler.go:79-163` (Handler — conditional hooks, V1/V2 wiring)

- [ ] **Step 1: Update `internal/server/manufacturing.go` to handle nil owner cert**

Change the owner cert loading block:

```go
	var ownerCert *x509.Certificate
	ownerCert, err = config.GetOwnerCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to load owner certificate: %w", err)
	}
	if ownerCert != nil {
		slog.Info("Owner certificate configured — vouchers will be auto-extended during DI")
	} else {
		slog.Info("No owner certificate configured — vouchers must be extended via the API")
	}
```

Also pass the manufacturer key to the `Manufacturer` struct so the extend endpoint can use it. Update the `NewManufacturer` call:

```go
	mfg := manufacturer.NewManufacturer(gormDB, mfgKey, deviceKey, deviceCACerts, ownerCert)
```

The manufacturer key (`mfgKey`) is already passed — it's used in `DeviceInfo` and `BeforeVoucherPersist`. The `OwnerKeyState` for the V2 voucher server needs to be built from the mfg key. This wiring happens in `Handler()`.

- [ ] **Step 2: Update manufacturer handler to conditionally register the DI hook**

In `api/v2/manufacturer/handler.go`, make `BeforeVoucherPersist` conditional on `OwnerCert != nil`:

```go
	fdoHandler := &fdo_http.Handler{
		Tokens: m.State.Token,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               m.State.DISession,
			Vouchers:              m.State.Voucher,
			SignDeviceCertificate: custom.SignDeviceCertificate(m.DeviceKey, m.DeviceCACerts),
			DeviceInfo: func(ctx context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.PublicKey, error) {
				mfgPubKey, err := utils.EncodePublicKey(info.KeyType, info.KeyEncoding, m.MfgKey.Public(), nil)
				if err != nil {
					return "", protocol.PublicKey{}, err
				}
				return info.DeviceInfo, *mfgPubKey, nil
			},
			RvInfo: func(ctx context.Context, _ *fdo.Voucher) ([][]protocol.RvInstruction, error) {
				return m.State.RvInfo.GetRvInfo(ctx)
			},
		},
	}

	if m.OwnerCert != nil {
		fdoHandler.DIResponder.(*fdo.DIServer[custom.DeviceMfgInfo]).BeforeVoucherPersist = func(ctx context.Context, ov *fdo.Voucher) error {
			extended, err := fdo.ExtendVoucher(ov, m.MfgKey, []*x509.Certificate{m.OwnerCert}, nil)
			if err != nil {
				return err
			}
			*ov = *extended
			return nil
		}
	}
```

- [ ] **Step 3: Build the V2 voucher server with manufacturer key as OwnerKeyState**

In the V2 wiring section of `Handler()`, replace the nil `OwnerKeyState` with one built from the manufacturer key. You need to determine the manufacturer key type. Update `NewManufacturer` to also accept the key type:

In the `Manufacturer` struct, add `MfgKeyType protocol.KeyType`.

Update `NewManufacturer`:

```go
func NewManufacturer(db *gorm.DB, mfgKey crypto.Signer, mfgKeyType protocol.KeyType, deviceKey crypto.Signer, deviceCACerts []*x509.Certificate, ownerCert *x509.Certificate) Manufacturer {
	return Manufacturer{
		DB:            db,
		MfgKey:        mfgKey,
		MfgKeyType:    mfgKeyType,
		DeviceKey:     deviceKey,
		DeviceCACerts: deviceCACerts,
		OwnerCert:     ownerCert,
	}
}
```

Update `internal/server/manufacturing.go` call site:

```go
	mfg := manufacturer.NewManufacturer(gormDB, mfgKey, mfgKeyType, deviceKey, deviceCACerts, ownerCert)
```

Where `mfgKeyType` comes from the second return value of `config.GetManufacturerKey()` (already returned but ignored with `_`):

```go
	mfgKey, mfgKeyType, err := config.GetManufacturerKey()
```

Then in the V2 voucher wiring:

```go
	mfgOwnerKeyState := state.NewOwnerKeyPersistentState(m.MfgKey, m.MfgKeyType, nil)
	voucherServerV2 := v2voucher.NewServer(m.State.Voucher, m.State.DeviceCA, mfgOwnerKeyState)
```

- [ ] **Step 4: Update V1 voucher API wiring for nil owner cert**

In `Handler()`, the V1 voucher server currently dereferences `m.OwnerCert.PublicKey`. Guard it:

```go
	var ownerPKeys []crypto.PublicKey
	if m.OwnerCert != nil {
		ownerPKeys = []crypto.PublicKey{m.OwnerCert.PublicKey}
	}
	voucherServerV1 := v1voucher.NewServer(m.State.Voucher, ownerPKeys)
```

- [ ] **Step 5: Build and run all tests**

Run: `make all shfmt && go test ./... 2>&1 | tail -20`
Expected: clean build, all tests pass, no diff from `make all shfmt`.

- [ ] **Step 6: Commit**

```bash
git add internal/server/manufacturing.go api/v2/manufacturer/handler.go
git commit --signoff -m "feat: wire optional owner cert through manufacturer server

When the owner certificate is configured, the DI handler auto-extends
vouchers (existing behavior). When absent, vouchers are stored with
the manufacturer key only.

The V2 voucher server now receives an OwnerKeyState built from the
manufacturer signing key, so the extend endpoint can use it to extend
vouchers the manufacturer owns. V1 gets an empty OwnerPKeys slice
when no owner cert is set (V1 import rejects, as designed)."
```

---

### Task 7: One-time ownership migration on schema upgrade

**Files:**
- Modify: `internal/state/voucher.go` (add `NeedsOwnershipMigration` flag, `MigrateOwnershipVerified` method, column detection in `InitVoucherDB`)
- Modify: `internal/state/voucher_ownership_test.go` (add migration tests)
- Modify: `internal/server/owner.go` (call migration on startup)
- Modify: `internal/server/manufacturing.go` (call migration on startup)
- Modify: `api/v2/voucher/openapi.yaml` (add verify-ownership endpoint)
- Modify: `api/v2/voucher/handler.go` (implement `VerifyOwnership` handler)

`InitVoucherDB` checks `db.Migrator().HasColumn(&Voucher{}, "ownership_verified")`
before `AutoMigrate`. If the column is new, sets `NeedsOwnershipMigration = true`.
After the owner key is loaded, the server calls `MigrateOwnershipVerified` which
scans all vouchers, compares owner keys, and batch-updates the flag.

`POST /api/v2/vouchers/verify-ownership` exposes the same method for manual
re-verification (key rotation, interrupted migration).

---

### Task 8: Verify end-to-end behavior and clean up

**Files:**
- No new files — verification and final test pass

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v 2>&1 | tail -30`
Expected: all tests pass.

- [ ] **Step 2: Verify `make all shfmt` produces no drift**

Run: `make all shfmt && git diff --stat`
Expected: no uncommitted changes.

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 4: Review the complete commit history**

Run: `git log --oneline HEAD~6..HEAD`
Expected: 6 clean commits matching the task structure above.
