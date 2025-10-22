// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"context"
	"encoding"
	"fmt"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/kex"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"gorm.io/gorm"
)

// TO2SessionState implementation

// SetGUID associates a voucher GUID with a TO2 session
func (s *State) SetGUID(ctx context.Context, guid protocol.GUID) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	to2Session := TO2Session{
		Session: sessionID,
		GUID:    guid[:],
	}

	return s.DB.Where("session = ?", sessionID).
		Assign(map[string]interface{}{"guid": guid[:]}).
		FirstOrCreate(&to2Session).Error
}

// GUID retrieves the GUID associated with the TO2 session
func (s *State) GUID(ctx context.Context) (protocol.GUID, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.GUID{}, err
	}

	var to2Session TO2Session
	if err := s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
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
func (s *State) SetRvInfo(ctx context.Context, rvInfo [][]protocol.RvInstruction) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	rvInfoBytes, err := cbor.Marshal(rvInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal rvInfo: %w", err)
	}

	return s.DB.Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("rv_info", rvInfoBytes).Error
}

// RvInfo retrieves the rendezvous instructions
func (s *State) RvInfo(ctx context.Context) ([][]protocol.RvInstruction, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return nil, err
	}

	var to2Session TO2Session
	if err := s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
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
func (s *State) SetReplacementGUID(ctx context.Context, guid protocol.GUID) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	replacementVoucher := ReplacementVoucher{
		Session: sessionID,
		GUID:    guid[:],
	}

	return s.DB.Where("session = ?", sessionID).
		Assign(map[string]interface{}{"guid": guid[:]}).
		FirstOrCreate(&replacementVoucher).Error
}

// ReplacementGUID retrieves the device GUID to persist at the end of TO2
func (s *State) ReplacementGUID(ctx context.Context) (protocol.GUID, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.GUID{}, err
	}

	var replacementVoucher ReplacementVoucher
	if err := s.DB.Where("session = ?", sessionID).First(&replacementVoucher).Error; err != nil {
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
func (s *State) SetReplacementHmac(ctx context.Context, hmac protocol.Hmac) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	hmacBytes, err := cbor.Marshal(hmac)
	if err != nil {
		return fmt.Errorf("failed to marshal hmac: %w", err)
	}

	return s.DB.Model(&ReplacementVoucher{}).Where("session = ?", sessionID).
		Update("hmac", hmacBytes).Error
}

// ReplacementHmac retrieves the voucher HMAC to persist at the end of TO2
func (s *State) ReplacementHmac(ctx context.Context) (protocol.Hmac, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.Hmac{}, err
	}

	var replacementVoucher ReplacementVoucher
	if err := s.DB.Where("session = ?", sessionID).First(&replacementVoucher).Error; err != nil {
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
func (s *State) SetXSession(ctx context.Context, suite kex.Suite, sess kex.Session) error {
	sessionID, err := s.getSessionID(ctx)
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

	return s.DB.Save(&keyExchange).Error
}

// XSession returns the current key exchange/encryption session
func (s *State) XSession(ctx context.Context) (kex.Suite, kex.Session, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return "", nil, err
	}

	var keyExchange KeyExchange
	if err := s.DB.Where("session = ?", sessionID).First(&keyExchange).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil, fdo.ErrNotFound
		}
		return "", nil, err
	}

	// Parse suite
	suite := kex.Suite(keyExchange.Suite)

	// Create a new session instance using the suite
	// Using cipher suite ID 1 (A128GcmCipher) as per go-fdo convention
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
func (s *State) SetProveDeviceNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	return s.DB.Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("prove_device", nonce[:]).Error
}

// ProveDeviceNonce returns the Nonce used in TO2.ProveDevice and TO2.Done
func (s *State) ProveDeviceNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to2Session TO2Session
	if err := s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
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
func (s *State) SetSetupDeviceNonce(ctx context.Context, nonce protocol.Nonce) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	return s.DB.Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("setup_device", nonce[:]).Error
}

// SetupDeviceNonce returns the Nonce used in TO2.SetupDevice and TO2.Done2
func (s *State) SetupDeviceNonce(ctx context.Context) (protocol.Nonce, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return protocol.Nonce{}, err
	}

	var to2Session TO2Session
	if err := s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
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
func (s *State) SetMTU(ctx context.Context, mtu uint16) error {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return err
	}

	mtuInt := int(mtu)
	return s.DB.Model(&TO2Session{}).Where("session = ?", sessionID).
		Update("mtu", mtuInt).Error
}

// MTU returns the max service info size the device may receive
func (s *State) MTU(ctx context.Context) (uint16, error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return 0, err
	}

	var to2Session TO2Session
	if err := s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, fdo.ErrNotFound
		}
		return 0, err
	}

	if to2Session.MTU == nil {
		return 0, fdo.ErrNotFound
	}

	return uint16(*to2Session.MTU), nil
}

// SetDevmod sets the device info and module support
func (s *State) SetDevmod(ctx context.Context, devmod serviceinfo.Devmod, modules []string, complete bool) error {
	sessionID, err := s.getSessionID(ctx)
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

	return s.DB.Model(&TO2Session{}).Where("session = ?", sessionID).
		Updates(map[string]interface{}{
			"devmod":          devmodBytes,
			"modules":         modulesBytes,
			"devmod_complete": complete,
		}).Error
}

// Devmod returns the device info and module support
func (s *State) Devmod(ctx context.Context) (devmod serviceinfo.Devmod, modules []string, complete bool, err error) {
	sessionID, err := s.getSessionID(ctx)
	if err != nil {
		return
	}

	var to2Session TO2Session
	if err = s.DB.Where("session = ?", sessionID).First(&to2Session).Error; err != nil {
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
