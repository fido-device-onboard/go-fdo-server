// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
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

	// Replace the old voucher
	return s.DB.Where("guid = ?", guid[:]).Assign(voucher).FirstOrCreate(&voucher).Error
}

// RemoveVoucher untracks a voucher, possibly by deleting it or marking it as removed
func (s *State) RemoveVoucher(ctx context.Context, guid protocol.GUID) (*fdo.Voucher, error) {
	var voucher Voucher
	if err := s.DB.Where("guid = ?", guid[:]).First(&voucher).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fdo.ErrNotFound
		}
		return nil, err
	}

	// Parse the voucher before deleting
	var ov fdo.Voucher
	if err := cbor.Unmarshal(voucher.CBOR, &ov); err != nil {
		return nil, fmt.Errorf("failed to unmarshal voucher: %w", err)
	}

	// Delete the voucher
	if err := s.DB.Where("guid = ?", guid[:]).Delete(&Voucher{}).Error; err != nil {
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
	var ownerKey OwnerKey

	query := s.DB.Where("type = ?", int(keyType))

	switch keyType {
	case protocol.Rsa2048RestrKeyType:
		query = query.Where("rsa_bits = 2048")
	case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
		query = query.Where("rsa_bits = ?", rsaBits)
	default:
		rsaBits = 0
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
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Marshal the certificate chain
	var chainBytes []byte
	for _, cert := range chain {
		chainBytes = append(chainBytes, cert.Raw...)
	}

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
	}

	return s.DB.Save(&ownerKey).Error
}

// AddManufacturerKey adds a manufacturer key to the database
func (s *State) AddManufacturerKey(keyType protocol.KeyType, key crypto.PrivateKey, chain []*x509.Certificate) error {
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Marshal the certificate chain
	var chainBytes []byte
	for _, cert := range chain {
		chainBytes = append(chainBytes, cert.Raw...)
	}

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
	}

	return s.DB.Save(&mfgKey).Error
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
		rsaBits = 0
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
