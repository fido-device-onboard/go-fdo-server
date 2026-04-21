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

	// 2. Verify the voucher is owned by this server
	voucherOwnerPubKey, err := voucher.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("failed to extract owner public key from voucher: %w", err)
	}

	// Compare the voucher's owner public key with our server's owner public key
	serverOwnerPubKey := ownerKey.Public()
	if !publicKeysEqual(voucherOwnerPubKey, serverOwnerPubKey) {
		return fmt.Errorf("voucher is not owned by this server (public key mismatch)")
	}

	// 3. Check if TO2 has already been completed for this voucher
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

	// 4. Verify the voucher exists in our database
	exists, err := ownerState.Voucher.Exists(ctx, voucher.Header.Val.GUID)
	if err != nil {
		return fmt.Errorf("failed to check voucher existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("voucher not found in database")
	}

	return nil
}

// publicKeysEqual compares two public keys for equality using the Equal method.
// All FDO-supported key types (RSA, ECDSA, Ed25519) implement the Equal method
// as of Go 1.15, making this a reliable comparison.
func publicKeysEqual(a, b crypto.PublicKey) bool {
	// Try using the Equal method if available (works for RSA, ECDSA, Ed25519)
	if eq, ok := a.(interface{ Equal(crypto.PublicKey) bool }); ok {
		return eq.Equal(b)
	}

	// If the key type doesn't implement Equal, we cannot reliably compare it.
	// The previous fallback using fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	// was unreliable because it could compare pointer addresses for struct types,
	// leading to false negatives when comparing different objects representing
	// the same cryptographic key.
	//
	// Since all FDO-supported key types implement Equal(), this fallback should
	// never be reached in normal operation. If we do reach here, it indicates
	// an unsupported key type, so we conservatively return false.
	return false
}
