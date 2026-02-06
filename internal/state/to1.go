package state

import (
	"context"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.TO1SessionState
} = (*TO1SessionState)(nil)

// TO1SessionState implementation
type TO1SessionState struct {
	DB    *gorm.DB
	Token *TokenService
}

// TO1Session stores TO1 session state
type TO1Session struct {
	Session []byte `gorm:"primaryKey"`
	Nonce   []byte
	Alg     *int `gorm:"type:integer"`
}

// TableName specifies the table name for TO1Session model
func (TO1Session) TableName() string {
	return "to1_sessions"
}

func InitTO1SessionDB(db *gorm.DB) (*TO1SessionState, error) {
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	state := &TO1SessionState{
		Token: tokenServiceState,
		DB:    db,
	}
	err = state.DB.AutoMigrate(
		&TO1Session{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Debug("TO1 Session database initialized successfully")
	return state, nil
}

// SetTO1ProofNonce stores the TO1 proof nonce
func (s *TO1SessionState) SetTO1ProofNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	to1Session := TO1Session{
		Session: sessionID,
		Nonce:   nonce[:],
	}

	return s.DB.WithContext(ctx).Save(&to1Session).Error
}

// TO1ProofNonce retrieves the TO1 proof nonce
func (s *TO1SessionState) TO1ProofNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to1Session TO1Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to1Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	var nonce protocol.Nonce
	copy(nonce[:], to1Session.Nonce)
	return nonce, nil
}
