// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// State implements all FDO server state persistence interfaces using GORM
type State struct {
	DB     *gorm.DB
	dbType string
}

type tokenKey struct{}

// InitDb initializes the database connection using GORM and returns a State
func InitDb(dbType, dsn string) (*State, error) {
	var dialector gorm.Dialector

	switch dbType {
	case "sqlite":
		dialector = sqlite.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set the global db variable for backward compatibility
	db = gormDB

	state := &State{
		DB:     gormDB,
		dbType: dbType,
	}

	// Auto-migrate all schemas
	err = state.DB.AutoMigrate(
		&Secret{},
		&MfgKey{},
		&OwnerKey{},
		&RvBlob{},
		&Session{},
		&DeviceInfo{},
		&IncompleteVoucher{},
		&TO0Session{},
		&TO1Session{},
		&TO2Session{},
		&Voucher{},
		&ReplacementVoucher{},
		&KeyExchange{},
		&OwnerInfo{},
		&RvInfo{},
		&DeviceOnboarding{},
		&DeviceCACertificate{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}

	// Enable foreign keys for SQLite
	if dbType == "sqlite" {
		sqlDB, err := state.DB.DB()
		if err == nil {
			_, _ = sqlDB.Exec("PRAGMA foreign_keys = ON")
		}
	}

	slog.Info("Database initialized successfully", "type", dbType)
	return state, nil
}

// Close closes the database connection
func (s *State) Close() error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Compile-time check for interface implementation correctness
var _ interface {
	protocol.TokenService
	fdo.DISessionState
	fdo.TO0SessionState
	fdo.TO1SessionState
	fdo.TO2SessionState
	fdo.RendezvousBlobPersistentState
	fdo.VoucherPersistentState
	fdo.OwnerVoucherPersistentState
	fdo.OwnerKeyPersistentState
} = (*State)(nil)

// TokenService implementation

// NewToken creates a new session token
func (s *State) NewToken(ctx context.Context, proto protocol.Protocol) (string, error) {
	// Generate a random session ID
	sessionID := make([]byte, 32)
	if _, err := rand.Read(sessionID); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create session record
	session := Session{
		ID:       sessionID,
		Protocol: int(proto),
	}

	if err := s.DB.Create(&session).Error; err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Encode as base64 URL-safe
	token := base64.RawURLEncoding.EncodeToString(sessionID)
	return token, nil
}

// InvalidateToken removes a session
func (s *State) InvalidateToken(ctx context.Context) error {
	sessionID, ok := s.TokenFromContext(ctx)
	if !ok {
		return fdo.ErrInvalidSession
	}

	decoded, err := base64.RawURLEncoding.DecodeString(sessionID)
	if err != nil {
		return fdo.ErrInvalidSession
	}

	result := s.DB.Where("id = ?", decoded).Delete(&Session{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fdo.ErrNotFound
	}

	return nil
}

// TokenContext injects a token into the context
func (s *State) TokenContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// TokenFromContext retrieves a token from the context
func (s *State) TokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenKey{}).(string)
	return token, ok
}

// getSessionID retrieves the decoded session ID from context
func (s *State) getSessionID(ctx context.Context) ([]byte, error) {
	token, ok := s.TokenFromContext(ctx)
	if !ok {
		return nil, fdo.ErrInvalidSession
	}

	sessionID, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fdo.ErrInvalidSession
	}

	// Verify session exists
	var session Session
	if err := s.DB.Where("id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrInvalidSession
		}
		return nil, err
	}

	return sessionID, nil
}

// DISessionState implementation

// SetDeviceCertChain stores the device certificate chain
func (s *State) SetDeviceCertChain(ctx context.Context, chain []*x509.Certificate) error {
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

	return s.DB.Where("session = ?", sessionID).
		Assign(map[string]interface{}{"x509_chain": chainBytes}).
		FirstOrCreate(&deviceInfo).Error
}

// DeviceCertChain retrieves the device certificate chain
func (s *State) DeviceCertChain(ctx context.Context) ([]*x509.Certificate, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var deviceInfo DeviceInfo
	if err = s.DB.Where("session = ?", sessionID).First(&deviceInfo).Error; err != nil {
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
func (s *State) SetIncompleteVoucherHeader(ctx context.Context, ovh *fdo.VoucherHeader) error {
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

	return s.DB.Save(&incompleteVoucher).Error
}

// IncompleteVoucherHeader retrieves an incomplete voucher header
func (s *State) IncompleteVoucherHeader(ctx context.Context) (*fdo.VoucherHeader, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var incompleteVoucher IncompleteVoucher
	if err := s.DB.Where("session = ?", sessionID).First(&incompleteVoucher).Error; err != nil {
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

// TO0SessionState implementation

// SetTO0SignNonce stores the TO0 sign nonce
func (s *State) SetTO0SignNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	to0Session := TO0Session{
		Session: sessionID,
		Nonce:   nonce[:],
	}

	return s.DB.Save(&to0Session).Error
}

// TO0SignNonce retrieves the TO0 sign nonce
func (s *State) TO0SignNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to0Session TO0Session
	if err := s.DB.Where("session = ?", sessionID).First(&to0Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	var nonce protocol.Nonce
	copy(nonce[:], to0Session.Nonce)
	return nonce, nil
}

// TO1SessionState implementation

// SetTO1ProofNonce stores the TO1 proof nonce
func (s *State) SetTO1ProofNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	to1Session := TO1Session{
		Session: sessionID,
		Nonce:   nonce[:],
	}

	return s.DB.Save(&to1Session).Error
}

// TO1ProofNonce retrieves the TO1 proof nonce
func (s *State) TO1ProofNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to1Session TO1Session
	if err := s.DB.Where("session = ?", sessionID).First(&to1Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	var nonce protocol.Nonce
	copy(nonce[:], to1Session.Nonce)
	return nonce, nil
}

// TO2SessionState implementation will be in the next file due to length
