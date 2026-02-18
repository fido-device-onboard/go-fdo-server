package state

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.TO0SessionState
} = (*TO0SessionState)(nil)

// TO0SessionState implementation
type TO0SessionState struct {
	DB    *gorm.DB
	Token *TokenService
}

// TO0Session stores TO0 session state
type TO0Session struct {
	Session    []byte `gorm:"primaryKey"`
	Nonce      []byte
	SessionRef *Session `gorm:"foreignKey:Session;references:ID;constraint:OnDelete:CASCADE"`
}

// TableName specifies the table name for TO0Session model
func (TO0Session) TableName() string {
	return "to0_sessions"
}

func InitTO0SessionDB(db *gorm.DB) (*TO0SessionState, error) {
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	state := &TO0SessionState{
		Token: tokenServiceState,
		DB:    db,
	}
	// Auto-migrate all schemas
	err = state.DB.AutoMigrate(
		&TO0Session{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}

	// Explicitly create the foreign key constraint using GORM's Migrator
	// This ensures CASCADE DELETE works properly to prevent orphaned sessions
	if !state.DB.Migrator().HasConstraint(&TO0Session{}, "SessionRef") {
		if err := state.DB.Migrator().CreateConstraint(&TO0Session{}, "SessionRef"); err != nil {
			slog.Error("Failed to create foreign key constraint for TO0 sessions", "error", err)
			return nil, fmt.Errorf("failed to create CASCADE DELETE constraint: %w", err)
		}
		slog.Debug("Created foreign key constraint for TO0 sessions")
	}

	slog.Debug("TO0 Session database initialized successfully")
	return state, nil
}

// SetTO0SignNonce stores the TO0 sign nonce
func (s *TO0SessionState) SetTO0SignNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	to0Session := TO0Session{
		Session: sessionID,
		Nonce:   nonce[:],
	}

	return s.DB.WithContext(ctx).Save(&to0Session).Error
}

// TO0SignNonce retrieves the TO0 sign nonce
func (s *TO0SessionState) TO0SignNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to0Session TO0Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to0Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	var nonce protocol.Nonce
	copy(nonce[:], to0Session.Nonce)
	return nonce, nil
}
