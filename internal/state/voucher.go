package state

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// Sentinel errors for voucher operations
var (
	ErrUnsupportedKeyType = errors.New("unsupported public key type")
)

// Compile-time check for interface implementation correctness.
// OwnerVoucherPersistentState embeds VoucherPersistentState, so this
// assertion covers both interfaces.
var _ interface {
	fdo.VoucherPersistentState
	fdo.OwnerVoucherPersistentState
} = (*VoucherPersistentState)(nil)

// VoucherPersistentState implements fdo.VoucherPersistentState and
// fdo.OwnerVoucherPersistentState.
type VoucherPersistentState struct {
	DB                      *gorm.DB
	NeedsOwnershipMigration bool
}

type GUID []byte

func (t *GUID) UnmarshalJSON(b []byte) (err error) {
	var g string
	if err = json.Unmarshal(b, &g); err != nil {
		return
	}
	*t, err = hex.DecodeString(g)
	return
}

func (t *GUID) MarshalJSON() (b []byte, err error) {
	return json.Marshal(hex.EncodeToString(*t))
}

type Voucher struct {
	GUID              GUID      `json:"guid" gorm:"primaryKey"`
	CBOR              []byte    `json:"cbor,omitempty"`
	DeviceInfo        string    `json:"device_info" gorm:"type:text"`
	OwnershipVerified bool      `json:"ownership_verified" gorm:"type:boolean;not null;default:false"`
	CreatedAt         time.Time `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

// TableName specifies the table name for Voucher model
func (Voucher) TableName() string {
	return "vouchers"
}

// DeviceOnboarding tracks TO2 completion per device GUID
type DeviceOnboarding struct {
	GUID           GUID `gorm:"primaryKey"`
	NewGUID        GUID `gorm:"index"`
	TO2Completed   bool `gorm:"type:boolean;not null;default:false"`
	TO2CompletedAt *time.Time
}

// TableName specifies the table name for DeviceOnboarding model
func (DeviceOnboarding) TableName() string {
	return "device_onboarding"
}

// Device is a projection used by the owner API to expose
// voucher metadata together with TO2 onboarding state for each device.
type Device struct {
	GUID           GUID       `json:"guid" gorm:"column:guid"`
	OldGUID        GUID       `json:"old_guid" gorm:"column:old_guid"`
	DeviceInfo     string     `json:"device_info" gorm:"column:device_info"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at"`
	TO2Completed   bool       `json:"to2_completed" gorm:"column:to2_completed"`
	TO2CompletedAt *time.Time `json:"to2_completed_at,omitempty" gorm:"column:to2_completed_at"`
}

// ReplacementVoucher stores replacement vouchers during TO2 device resale
type ReplacementVoucher struct {
	Session []byte `gorm:"primaryKey"`
	GUID    []byte
	Hmac    []byte
}

// TableName specifies the table name for ReplacementVoucher model
func (ReplacementVoucher) TableName() string {
	return "replacement_vouchers"
}

func InitVoucherDB(db *gorm.DB) (*VoucherPersistentState, error) {
	hadOwnershipColumn := db.Migrator().HasColumn(&Voucher{}, "ownership_verified")

	state := &VoucherPersistentState{
		DB: db,
	}
	// Auto-migrate all schemas
	err := state.DB.AutoMigrate(
		&Voucher{},
		&DeviceOnboarding{},
		&ReplacementVoucher{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}

	if !hadOwnershipColumn {
		state.NeedsOwnershipMigration = true
		slog.Info("New ownership_verified column detected — data migration required")
	}

	slog.Info("Voucher database initialized successfully")
	return state, nil
}

// VoucherPersistentState implementation

// NewVoucher creates and stores a voucher for a newly initialized device.
// Called by the manufacturer's DIServer during Device Initialization.
func (s VoucherPersistentState) NewVoucher(ctx context.Context, ov *fdo.Voucher) error {
	return s.AddVoucher(ctx, ov)
}

// OwnerVoucherPersistentState implementation

// AddVoucher stores the voucher of a device owned by the service
func (s VoucherPersistentState) AddVoucher(ctx context.Context, ov *fdo.Voucher) error {
	voucherBytes, err := cbor.Marshal(ov)
	if err != nil {
		return fmt.Errorf("failed to marshal voucher: %w", err)
	}

	now := time.Now()
	voucher := Voucher{
		GUID:       ov.Header.Val.GUID[:],
		DeviceInfo: ov.Header.Val.DeviceInfo,
		CBOR:       voucherBytes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&voucher).Error; err != nil {
			return err
		}
		return tx.FirstOrCreate(&DeviceOnboarding{GUID: voucher.GUID}).Error
	})
}

// ReplaceVoucher stores a new voucher, marks TO2 as completed, and records the
// GUID transition. This is the public entry point used at the end of TO2.
func (s VoucherPersistentState) ReplaceVoucher(ctx context.Context, guid protocol.GUID, ov *fdo.Voucher) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.replaceVoucherInTx(tx, guid, ov, true)
	})
}

// replaceVoucherInTx marshals the voucher and delegates to replaceVoucherInTxRaw.
func (s VoucherPersistentState) replaceVoucherInTx(tx *gorm.DB, guid protocol.GUID, ov *fdo.Voucher, markTO2Completed bool) error {
	voucherBytes, err := cbor.Marshal(ov)
	if err != nil {
		return fmt.Errorf("failed to marshal voucher: %w", err)
	}
	return replaceVoucherInTxRaw(tx, guid, ov, voucherBytes, markTO2Completed)
}

// replaceVoucherInTxRaw is the internal implementation that operates on an
// existing transaction handle using pre-marshaled CBOR bytes. When
// markTO2Completed is true the device onboarding record is marked as having
// finished TO2; when false (resell) only the GUID mapping is updated.
func replaceVoucherInTxRaw(tx *gorm.DB, guid protocol.GUID, ov *fdo.Voucher, voucherBytes []byte, markTO2Completed bool) error {
	now := time.Now()
	voucher := Voucher{
		GUID:       ov.Header.Val.GUID[:],
		DeviceInfo: ov.Header.Val.DeviceInfo,
		CBOR:       voucherBytes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Delete the old voucher row (by original GUID), then create the new voucher
	if err := tx.Where("guid = ?", guid[:]).Delete(&Voucher{}).Error; err != nil {
		return err
	}
	if err := tx.Create(&voucher).Error; err != nil {
		return err
	}

	// Build the onboarding record update
	onboarding := DeviceOnboarding{GUID: guid[:], NewGUID: ov.Header.Val.GUID[:]}
	if markTO2Completed {
		completedAt := time.Now()
		onboarding.TO2Completed = true
		onboarding.TO2CompletedAt = &completedAt
	}

	return tx.Where("guid = ?", guid[:]).
		Assign(onboarding).
		FirstOrCreate(&DeviceOnboarding{}).Error
}

// GetReplacementGUID returns the replacement GUID for a device given its old GUID
func (s *VoucherPersistentState) GetReplacementGUID(ctx context.Context, oldGuid protocol.GUID) (protocol.GUID, error) {
	var rec DeviceOnboarding
	if err := s.DB.WithContext(ctx).Where("guid = ?", oldGuid[:]).First(&rec).Error; err != nil {
		return protocol.GUID{}, err
	}
	var newGuid protocol.GUID
	copy(newGuid[:], rec.NewGUID)
	return newGuid, nil
}

// RemoveVoucher untracks a voucher, possibly by deleting it or marking it as removed
// shall we mark the voucher as removed instead of deleting it?
func (s VoucherPersistentState) RemoveVoucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
	var ov fdo.Voucher
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var voucher Voucher
		if err := tx.Where("guid = ?", guid[:]).First(&voucher).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fdo.ErrNotFound
			}
			return err
		}
		// Parse the voucher before deleting
		if err := cbor.Unmarshal(voucher.CBOR, &ov); err != nil {
			return fmt.Errorf("failed to unmarshal voucher: %w", err)
		}
		// Delete the voucher
		if err := tx.Where("guid = ?", guid[:]).Delete(&Voucher{}).Error; err != nil {
			return err
		}
		// Delete the onboarding tracking row for this GUID
		return tx.Where("guid = ?", guid[:]).Delete(&DeviceOnboarding{}).Error
	}); err != nil {
		return nil, err
	}
	return &ov, nil
}

// Voucher retrieves a voucher by GUID
func (s VoucherPersistentState) Voucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
	var voucher Voucher
	if err := s.DB.WithContext(ctx).Where("guid = ?", guid[:]).First(&voucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrNotFound
		}
		return nil, err
	}

	var ov fdo.Voucher
	if err := cbor.Unmarshal(voucher.CBOR, &ov); err != nil {
		return nil, fmt.Errorf("failed to unmarshal voucher: %w", err)
	}

	return &ov, nil
}

// ListVouchers retrieves a paginated, filtered, and sorted list of vouchers
func (s *VoucherPersistentState) ListVouchers(ctx context.Context, limit, offset int, guidFilter, deviceInfoFilter, searchFilter *string, sortBy, sortOrder string) ([]Voucher, int64, error) {
	var vouchers []Voucher
	var total int64

	query := s.DB.WithContext(ctx).Model(&Voucher{})

	// Apply filters
	if guidFilter != nil && *guidFilter != "" {
		// Convert hex string to bytes for GUID comparison
		guidBytes, err := hex.DecodeString(*guidFilter)
		if err == nil && len(guidBytes) == 16 {
			query = query.Where("guid = ?", guidBytes)
		}
	}
	if deviceInfoFilter != nil && *deviceInfoFilter != "" {
		query = query.Where("device_info = ?", *deviceInfoFilter)
	}
	if searchFilter != nil && *searchFilter != "" {
		// Escape LIKE meta-characters to prevent unintended wildcard expansion
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(*searchFilter)
		searchPattern := "%" + escaped + "%"
		hexExpr := "hex(guid)"
		if s.DB.Dialector.Name() == "postgres" {
			hexExpr = "encode(guid, 'hex')"
		}
		query = query.Where(hexExpr+" LIKE ? ESCAPE '\\' OR device_info LIKE ? ESCAPE '\\'", searchPattern, searchPattern)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count vouchers: %w", err)
	}

	// Apply sorting with allowed list validation to prevent SQL injection
	allowedSortColumns := map[string]bool{
		"created_at":  true,
		"updated_at":  true,
		"guid":        true,
		"device_info": true,
	}
	allowedSortOrders := map[string]bool{
		"asc":  true,
		"desc": true,
	}
	if sortBy == "" || !allowedSortColumns[sortBy] {
		sortBy = "created_at"
	}
	if sortOrder == "" || !allowedSortOrders[sortOrder] {
		sortOrder = "asc"
	}
	orderClause := fmt.Sprintf("%s %s", sortBy, sortOrder)
	query = query.Order(orderClause)

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&vouchers).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list vouchers: %w", err)
	}

	return vouchers, total, nil
}

// ListPendingTO0Vouchers returns vouchers whose devices have not completed TO2 yet
func (s *VoucherPersistentState) ListPendingTO0Vouchers(ctx context.Context) ([]Voucher, error) {
	var vouchers []Voucher

	// Join with device_onboarding to filter by completion state.
	// Only return vouchers whose ownership has been verified (i.e. the
	// server holds the signing key for the current owner).
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

// IsTO2Completed returns whether a device has completed TO2
func (s *VoucherPersistentState) IsTO2Completed(ctx context.Context, guid protocol.GUID) (bool, error) {
	var rec DeviceOnboarding
	if err := s.DB.WithContext(ctx).Where("guid = ?", guid[:]).First(&rec).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return rec.TO2Completed, nil
}

// Exists checks if a voucher exists in the database
func (s *VoucherPersistentState) Exists(ctx context.Context, guid protocol.GUID) (bool, error) {
	var count int64
	if err := s.DB.WithContext(ctx).Model(&Voucher{}).Where("guid = ?", guid[:]).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// SetOwnershipVerified updates the ownership_verified flag for a voucher.
func (s *VoucherPersistentState) SetOwnershipVerified(ctx context.Context, guid protocol.GUID, verified bool) error {
	result := s.DB.WithContext(ctx).Model(&Voucher{}).Where("guid = ?", guid[:]).Update("ownership_verified", verified)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fdo.ErrNotFound
	}
	return nil
}

// MigrateOwnershipVerified scans all vouchers and sets ownership_verified
// based on whether the voucher's current owner key matches ownerPubKey.
// Returns the number of owned and total vouchers processed.
func (s *VoucherPersistentState) MigrateOwnershipVerified(ctx context.Context, ownerPubKey crypto.PublicKey) (owned, total int, err error) {
	if ownerPubKey == nil {
		slog.Info("No owner key provided — skipping ownership migration")
		s.NeedsOwnershipMigration = false
		return 0, 0, nil
	}

	var vouchers []Voucher
	if err := s.DB.WithContext(ctx).Select("guid", "cbor").Find(&vouchers).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to query vouchers: %w", err)
	}

	total = len(vouchers)
	var ownedGUIDs [][]byte

	for _, v := range vouchers {
		var ov fdo.Voucher
		if err := cbor.Unmarshal(v.CBOR, &ov); err != nil {
			slog.Warn("Skipping voucher with corrupt CBOR during migration", "guid", hex.EncodeToString(v.GUID), "error", err)
			continue
		}
		voucherOwnerKey, err := ov.OwnerPublicKey()
		if err != nil {
			slog.Warn("Skipping voucher — cannot extract owner key", "guid", hex.EncodeToString(v.GUID), "error", err)
			continue
		}
		if utils.PublicKeysEqual(voucherOwnerKey, ownerPubKey) {
			ownedGUIDs = append(ownedGUIDs, v.GUID)
		}
	}

	// Reset all to false, then set owned ones to true
	if err := s.DB.WithContext(ctx).Model(&Voucher{}).
		Where("1 = 1").
		Update("ownership_verified", false).Error; err != nil {
		return 0, total, fmt.Errorf("failed to reset ownership_verified: %w", err)
	}

	if len(ownedGUIDs) > 0 {
		if err := s.DB.WithContext(ctx).Model(&Voucher{}).
			Where("guid IN ?", ownedGUIDs).
			Update("ownership_verified", true).Error; err != nil {
			return 0, total, fmt.Errorf("failed to update ownership_verified: %w", err)
		}
	}

	owned = len(ownedGUIDs)
	s.NeedsOwnershipMigration = false
	slog.Info("Ownership migration completed", "total", total, "owned", owned)
	return owned, total, nil
}

// ErrNotOwner is returned when the server does not own the voucher.
var ErrNotOwner = errors.New("server does not own this voucher")

// ExtendVoucher extends a voucher with a new owner's public key (resell operation).
// This operation is performed in a transaction to ensure atomicity:
// if the extension fails, the original voucher is preserved.
//
// The ownership check (comparing the voucher's owner key against ownerPubKey)
// runs inside the transaction to prevent TOCTOU races.
//
// Returns both the extended voucher and its CBOR-encoded bytes (the exact bytes
// persisted in the database) so callers can use them directly without re-marshaling.
func (s *VoucherPersistentState) ExtendVoucher(
	ctx context.Context,
	guid protocol.GUID,
	currentOwnerKey crypto.Signer,
	ownerPubKey crypto.PublicKey,
	nextOwnerKey crypto.PublicKey,
) (*fdo.Voucher, []byte, error) {
	var extended *fdo.Voucher
	var extendedCBOR []byte

	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var voucherRow Voucher
		if err := tx.Where("guid = ?", guid[:]).First(&voucherRow).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fdo.ErrNotFound
			}
			return err
		}
		var voucher fdo.Voucher
		if err := cbor.Unmarshal(voucherRow.CBOR, &voucher); err != nil {
			return fmt.Errorf("failed to unmarshal voucher: %w", err)
		}

		// Verify ownership inside the transaction to prevent TOCTOU races.
		voucherOwnerKey, err := voucher.OwnerPublicKey()
		if err != nil {
			return fmt.Errorf("failed to extract owner public key: %w", err)
		}
		if !utils.PublicKeysEqual(voucherOwnerKey, ownerPubKey) {
			return ErrNotOwner
		}

		// Type-assert the public key to a concrete type that satisfies
		// the protocol.PublicKeyOrChain constraint.
		switch key := nextOwnerKey.(type) {
		case *ecdsa.PublicKey:
			extended, err = fdo.ExtendVoucher(&voucher, currentOwnerKey, key, nil)
		case *rsa.PublicKey:
			extended, err = fdo.ExtendVoucher(&voucher, currentOwnerKey, key, nil)
		default:
			return fmt.Errorf("%w: %T", ErrUnsupportedKeyType, nextOwnerKey)
		}
		if err != nil {
			return fmt.Errorf("failed to extend voucher: %w", err)
		}

		extendedCBOR, err = cbor.Marshal(extended)
		if err != nil {
			return fmt.Errorf("failed to marshal extended voucher: %w", err)
		}
		// Replace old voucher with extended one in the database.
		// Pass markTO2Completed=false: reselling transfers ownership but does
		// NOT complete TO2 — the device still needs to onboard with the new owner.
		if err := replaceVoucherInTxRaw(tx, guid, extended, extendedCBOR, false); err != nil {
			return err
		}
		// Clear ownership_verified on the new voucher — the new owner
		// must re-verify ownership before the voucher is eligible for TO0.
		return tx.Model(&Voucher{}).Where("guid = ?", extended.Header.Val.GUID[:]).Update("ownership_verified", false).Error
	})

	return extended, extendedCBOR, err
}

// DeviceFilter holds typed filter parameters for listing devices.
type DeviceFilter struct {
	OldGUID []byte
}

// ListDevices retrieves a list of devices (vouchers joined with onboarding status)
func (s *VoucherPersistentState) ListDevices(ctx context.Context, filter DeviceFilter, limit, offset int) ([]Device, int64, error) {
	var devices []Device
	var total int64

	query := s.DB.WithContext(ctx).Table("vouchers").
		Select("vouchers.guid, device_onboarding.guid as old_guid, vouchers.device_info, vouchers.created_at, vouchers.updated_at, device_onboarding.to2_completed, device_onboarding.to2_completed_at").
		Joins("LEFT JOIN device_onboarding ON device_onboarding.new_guid = vouchers.guid")

	if filter.OldGUID != nil {
		query = query.Where("device_onboarding.guid = ?", filter.OldGUID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count devices: %w", err)
	}

	query = query.Order("vouchers.updated_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Scan(&devices).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list devices: %w", err)
	}

	return devices, total, nil
}
