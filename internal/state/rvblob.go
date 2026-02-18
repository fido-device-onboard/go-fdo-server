package state

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.RendezvousBlobPersistentState
} = (*RendezvousBlobPersistentState)(nil)

// RendezvousBlobPersistentState implementation

type RendezvousBlobPersistentState struct {
	Token *TokenService
	DB    *gorm.DB
}

// RvBlob stores rendezvous blobs
type RvBlob struct {
	GUID    []byte    `gorm:"primaryKey"`
	RV      []byte    `gorm:"not null"`
	Voucher []byte    `gorm:"not null"`
	Exp     time.Time `gorm:"not null;index:idx_rv_blob_exp"`
}

// TableName specifies the table name for RvBlob model
func (RvBlob) TableName() string {
	return "rv_blobs"
}

func InitRendezvousBlobDB(db *gorm.DB) (*RendezvousBlobPersistentState, error) {
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	state := &RendezvousBlobPersistentState{
		Token: tokenServiceState,
		DB:    db,
	}
	// Auto-migrate all schemas
	err = state.DB.AutoMigrate(
		&RvBlob{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Debug("Rendezvous Blob Database initialized successfully")
	return state, nil
}

// SetRVBlob sets the owner rendezvous blob for a device
func (s RendezvousBlobPersistentState) SetRVBlob(ctx context.Context, voucher *fdo.Voucher, to1d *cose.Sign1[protocol.To1d, []byte], exp time.Time) error {
	rvBytes, err := cbor.Marshal(to1d)
	if err != nil {
		return fmt.Errorf("failed to marshal rv blob: %w", err)
	}

	voucherBytes, err := cbor.Marshal(voucher)
	if err != nil {
		return fmt.Errorf("failed to marshal voucher: %w", err)
	}

	rvBlob := RvBlob{
		GUID:    voucher.Header.Val.GUID[:],
		RV:      rvBytes,
		Voucher: voucherBytes,
		Exp:     exp,
	}

	return s.DB.WithContext(ctx).Save(&rvBlob).Error
}

// RVBlob returns the owner rendezvous blob for a device
func (s RendezvousBlobPersistentState) RVBlob(ctx context.Context, guid protocol.GUID) (*cose.Sign1[protocol.To1d, []byte], *fdo.Voucher, error) {
	var rvBlob RvBlob
	if err := s.DB.WithContext(ctx).Where("guid = ?", guid[:]).First(&rvBlob).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fdo.ErrNotFound
		}
		return nil, nil, err
	}

	// Check if expired
	if time.Now().After(rvBlob.Exp) {
		return nil, nil, fdo.ErrNotFound
	}

	var to1d cose.Sign1[protocol.To1d, []byte]
	if err := cbor.Unmarshal(rvBlob.RV, &to1d); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal rv blob: %w", err)
	}

	var voucher fdo.Voucher
	if err := cbor.Unmarshal(rvBlob.Voucher, &voucher); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal voucher: %w", err)
	}

	return &to1d, &voucher, nil
}

// CleanupExpiredBlobs deletes all rendezvous blobs that have expired
// Returns the number of deleted blobs and any error encountered
func (s RendezvousBlobPersistentState) CleanupExpiredBlobs(ctx context.Context) (int64, error) {
	result := s.DB.WithContext(ctx).Where("exp < ?", time.Now()).Delete(&RvBlob{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired rv blobs: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		slog.Debug("Cleaned up expired rendezvous blobs", "count", result.RowsAffected)
	}
	return result.RowsAffected, nil
}
