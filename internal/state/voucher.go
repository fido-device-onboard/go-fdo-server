package state

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.VoucherPersistentState
	fdo.OwnerVoucherPersistentState
} = (*VoucherPersistentState)(nil)

// VoucherPersistentState implements
type VoucherPersistentState struct {
	Token *TokenService
	DB    *gorm.DB
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
	GUID       GUID      `json:"guid" gorm:"primaryKey"`
	CBOR       []byte    `json:"cbor,omitempty"`
	DeviceInfo string    `json:"device_info" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime:milli"`
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

	slog.Info("Voucher database initialized successfully")
	return state, nil
}

// ManufacturerVoucherPersistentState implementation

// NewVoucher creates and stores a voucher for a newly initialized device
func (s VoucherPersistentState) NewVoucher(ctx context.Context, ov *fdo.Voucher) error {
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

	return s.DB.Create(&voucher).Error
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

	return s.DB.Create(&voucher).Error
}

// ReplaceVoucher stores a new voucher, possibly deleting or marking the previous voucher as replaced
func (s VoucherPersistentState) ReplaceVoucher(ctx context.Context, guid protocol.GUID, ov *fdo.Voucher) error {
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

	// Mark TO2 completion for this GUID and record new GUID that changed
	completedAt := time.Now()
	replacement := DeviceOnboarding{GUID: guid[:], NewGUID: ov.Header.Val.GUID[:], TO2Completed: true, TO2CompletedAt: &completedAt}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		// Delete the old voucher row (by original GUID), then create the new voucher
		if err := tx.Where("guid = ?", guid[:]).Delete(&Voucher{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&voucher).Error; err != nil {
			return err
		}
		// Update onboarding completion and new GUID
		return tx.Where("guid = ?", guid[:]).
			Assign(replacement).
			FirstOrCreate(&DeviceOnboarding{}).Error
	})
}

// RemoveVoucher untracks a voucher, possibly by deleting it or marking it as removed
// TODO: we should mark the voucher as removed instead of deleting it
// GetReplacementGUID returns the replacement GUID for a device given its old GUID
func (s *VoucherPersistentState) GetReplacementGUID(ctx context.Context, oldGuid protocol.GUID) (protocol.GUID, error) {
	var rec DeviceOnboarding
	if err := s.DB.Where("guid = ?", oldGuid[:]).First(&rec).Error; err != nil {
		return protocol.GUID{}, err
	}
	var newGuid protocol.GUID
	copy(newGuid[:], rec.NewGUID)
	return newGuid, nil
}

func (s VoucherPersistentState) RemoveVoucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
	var ov fdo.Voucher
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
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
		// Delete the onboarding tracking row for this GUID (best-effort)
		return tx.Where("guid = ?", guid[:]).Delete(&DeviceOnboarding{}).Error
	}); err != nil {
		return nil, err
	}
	return &ov, nil
}

// Voucher retrieves a voucher by GUID
func (s VoucherPersistentState) Voucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
	var voucher Voucher
	if err := s.DB.Where("guid = ?", guid[:]).First(&voucher).Error; err != nil {
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
		searchPattern := "%" + *searchFilter + "%"
		// Search in both GUID (as hex string) and device_info
		// Use hex() for SQLite, encode(guid, 'hex') for PostgreSQL
		// GORM will use the appropriate dialect
		query = query.Where("hex(guid) LIKE ? OR device_info LIKE ?", searchPattern, searchPattern)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count vouchers: %w", err)
	}

	// Apply sorting
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
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

	// Join with device_onboarding to filter by completion state
	err := s.DB.WithContext(ctx).Model(&Voucher{}).
		Joins("LEFT JOIN device_onboarding ON device_onboarding.guid = vouchers.guid").
		Where("device_onboarding.to2_completed = ? OR device_onboarding.guid IS NULL", false).
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
