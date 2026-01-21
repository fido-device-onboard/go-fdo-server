// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"log/slog"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// RendezvousBlobPersistentState implementation

// SetRVBlob sets the owner rendezvous blob for a device
func (s *State) SetRVBlob(ctx context.Context, voucher *fdo.Voucher, to1d *cose.Sign1[protocol.To1d, []byte], exp time.Time) error {
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

	return s.DB.Save(&rvBlob).Error
}

// RVBlob returns the owner rendezvous blob for a device
func (s *State) RVBlob(ctx context.Context, guid protocol.GUID) (*cose.Sign1[protocol.To1d, []byte], *fdo.Voucher, error) {
	var rvBlob RvBlob
	if err := s.DB.Where("guid = ?", guid[:]).First(&rvBlob).Error; err != nil {
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

// ManufacturerVoucherPersistentState implementation

// NewVoucher creates and stores a voucher for a newly initialized device
func (s *State) NewVoucher(ctx context.Context, ov *fdo.Voucher) error {
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
func (s *State) AddVoucher(ctx context.Context, ov *fdo.Voucher) error {
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
func (s *State) ReplaceVoucher(ctx context.Context, guid protocol.GUID, ov *fdo.Voucher) error {
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
func (s *State) RemoveVoucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
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
func (s *State) Voucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
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

// OwnerKeyPersistentState implementation

// OwnerKey returns the private key matching a given key type and optionally its certificate chain
func (s *State) OwnerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	// First, try to find the key in the in-memory cache
	s.Mutex.RLock()
	for _, entry := range s.ownerKeyEntries {
		if entry.Type != int(keyType) {
			continue
		}

		// Check rsaBits for RSA key types
		switch keyType {
		case protocol.Rsa2048RestrKeyType:
			if entry.RsaBits != 2048 {
				continue
			}
		case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
			if entry.RsaBits != rsaBits {
				continue
			}
		}

		// Found matching key
		signer := entry.Signer
		certChain := entry.CertChain
		s.Mutex.RUnlock()
		return signer, certChain, nil
	}
	s.Mutex.RUnlock()

	// Fallback: query the database if not found in cache
	// This handles the case where keys were added after LoadOwnerKeys was called
	var ownerKey OwnerKey

	query := s.DB.Where("type = ?", int(keyType))

	switch keyType {
	case protocol.Rsa2048RestrKeyType:
		query = query.Where("rsa_bits = 2048")
	case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
		query = query.Where("rsa_bits = ?", rsaBits)
	default:
		// For non-RSA keys (ECDSA, etc.), rsa_bits is 0
		query = query.Where("rsa_bits = 0")
	}

	if err := query.First(&ownerKey).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fdo.ErrNotFound
		}
		return nil, nil, err
	}

	// Parse the private key
	key, err := x509.ParsePKCS8PrivateKey(ownerKey.PKCS8)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse owner private key: %w", err)
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("key is not a signer")
	}

	chain, err := x509.ParseCertificates(ownerKey.X509Chain)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing certificate chain: %w", err)
	}

	return signer, chain, nil
}

// AddOwnerKey adds an owner key to the database
func (s *State) AddOwnerKey(keyType protocol.KeyType, key crypto.PrivateKey, chain []*x509.Certificate) error {
	slog.Debug("Adding owner key to database", "keyType", keyType)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		slog.Error("Failed to marshal owner private key", "keyType", keyType, "err", err)
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	slog.Debug("Owner private key marshaled successfully", "size", len(pkcs8))

	// Marshal the certificate chain
	var chainBytes []byte
	for _, cert := range chain {
		chainBytes = append(chainBytes, cert.Raw...)
	}
	slog.Debug("Owner certificate chain marshaled", "chainLength", len(chain), "chainBytes", len(chainBytes))

	ownerKey := OwnerKey{
		Type:      int(keyType),
		PKCS8:     pkcs8,
		X509Chain: chainBytes,
	}

	switch keyType {
	case protocol.Rsa2048RestrKeyType:
		rsaBits := 2048
		ownerKey.RsaBits = &rsaBits
	case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("expected key type to be *rsa.PrivateKey, got %T", key)
		}
		rsaBits := rsaKey.Size() * 8
		ownerKey.RsaBits = &rsaBits
	default:
		// For non-RSA keys (ECDSA, etc.), set RsaBits to 0
		// This is required because RsaBits is part of the composite primary key
		rsaBits := 0
		ownerKey.RsaBits = &rsaBits
	}

	// Save the key to the database
	slog.Debug("Saving owner key to database", "keyType", keyType, "rsaBits", ownerKey.RsaBits)
	if err := s.DB.Save(&ownerKey).Error; err != nil {
		slog.Error("Failed to save owner key to database", "keyType", keyType, "rsaBits", ownerKey.RsaBits, "err", err)
		return fmt.Errorf("failed to save owner key to database: %w", err)
	}
	slog.Debug("Owner key saved to database successfully")

	// Reload owner keys to refresh the in-memory cache
	slog.Debug("Reloading owner keys cache")
	if err := s.LoadOwnerKeys(context.Background()); err != nil {
		slog.Error("Failed to reload owner keys cache", "err", err)
		return fmt.Errorf("failed to reload owner keys after adding: %w", err)
	}

	slog.Info("Owner key added and cache reloaded successfully", "keyType", keyType, "rsaBits", ownerKey.RsaBits)
	return nil
}

// LoadOwnerKeys loads all owner keys from the database into memory.
// This should be called on server startup to avoid repeated database queries during runtime.
func (s *State) LoadOwnerKeys(ctx context.Context) error {
	// Retrieve all owner keys from the database
	var dbKeys []OwnerKey
	if err := s.DB.Find(&dbKeys).Error; err != nil {
		return fmt.Errorf("failed to load owner keys from database: %w", err)
	}

	// Parse and store each owner key
	ownerKeyEntries := make([]OwnerKeyEntry, 0, len(dbKeys))
	ownerPublicKeys := make([]crypto.PublicKey, 0, len(dbKeys))

	for _, dbKey := range dbKeys {
		// Parse the private key
		key, err := x509.ParsePKCS8PrivateKey(dbKey.PKCS8)
		if err != nil {
			return fmt.Errorf("failed to parse owner private key (type=%d, rsaBits=%v): %w", dbKey.Type, dbKey.RsaBits, err)
		}

		signer, ok := key.(crypto.Signer)
		if !ok {
			return fmt.Errorf("owner key (type=%d, rsaBits=%v) is not a signer", dbKey.Type, dbKey.RsaBits)
		}

		// Parse certificate chain if present
		var certChain []*x509.Certificate
		if len(dbKey.X509Chain) > 0 {
			certChain, err = x509.ParseCertificates(dbKey.X509Chain)
			if err != nil {
				return fmt.Errorf("failed to parse certificate chain for owner key (type=%d, rsaBits=%v): %w", dbKey.Type, dbKey.RsaBits, err)
			}
		}

		// Determine rsaBits value
		rsaBits := 0
		if dbKey.RsaBits != nil {
			rsaBits = *dbKey.RsaBits
		}

		// Store full key entry
		ownerKeyEntries = append(ownerKeyEntries, OwnerKeyEntry{
			Type:      dbKey.Type,
			RsaBits:   rsaBits,
			Signer:    signer,
			CertChain: certChain,
		})

		// Store public key for voucher verification
		ownerPublicKeys = append(ownerPublicKeys, signer.Public())
	}

	// Update the state with the loaded keys
	s.Mutex.Lock()
	s.ownerKeyEntries = ownerKeyEntries
	s.OwnerKeys = ownerPublicKeys
	s.Mutex.Unlock()

	slog.Info("Loaded owner keys into memory", "count", len(ownerKeyEntries))
	return nil
}

// AddManufacturerKey adds a manufacturer key to the database
func (s *State) AddManufacturerKey(keyType protocol.KeyType, key crypto.PrivateKey, chain []*x509.Certificate) error {
	slog.Debug("Adding manufacturer key to database", "keyType", keyType)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		slog.Error("Failed to marshal manufacturer private key", "keyType", keyType, "err", err)
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	slog.Debug("Manufacturer private key marshaled successfully", "size", len(pkcs8))

	// Marshal the certificate chain
	var chainBytes []byte
	for _, cert := range chain {
		chainBytes = append(chainBytes, cert.Raw...)
	}
	slog.Debug("Manufacturer certificate chain marshaled", "chainLength", len(chain), "chainBytes", len(chainBytes))

	mfgKey := MfgKey{
		Type:      int(keyType),
		PKCS8:     pkcs8,
		X509Chain: chainBytes,
	}

	switch keyType {
	case protocol.Rsa2048RestrKeyType:
		rsaBits := 2048
		mfgKey.RsaBits = &rsaBits
	case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("expected key type to be *rsa.PrivateKey, got %T", key)
		}
		rsaBits := rsaKey.Size() * 8
		mfgKey.RsaBits = &rsaBits
	default:
		// For non-RSA keys (ECDSA, etc.), set RsaBits to 0
		// This is required because RsaBits is part of the composite primary key
		rsaBits := 0
		mfgKey.RsaBits = &rsaBits
	}

	slog.Debug("Saving manufacturer key to database", "keyType", keyType, "rsaBits", mfgKey.RsaBits)
	if err := s.DB.Save(&mfgKey).Error; err != nil {
		slog.Error("Failed to save manufacturer key to database", "keyType", keyType, "rsaBits", mfgKey.RsaBits, "err", err)
		return fmt.Errorf("failed to save manufacturer key to database: %w", err)
	}
	slog.Info("Manufacturer key saved to database successfully", "keyType", keyType, "rsaBits", mfgKey.RsaBits)
	return nil
}

// ManufacturerKey returns the private key matching a given key type and optionally its certificate chain
func (s *State) ManufacturerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	var mfgKey MfgKey

	query := s.DB.Where("type = ?", int(keyType))

	switch keyType {
	case protocol.Rsa2048RestrKeyType:
		query = query.Where("rsa_bits = 2048")
	case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
		query = query.Where("rsa_bits = ?", rsaBits)
	default:
		// For non-RSA keys (ECDSA, etc.), rsa_bits is 0
		query = query.Where("rsa_bits = 0")
	}

	if err := query.First(&mfgKey).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fdo.ErrNotFound
		}
		return nil, nil, err
	}

	// Parse the private key
	key, err := x509.ParsePKCS8PrivateKey(mfgKey.PKCS8)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("key is not a signer")
	}

	chain, err := x509.ParseCertificates(mfgKey.X509Chain)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing certificate chain: %w", err)
	}

	return signer, chain, nil
}
