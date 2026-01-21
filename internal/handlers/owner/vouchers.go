package owner

import (
	"bytes"
	"crypto"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// VerifyVoucherOwnership verifies the ownership voucher belongs to this owner.
// It checks that the voucher's owner key matches one of the server's configured keys.
func VerifyVoucherOwnership(ov *fdo.Voucher, state *db.State) error {
	state.Mutex.RLock()
	ownerPKeys := state.OwnerKeys
	state.Mutex.RUnlock()
	if len(ownerPKeys) == 0 {
		return fmt.Errorf("ownerPKeys must contain at least one owner public key")
	}

	expectedPubKey, err := ov.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("unable to parse owner public key from voucher: %w", err)
	}

	// Cast is needed to call Equal()
	// See: https://pkg.go.dev/crypto#PublicKey
	if !slices.ContainsFunc(ownerPKeys, expectedPubKey.(interface{ Equal(crypto.PublicKey) bool }).Equal) {
		return fmt.Errorf("voucher owner key does not match any of the server's configured keys")
	}

	return nil
}

// VerifyOwnershipVoucher performs header field validation and cryptographic verification.
// Note: The following validations can be performed by the device during TO2, not by owner-server,
// so are not included in this verification:
//   - HMAC verification (ov.VerifyHeader): Owner server does not have the device HMAC secret
//   - Manufacturer key hash verification (ov.VerifyManufacturerKey): Requires trusted manufacturer
//     key hashes to be configured (owner server has no source for these hashes)
func VerifyOwnershipVoucher(ov *fdo.Voucher, state *db.State) error {
	// TODO: Investigate whether protocol version should be verified for all messages received by the server
	// and whether FDOProtocolVersion const should be moved to a common package (e.g., api package)
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
	// even for rv bypass there needs to be some instruction, not empty array
	if len(ov.Header.Val.RvInfo) == 0 {
		return fmt.Errorf("invalid voucher: RvInfo is empty")
	}

	// Cryptographic Integrity Verification
	if err := ov.VerifyEntries(); err != nil {
		return fmt.Errorf("signature chain (manufacturer -> owner transfers) verification failed: %w", err)
	}
	if err := ov.VerifyCertChainHash(); err != nil {
		return fmt.Errorf("device certificate chain hash verification failed: %w", err)
	}

	// Verify device certificate chain against trusted device CAs
	state.Mutex.RLock()
	certPool := state.TrustedDeviceCACertPool
	state.Mutex.RUnlock()
	if err := ov.VerifyDeviceCertChain(certPool); err != nil {
		return fmt.Errorf("device certificate chain verification failed: %w", err)
	}
	if err := ov.VerifyManufacturerCertChain(nil); err != nil {
		return fmt.Errorf("manufacturer certificate chain verification failed: %w", err)
	}

	return nil
}

// VerifyVoucher performs comprehensive verification of an ownership voucher
// as per FDO spec section 3.4.6. This combines both ownership and integrity checks.
func VerifyVoucher(ov *fdo.Voucher, state *db.State) error {
	slog.Debug("verifying voucher ownership")
	if err := VerifyVoucherOwnership(ov, state); err != nil {
		slog.Error("verifying voucher ownership", "err", err)
		return err
	}

	slog.Debug("verifying ownership voucher")
	if err := VerifyOwnershipVoucher(ov, state); err != nil {
		slog.Error("verifying ownership voucher", "err", err)
		return err
	}

	return nil
}

// InsertVoucherHandler verifies and inserts vouchers. Background TO0 is handled by the owner server.
func InsertVoucherHandler(state *db.State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failure to read the request body", http.StatusInternalServerError)
			return
		}

		block, rest := pem.Decode(body)
		for ; block != nil; block, rest = pem.Decode(rest) {
			if block.Type != "OWNERSHIP VOUCHER" {
				slog.Debug("Got unknown label type", "type", block.Type)
				continue
			}
			var ov fdo.Voucher
			if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
				slog.Debug("Unable to decode cbor", "block", block.Bytes)
				http.Error(w, "Unable to decode cbor", http.StatusBadRequest)
				return
			}

			// Ov Verification
			if err := VerifyVoucher(&ov, state); err != nil {
				slog.Error("Ownership voucher verification failed", "guid", ov.Header.Val.GUID[:], "err", err)
				http.Error(w, "Invalid ownership voucher", http.StatusBadRequest)
				return
			}

			// Check for duplicate vouchers in database
			if dbOv, err := db.FetchVoucher(map[string]interface{}{"guid": ov.Header.Val.GUID[:]}); err == nil {
				if bytes.Equal(block.Bytes, dbOv.CBOR) {
					slog.Debug("Voucher already exists", "guid", ov.Header.Val.GUID[:])
					continue
				}
				slog.Debug("Voucher guid already exists. not overwriting it", "guid", ov.Header.Val.GUID[:])
				continue
			}

			// Insert voucher into database
			slog.Debug("Inserting voucher", "GUID", ov.Header.Val.GUID)

			if err := db.InsertVoucher(db.Voucher{GUID: ov.Header.Val.GUID[:], CBOR: block.Bytes, DeviceInfo: ov.Header.Val.DeviceInfo, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
				slog.Debug("Error inserting into database", "error", err.Error())
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		if len(bytes.TrimSpace(rest)) > 0 {
			http.Error(w, "Unable to decode PEM content", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}
}
