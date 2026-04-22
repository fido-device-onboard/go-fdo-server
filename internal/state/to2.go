package state

import (
	"context"
	"encoding"
	"fmt"
	"log/slog"
	"math"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/kex"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"gorm.io/gorm"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.TO2SessionState
} = (*TO2SessionState)(nil)

// checkTO2SessionUpdate returns fdo.ErrNotFound when a TO2Session Update/Updates
// call succeeds but matched no rows (i.e. the session row does not exist yet).
func checkTO2SessionUpdate(tx *gorm.DB) error {
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return fdo.ErrNotFound
	}
	return nil
}

// TO2SessionState implementation
type TO2SessionState struct {
	Token *TokenService
	DB    *gorm.DB
}

// TO2Session stores TO2 session state
type TO2Session struct {
	Session        []byte `gorm:"primaryKey"`
	GUID           []byte
	RvInfo         []byte
	ProveDevice    []byte
	SetupDevice    []byte
	MTU            *int `gorm:"type:integer"`
	Devmod         []byte
	Modules        []byte
	DevmodComplete *bool `gorm:"type:boolean"`
}

// TableName specifies the table name for TO2Session model
func (TO2Session) TableName() string {
	return "to2_sessions"
}

// KeyExchange stores key exchange sessions
type KeyExchange struct {
	Session []byte `gorm:"primaryKey"`
	Suite   string `gorm:"type:text;not null"`
	CBOR    []byte `gorm:"not null"`
}

// TableName specifies the table name for KeyExchange model
func (KeyExchange) TableName() string {
	return "key_exchanges"
}

func InitTO2SessionDB(db *gorm.DB) (*TO2SessionState, error) {
	tokenServiceState, err := InitTokenServiceDB(db)
	if err != nil {
		return nil, err
	}
	state := &TO2SessionState{
		Token: tokenServiceState,
		DB:    db,
	}
	// Auto-migrate all schemas
	err = state.DB.AutoMigrate(
		&TO2Session{},
		&KeyExchange{},
	)
	if err != nil {
		slog.Error("Failed to migrate database schema", "error", err)
		return nil, err
	}
	slog.Info("TO2 Session database initialized successfully")
	return state, nil
}

// TO2SessionState implementation

// SetGUID associates a voucher GUID with a TO2 session
func (s *TO2SessionState) SetGUID(ctx context.Context, guid protocol.GUID) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	to2Session := TO2Session{
		Session: sessionID,
		GUID:    guid[:],
	}

	// Assign+FirstOrCreate upserts the GUID: creates a new row if the session
	// does not exist yet, or overwrites the GUID on an existing row. The
	// overwrite handles TO2 protocol retries gracefully — a retried
	// TO2.HelloDevice re-associates the same (or updated) GUID with the session.
	return s.DB.WithContext(ctx).Where("session = ?", sessionID).
		Assign(map[string]interface{}{"guid": guid[:]}).
		FirstOrCreate(&to2Session).Error
}

// GUID retrieves the GUID associated with the TO2 session
func (s *TO2SessionState) GUID(ctx context.Context) (protocol.GUID, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.GUID{}, err
	}

	var to2Session TO2Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.GUID{}, fdo.ErrNotFound
		}
		return protocol.GUID{}, err
	}

	var guid protocol.GUID
	copy(guid[:], to2Session.GUID)
	return guid, nil
}

// SetRvInfo stores the rendezvous instructions
func (s *TO2SessionState) SetRvInfo(ctx context.Context, rvInfo [][]protocol.RvInstruction) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	rvInfoBytes, err := cbor.Marshal(rvInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal rvInfo: %w", err)
	}

	return checkTO2SessionUpdate(s.DB.WithContext(ctx).Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("rv_info", rvInfoBytes))
}

// RvInfo retrieves the rendezvous instructions
func (s *TO2SessionState) RvInfo(ctx context.Context) ([][]protocol.RvInstruction, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var to2Session TO2Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrNotFound
		}
		return nil, err
	}

	if to2Session.RvInfo == nil {
		return nil, fdo.ErrNotFound
	}

	var rvInfo [][]protocol.RvInstruction
	if err := cbor.Unmarshal(to2Session.RvInfo, &rvInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rvInfo: %w", err)
	}

	return rvInfo, nil
}

// SetReplacementGUID stores the device GUID to persist at the end of TO2
func (s *TO2SessionState) SetReplacementGUID(ctx context.Context, guid protocol.GUID) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	replacementVoucher := ReplacementVoucher{
		Session: sessionID,
		GUID:    guid[:],
	}

	return s.DB.WithContext(ctx).Where("session = ?", sessionID).
		Assign(map[string]interface{}{"guid": guid[:]}).
		FirstOrCreate(&replacementVoucher).Error
}

// ReplacementGUID retrieves the device GUID to persist at the end of TO2
func (s *TO2SessionState) ReplacementGUID(ctx context.Context) (protocol.GUID, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.GUID{}, err
	}

	var replacementVoucher ReplacementVoucher
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&replacementVoucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.GUID{}, fdo.ErrNotFound
		}
		return protocol.GUID{}, err
	}

	var guid protocol.GUID
	copy(guid[:], replacementVoucher.GUID)
	return guid, nil
}

// SetReplacementHmac stores the voucher HMAC to persist at the end of TO2
func (s *TO2SessionState) SetReplacementHmac(ctx context.Context, hmac protocol.Hmac) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	hmacBytes, err := cbor.Marshal(hmac)
	if err != nil {
		return fmt.Errorf("failed to marshal hmac: %w", err)
	}

	tx := s.DB.WithContext(ctx).Model(&ReplacementVoucher{}).Where("session = ?", sessionID).Update("hmac", hmacBytes)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return fdo.ErrNotFound
	}
	return nil
}

// ReplacementHmac retrieves the voucher HMAC to persist at the end of TO2
func (s *TO2SessionState) ReplacementHmac(ctx context.Context) (protocol.Hmac, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.Hmac{}, err
	}

	var replacementVoucher ReplacementVoucher
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&replacementVoucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Hmac{}, fdo.ErrNotFound
		}
		return protocol.Hmac{}, err
	}

	var hmac protocol.Hmac
	if err := cbor.Unmarshal(replacementVoucher.Hmac, &hmac); err != nil {
		return protocol.Hmac{}, fmt.Errorf("failed to unmarshal hmac: %w", err)
	}
	return hmac, nil
}

// SetXSession updates the current key exchange/encryption session
func (s *TO2SessionState) SetXSession(ctx context.Context, suite kex.Suite, sess kex.Session) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	// Use BinaryMarshaler to serialize the session state
	marshaler, ok := sess.(encoding.BinaryMarshaler)
	if !ok {
		return fmt.Errorf("key exchange session does not support binary marshaling")
	}

	sessBytes, err := marshaler.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	keyExchange := KeyExchange{
		Session: sessionID,
		Suite:   string(suite),
		CBOR:    sessBytes,
	}

	return s.DB.WithContext(ctx).Save(&keyExchange).Error
}

// XSession returns the current key exchange/encryption session
func (s *TO2SessionState) XSession(ctx context.Context) (kex.Suite, kex.Session, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return "", nil, err
	}

	var keyExchange KeyExchange
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&keyExchange).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil, fdo.ErrNotFound
		}
		return "", nil, err
	}

	// Parse suite
	suite := kex.Suite(keyExchange.Suite)

	// Create a new session instance using the suite.
	// The cipher suite ID (1) is a placeholder; UnmarshalBinary restores the
	// actual cipher suite from the serialized session state.
	sess := suite.New(nil, 1)

	// Use BinaryUnmarshaler to deserialize the session state
	unmarshaler, ok := sess.(encoding.BinaryUnmarshaler)
	if !ok {
		return "", nil, fmt.Errorf("key exchange session does not support binary unmarshaling")
	}

	if err := unmarshaler.UnmarshalBinary(keyExchange.CBOR); err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return suite, sess, nil
}

// SetProveDeviceNonce stores the Nonce used in TO2.ProveDevice
func (s *TO2SessionState) SetProveDeviceNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	return checkTO2SessionUpdate(s.DB.WithContext(ctx).Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("prove_device", nonce[:]))
}

// ProveDeviceNonce returns the Nonce used in TO2.ProveDevice and TO2.Done
func (s *TO2SessionState) ProveDeviceNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to2Session TO2Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	if to2Session.ProveDevice == nil {
		return protocol.Nonce{}, fdo.ErrNotFound
	}

	var nonce protocol.Nonce
	copy(nonce[:], to2Session.ProveDevice)
	return nonce, nil
}

// SetSetupDeviceNonce stores the Nonce used in TO2.SetupDevice
func (s *TO2SessionState) SetSetupDeviceNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	return checkTO2SessionUpdate(s.DB.WithContext(ctx).Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("setup_device", nonce[:]))
}

// SetupDeviceNonce returns the Nonce used in TO2.SetupDevice and TO2.Done2
func (s *TO2SessionState) SetupDeviceNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to2Session TO2Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return protocol.Nonce{}, fdo.ErrNotFound
		}
		return protocol.Nonce{}, err
	}

	if to2Session.SetupDevice == nil {
		return protocol.Nonce{}, fdo.ErrNotFound
	}

	var nonce protocol.Nonce
	copy(nonce[:], to2Session.SetupDevice)
	return nonce, nil
}

// SetMTU sets the max service info size the device may receive
func (s *TO2SessionState) SetMTU(ctx context.Context, mtu uint16) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	mtuInt := int(mtu)
	return checkTO2SessionUpdate(s.DB.WithContext(ctx).Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("mtu", mtuInt))
}

// MTU returns the max service info size the device may receive
func (s *TO2SessionState) MTU(ctx context.Context) (uint16, error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return 0, err
	}

	var to2Session TO2Session
	if err := s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, fdo.ErrNotFound
		}
		return 0, err
	}

	if to2Session.MTU == nil {
		return 0, fdo.ErrNotFound
	}

	if *to2Session.MTU < 0 || *to2Session.MTU > math.MaxUint16 {
		return 0, fmt.Errorf("MTU value out of valid range (0-%d): %d", math.MaxUint16, *to2Session.MTU)
	}
	return uint16(*to2Session.MTU), nil
}

// SetDevmod sets the device info and module support
func (s *TO2SessionState) SetDevmod(ctx context.Context, devmod serviceinfo.Devmod, modules []string, complete bool) error {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return err
	}

	devmodBytes, err := cbor.Marshal(devmod)
	if err != nil {
		return fmt.Errorf("failed to marshal devmod: %w", err)
	}

	modulesBytes, err := cbor.Marshal(modules)
	if err != nil {
		return fmt.Errorf("failed to marshal modules: %w", err)
	}

	return checkTO2SessionUpdate(s.DB.WithContext(ctx).Model(&TO2Session{}).Where("session = ?", sessionID).
		Updates(map[string]interface{}{
			"devmod":          devmodBytes,
			"modules":         modulesBytes,
			"devmod_complete": complete,
		}))
}

// Devmod returns the device info and module support
func (s *TO2SessionState) Devmod(ctx context.Context) (devmod serviceinfo.Devmod, modules []string, complete bool, err error) {
	sessionID, err := s.Token.getSessionID(ctx)
	if err != nil {
		return
	}

	var to2Session TO2Session
	if err = s.DB.WithContext(ctx).Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			err = fdo.ErrNotFound
		}
		return
	}

	if to2Session.Devmod != nil {
		if err = cbor.Unmarshal(to2Session.Devmod, &devmod); err != nil {
			err = fmt.Errorf("failed to unmarshal devmod: %w", err)
			return
		}
	}

	if to2Session.Modules != nil {
		if err = cbor.Unmarshal(to2Session.Modules, &modules); err != nil {
			err = fmt.Errorf("failed to unmarshal modules: %w", err)
			return
		}
	}

	if to2Session.DevmodComplete != nil {
		complete = *to2Session.DevmodComplete
	}

	return
}
