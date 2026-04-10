package state

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

var _ interface {
	fdo.DISessionState
} = (*DISessionState)(nil)

// DISessionState implementation
type DISessionState struct {
	Token *TokenService
	DB    *gorm.DB
}

// DeviceInfo stores device information
type DeviceInfo struct {
	Session      []byte  `gorm:"index"`
	KeyType      *int    `gorm:"type:integer"`
	KeyEncoding  *int    `gorm:"type:integer"`
	SerialNumber *string `gorm:"type:text"`
	InfoString   *string `gorm:"type:text"`
	CSR          []byte
	X509Chain    []byte `gorm:"not null"`
}

// TableName specifies the table name for DeviceInfo model
func (DeviceInfo) TableName() string {
	return "device_info"
}

// IncompleteVoucher stores incomplete voucher headers
type IncompleteVoucher struct {
	Session []byte `gorm:"primaryKey"`
	Header  []byte `gorm:"not null"`
}

// TableName specifies the table name for IncompleteVoucher model
func (IncompleteVoucher) TableName() string {
	return "incomplete_vouchers"
}

func InitDISessionDB(db *gorm.DB) (*DISessionState, error) {
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	state := &DISessionState{
		Token: tokenServiceState,
		DB:    db,
	}
	// Auto-migrate all schemas
	err = state.DB.AutoMigrate(
		&DeviceInfo{},
		&IncompleteVoucher{},
		&ReplacementVoucher{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Info("Device Initialization database initialized successfully")
	return state, nil
}

func (s *DISessionState) getSessionID(ctx context.Context) ([]byte, error) {
	token, ok := s.Token.TokenFromContext(ctx)
	if !ok {
		return nil, fdo.ErrInvalidSession
	}

	sessionID, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fdo.ErrInvalidSession
	}

	// Verify session exists
	var session Session
	if err := s.Token.DB.Where("id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrInvalidSession
		}
		return nil, err
	}

	return sessionID, nil
}

// SetDeviceCertChain stores the device certificate chain
func (s DISessionState) SetDeviceCertChain(ctx context.Context, chain []*x509.Certificate) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	// Marshal the certificate chain
	chainBytes := make([]byte, 0)
	for _, cert := range chain {
		chainBytes = append(chainBytes, cert.Raw...)
	}

	// Update or create device info
	deviceInfo := DeviceInfo{
		Session:   sessionID,
		X509Chain: chainBytes,
	}

	return s.Token.DB.Where("session = ?", sessionID).
		Assign(map[string]interface{}{"x509_chain": chainBytes}).
		FirstOrCreate(&deviceInfo).Error
}

func (s DISessionState) GetReplacementGUID(ctx context.Context) (protocol.GUID, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.GUID{}, err
	}

	var replacementVoucher ReplacementVoucher
	if err = s.Token.DB.Where("session = ?", sessionID).First(&replacementVoucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.GUID{}, fdo.ErrNotFound
		}
		return protocol.GUID{}, err
	}
	return protocol.GUID(replacementVoucher.GUID), nil
}

// DeviceCertChain retrieves the device certificate chain
func (s DISessionState) DeviceCertChain(ctx context.Context) ([]*x509.Certificate, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var deviceInfo DeviceInfo
	if err = s.Token.DB.Where("session = ?", sessionID).First(&deviceInfo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrNotFound
		}
		return nil, err
	}

	chain, err := x509.ParseCertificates(deviceInfo.X509Chain)
	if err != nil {
		return nil, fmt.Errorf("error parsing certificate chain: %w", err)
	}

	return chain, nil
}

// SetIncompleteVoucherHeader stores an incomplete voucher header
func (s DISessionState) SetIncompleteVoucherHeader(ctx context.Context, ovh *fdo.VoucherHeader) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	headerBytes, err := cbor.Marshal(ovh)
	if err != nil {
		return fmt.Errorf("failed to marshal voucher header: %w", err)
	}

	incompleteVoucher := IncompleteVoucher{
		Session: sessionID,
		Header:  headerBytes,
	}

	return s.Token.DB.Save(&incompleteVoucher).Error
}

// IncompleteVoucherHeader retrieves an incomplete voucher header
func (s DISessionState) IncompleteVoucherHeader(ctx context.Context) (*fdo.VoucherHeader, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var incompleteVoucher IncompleteVoucher
	if err := s.Token.DB.Where("session = ?", sessionID).First(&incompleteVoucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrNotFound
		}
		return nil, err
	}

	var header fdo.VoucherHeader
	if err := cbor.Unmarshal(incompleteVoucher.Header, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal voucher header: %w", err)
	}

	return &header, nil
}
