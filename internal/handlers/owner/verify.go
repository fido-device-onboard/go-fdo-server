// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package owner

import (
	"context"
	"crypto"
	"fmt"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

// VerifyVoucher verifies that a voucher is valid and owned by this server
func VerifyVoucher(ctx context.Context, voucher fdo.Voucher, ownerKey crypto.Signer, ownerState *state.OwnerState, reuseCred bool) error {
	// 1. Verify the voucher has at least one entry
	// Per spec, vouchers with zero entries/extensions should be rejected
	if len(voucher.Entries) == 0 {
		return fmt.Errorf("voucher has no ownership entries")
	}

	// 2. Verify device certificate chain against trusted device CAs
	// This prevents accepting vouchers from unknown/untrusted manufacturers
	ownerState.DeviceCA.Mutex.RLock()
	certPool := ownerState.DeviceCA.TrustedDeviceCACertPool
	ownerState.DeviceCA.Mutex.RUnlock()

	if err := voucher.VerifyDeviceCertChain(certPool); err != nil {
		return fmt.Errorf("device certificate chain verification failed: %w", err)
	}

	// 3. Verify the voucher is owned by this server
	voucherOwnerPubKey, err := voucher.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("failed to extract owner public key from voucher: %w", err)
	}

	// Get the public key from our owner private key
	var serverOwnerPubKey crypto.PublicKey
	switch k := ownerKey.(type) {
	case interface{ Public() crypto.PublicKey }:
		serverOwnerPubKey = k.Public()
	default:
		return fmt.Errorf("owner key does not expose public key")
	}

	// Compare the voucher's owner public key with our server's owner public key
	if !publicKeysEqual(voucherOwnerPubKey, serverOwnerPubKey) {
		return fmt.Errorf("voucher is not owned by this server (public key mismatch)")
	}

	// 4. Check if TO2 has already been completed for this voucher
	// (unless credential reuse is enabled)
	if !reuseCred {
		completed, err := ownerState.Voucher.IsTO2Completed(ctx, voucher.Header.Val.GUID)
		if err != nil {
			return fmt.Errorf("failed to check TO2 completion status: %w", err)
		}
		if completed {
			return fmt.Errorf("voucher has already completed TO2 and credential reuse is disabled")
		}
	}

	// 5. Verify the voucher exists in our database
	exists, err := ownerState.Voucher.Exists(ctx, voucher.Header.Val.GUID)
	if err != nil {
		return fmt.Errorf("failed to check voucher existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("voucher not found in database")
	}

	return nil
}

// publicKeysEqual compares two public keys for equality
func publicKeysEqual(a, b crypto.PublicKey) bool {
	// Try using the Equal method if available (works for RSA, ECDSA, Ed25519)
	if eq, ok := a.(interface{ Equal(crypto.PublicKey) bool }); ok {
		return eq.Equal(b)
	}

	// Fallback: compare string representations
	// This handles custom key types that don't implement Equal
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
