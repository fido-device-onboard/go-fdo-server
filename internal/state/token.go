package state

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	protocol.TokenService
} = (*TokenService)(nil)

// TokenService implementation
type TokenService struct {
	DB *gorm.DB
}

type tokenKey struct{}

// Session stores session information
type Session struct {
	ID        []byte    `gorm:"primaryKey"`
	Protocol  int       `gorm:"type:integer;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime:milli;not null;index:idx_sessions_created_at"`
}

// TableName specifies the table name for Session model
func (Session) TableName() string {
	return "sessions"
}

func InitTokenServiceDB(db *gorm.DB) (*TokenService, error) {
	state := &TokenService{
		DB: db,
	}
	// Auto-migrate all schemas
	err := state.DB.AutoMigrate(
		&Session{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Debug("Token service database initialized successfully")
	return state, nil
}

// NewToken creates a new session token
func (s TokenService) NewToken(ctx context.Context, proto protocol.Protocol) (string, error) {
	// Generate a random session ID
	sessionID := make([]byte, 32)
	if _, err := rand.Read(sessionID); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create session record
	// Note: CreatedAt is automatically set by GORM due to autoCreateTime tag
	session := Session{
		ID:       sessionID,
		Protocol: int(proto),
	}

	if err := s.DB.WithContext(ctx).Create(&session).Error; err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Encode as base64 URL-safe
	token := base64.RawURLEncoding.EncodeToString(sessionID)
	return token, nil
}

// InvalidateToken removes a session
func (s TokenService) InvalidateToken(ctx context.Context) error {
	sessionID, ok := s.TokenFromContext(ctx)
	if !ok {
		return fdo.ErrInvalidSession
	}

	decoded, err := base64.RawURLEncoding.DecodeString(sessionID)
	if err != nil {
		return fdo.ErrInvalidSession
	}

	result := s.DB.WithContext(ctx).Where("id = ?", decoded).Delete(&Session{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fdo.ErrNotFound
	}

	return nil
}

// TokenContext injects a token into the context
func (s TokenService) TokenContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// TokenFromContext retrieves a token from the context
func (s TokenService) TokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenKey{}).(string)
	return token, ok
}

// getSessionID retrieves the decoded session ID from context
func (s TokenService) getSessionID(ctx context.Context) ([]byte, error) {
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
	if err := s.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrInvalidSession
		}
		return nil, err
	}

	return sessionID, nil
}

// CleanupExpiredSessions deletes all sessions older than the specified max age
// Returns the number of deleted sessions and any error encountered
func (s TokenService) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-maxAge)
	result := s.DB.WithContext(ctx).Where("created_at < ?", cutoffTime).Delete(&Session{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		slog.Debug("Cleaned up expired sessions", "count", result.RowsAffected, "maxAge", maxAge)
	}
	return result.RowsAffected, nil
}
