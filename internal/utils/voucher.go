// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"slices"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// VerifyVoucherOwnership verifies the ownership voucher belongs to one of the
// provided owner public keys.
func VerifyVoucherOwnership(ov *fdo.Voucher, ownerPKeys []crypto.PublicKey) error {
	if len(ownerPKeys) == 0 {
		return fmt.Errorf("ownerPKeys must contain at least one owner public key")
	}

	expectedPubKey, err := ov.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("unable to parse owner public key from voucher: %w", err)
	}

	if !slices.ContainsFunc(ownerPKeys, func(k crypto.PublicKey) bool {
		return PublicKeysEqual(expectedPubKey, k)
	}) {
		return fmt.Errorf("voucher owner key does not match any of the server's configured keys")
	}

	return nil
}

// VerifyOwnershipVoucher performs header field validation and cryptographic
// verification of an ownership voucher.
//
// If certPool is non-nil, the device certificate chain is verified against
// the provided CA pool. If certPool is nil, the device certificate chain
// verification is skipped, allowing vouchers signed by any device CA to be
// imported.
//
// Note: prior to this behavior (release-1.0), VerifyDeviceCertChain was called
// unconditionally with nil, which validated the chain against the system trust
// roots. The current behavior intentionally skips this check when no trusted
// CAs are configured, to support open import workflows.
func VerifyOwnershipVoucher(ov *fdo.Voucher, certPool *x509.CertPool) error {
	const FDOProtocolVersion uint16 = 101 // FDO spec v1.1

	// Header Field Validation
	if ov.Version != FDOProtocolVersion {
		return fmt.Errorf("unsupported protocol version: %d (expected %d)", ov.Version, FDOProtocolVersion)
	}
	if ov.Version != ov.Header.Val.Version {
		return fmt.Errorf("protocol version mismatch: voucher version=%d, header version=%d",
			ov.Version, ov.Header.Val.Version)
	}
	var zeroGUID protocol.GUID
	if ov.Header.Val.GUID == zeroGUID {
		return fmt.Errorf("invalid voucher: GUID is zero")
	}
	if ov.Header.Val.DeviceInfo == "" {
		return fmt.Errorf("invalid voucher: DeviceInfo is empty")
	}
	if ov.Header.Val.ManufacturerKey.Type == 0 {
		return fmt.Errorf("invalid voucher: ManufacturerKey is missing or invalid")
	}
	if len(ov.Header.Val.RvInfo) == 0 {
		return fmt.Errorf("invalid voucher: RvInfo is empty")
	}

	// Cryptographic Integrity Verification
	if err := ov.VerifyEntries(); err != nil {
		return fmt.Errorf("signature chain verification failed: %w", err)
	}
	if err := ov.VerifyCertChainHash(); err != nil {
		return fmt.Errorf("device certificate chain hash verification failed: %w", err)
	}
	if certPool != nil {
		if err := ov.VerifyDeviceCertChain(certPool); err != nil {
			return fmt.Errorf("device certificate chain verification failed: %w", err)
		}
	}

	// Verify the manufacturer certificate chain is internally consistent.
	// Passing nil uses the chain's own root as the trust anchor (self-validation).
	// This is intentional: manufacturer CA pinning is not supported — the
	// manufacturer identity is verified through the voucher's ownership chain
	// and HMAC, not through a separate CA trust store.
	if err := ov.VerifyManufacturerCertChain(nil); err != nil {
		return fmt.Errorf("manufacturer certificate chain verification failed: %w", err)
	}

	return nil
}
