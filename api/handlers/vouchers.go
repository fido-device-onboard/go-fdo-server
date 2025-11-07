// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

func GetVoucherHandler(w http.ResponseWriter, r *http.Request) {
	guidHex := r.URL.Query().Get("guid")
	deviceInfo := r.URL.Query().Get("device_info")

	filters := make(map[string]interface{})

	if guidHex != "" {
		if !utils.IsValidGUID(guidHex) {
			http.Error(w, fmt.Sprintf("Invalid GUID: %s", guidHex), http.StatusBadRequest)
			return
		}

		guid, err := hex.DecodeString(guidHex)
		if err != nil {
			http.Error(w, "Invalid GUID format", http.StatusBadRequest)
			return
		}
		filters["guid"] = guid
	}

	if deviceInfo != "" {
		filters["device_info"] = deviceInfo
	}

	vouchers, err := db.QueryVouchers(filters, false)
	if err != nil {
		slog.Debug("Error querying vouchers", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(vouchers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetVoucherByGUIDHandler returns a PEM-encoded voucher by path GUID.
func GetVoucherByGUIDHandler(w http.ResponseWriter, r *http.Request) {
	guidHex := r.PathValue("guid")
	if !utils.IsValidGUID(guidHex) {
		http.Error(w, "Invalid GUID", http.StatusBadRequest)
		return
	}
	guid, err := hex.DecodeString(guidHex)
	if err != nil {
		http.Error(w, "Invalid GUID format", http.StatusBadRequest)
		return
	}
	voucher, err := db.FetchVoucher(map[string]interface{}{"guid": guid})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Voucher not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	if err := pem.Encode(w, &pem.Block{Type: "OWNERSHIP VOUCHER", Bytes: voucher.CBOR}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// VerifyVoucherOwnership verifies the ownership voucher belongs to this owner.
// It checks that the voucher's owner key matches one of the server's configured keys.
func VerifyVoucherOwnership(ov *fdo.Voucher, ownerPKeys []crypto.PublicKey) error {
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
func VerifyOwnershipVoucher(ov *fdo.Voucher) error {
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
	if err := ov.VerifyDeviceCertChain(nil); err != nil {
		return fmt.Errorf("device certificate chain verification failed: %w", err)
	}
	if err := ov.VerifyManufacturerCertChain(nil); err != nil {
		return fmt.Errorf("manufacturer certificate chain verification failed: %w", err)
	}

	return nil
}

// VerifyVoucher performs comprehensive verification of an ownership voucher
// as per FDO spec section 3.4.6. This combines both ownership and integrity checks.
func VerifyVoucher(ov *fdo.Voucher, ownerPKeys []crypto.PublicKey) error {
	if err := VerifyVoucherOwnership(ov, ownerPKeys); err != nil {
		return err
	}

	if err := VerifyOwnershipVoucher(ov); err != nil {
		return err
	}

	return nil
}

func InsertVoucherHandler(ownerPKeys []crypto.PublicKey) http.HandlerFunc {
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
			if err := VerifyVoucher(&ov, ownerPKeys); err != nil {
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

func ResellHandler(to2Server *fdo.TO2Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidHex := r.PathValue("guid")

		if !utils.IsValidGUID(guidHex) {
			http.Error(w, "GUID is not a valid GUID", http.StatusBadRequest)
			return
		}

		guidBytes, err := hex.DecodeString(guidHex)
		if err != nil {
			http.Error(w, "Invalid GUID format", http.StatusBadRequest)
			slog.Debug(err.Error())
			return
		}

		var guid protocol.GUID
		copy(guid[:], guidBytes)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failure to read the request body", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}
		blk, _ := pem.Decode(body)
		if blk == nil {
			http.Error(w, "Invalid PEM content", http.StatusInternalServerError)
			return
		}
		nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
		if err != nil {
			http.Error(w, "Error parsing x.509 public key", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		extended, err := to2Server.Resell(context.TODO(), guid, nextOwner, nil)
		if err != nil {
			http.Error(w, "Error reselling voucher", http.StatusInternalServerError)
			slog.Debug(err.Error())
			if extended != nil {
				if err := to2Server.Vouchers.AddVoucher(context.TODO(), extended); err != nil {
					slog.Debug("Error adding voucher to database", "error", err.Error())
				}
			}
			return
		}
		ovBytes, err := cbor.Marshal(extended)
		if err != nil {
			http.Error(w, "Error marshaling voucher", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/x-pem-file")
		if err := pem.Encode(w, &pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: ovBytes,
		}); err != nil {
			slog.Debug("Error encoding voucher", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
