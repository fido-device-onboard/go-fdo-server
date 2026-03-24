// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Server implements the StrictServerInterface for Voucher management (v1 - legacy behavior)
type Server struct {
	VoucherState *state.VoucherPersistentState // Voucher state for database operations
	OwnerPKeys   []crypto.PublicKey            // Owner public keys for verification
}

func NewServer(voucherState *state.VoucherPersistentState, ownerPKeys []crypto.PublicKey) Server {
	return Server{
		VoucherState: voucherState,
		OwnerPKeys:   ownerPKeys,
	}
}

var _ StrictServerInterface = (*Server)(nil)

// ListVouchers implements GET /v1/vouchers
func (s *Server) ListVouchers(ctx context.Context, request ListVouchersRequestObject) (ListVouchersResponseObject, error) {
	filters := make(map[string]interface{})

	// Handle GUID filter
	if request.Params.Guid != nil {
		guidHex := *request.Params.Guid
		if !utils.IsValidGUID(guidHex) {
			return ListVouchers400TextResponse(fmt.Sprintf("Invalid GUID: %s", guidHex)), nil
		}

		guid, err := hex.DecodeString(guidHex)
		if err != nil {
			return ListVouchers400TextResponse("Invalid GUID format"), nil
		}
		filters["guid"] = guid
	}

	// Handle device_info filter
	if request.Params.DeviceInfo != nil {
		filters["device_info"] = *request.Params.DeviceInfo
	}

	// Query vouchers using state
	var guidFilter, deviceInfoFilter *string
	if guid, ok := filters["guid"].([]byte); ok {
		guidHex := hex.EncodeToString(guid)
		guidFilter = &guidHex
	}
	if devInfo, ok := filters["device_info"].(string); ok {
		deviceInfoFilter = &devInfo
	}

	vouchers, _, err := s.VoucherState.ListVouchers(ctx, 0, 0, guidFilter, deviceInfoFilter, nil, "updated_at", "desc")
	if err != nil {
		slog.Debug("Error querying vouchers", "error", err)
		return ListVouchers500TextResponse(err.Error()), nil
	}

	// Convert to VoucherMetadata
	result := make([]VoucherMetadata, len(vouchers))
	for i, v := range vouchers {
		result[i] = VoucherMetadata{
			Guid:       hex.EncodeToString(v.GUID),
			DeviceInfo: v.DeviceInfo,
			CreatedAt:  v.CreatedAt,
			UpdatedAt:  v.UpdatedAt,
		}
	}

	return ListVouchers200JSONResponse(result), nil
}

// GetVoucherByGUID implements GET /v1/vouchers/{guid}
func (s *Server) GetVoucherByGUID(ctx context.Context, request GetVoucherByGUIDRequestObject) (GetVoucherByGUIDResponseObject, error) {
	guidHex := request.Guid
	if !utils.IsValidGUID(guidHex) {
		return GetVoucherByGUID400TextResponse("Invalid GUID"), nil
	}

	guidBytes, err := hex.DecodeString(guidHex)
	if err != nil {
		return GetVoucherByGUID400TextResponse("Invalid GUID format"), nil
	}

	var guid protocol.GUID
	copy(guid[:], guidBytes)

	voucher, err := s.VoucherState.Voucher(ctx, guid)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return GetVoucherByGUID404TextResponse("Voucher not found"), nil
		}
		return GetVoucherByGUID500TextResponse(err.Error()), nil
	}

	// Marshal voucher to CBOR
	voucherCBOR, err := cbor.Marshal(voucher)
	if err != nil {
		return GetVoucherByGUID500TextResponse("Error marshaling voucher"), nil
	}

	// Encode as PEM
	pemBlock := &pem.Block{Type: "OWNERSHIP VOUCHER", Bytes: voucherCBOR}
	pemBytes := pem.EncodeToMemory(pemBlock)

	return GetVoucherByGUID200ApplicationxPemFileResponse{
		Body: bytes.NewReader(pemBytes),
	}, nil
}

// InsertVoucher implements POST /v1/vouchers
func (s *Server) InsertVoucher(ctx context.Context, request InsertVoucherRequestObject) (InsertVoucherResponseObject, error) {
	// Read PEM body
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return InsertVoucher500TextResponse("Failure to read the request body"), nil
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
			return InsertVoucher400TextResponse("Unable to decode cbor"), nil
		}

		// Verify voucher
		if err := s.verifyVoucher(&ov); err != nil {
			slog.Error("Ownership voucher verification failed", "guid", ov.Header.Val.GUID[:], "err", err)
			return InsertVoucher400TextResponse("Invalid ownership voucher"), nil
		}

		// Check for duplicate vouchers in database
		exists, err := s.VoucherState.Exists(ctx, ov.Header.Val.GUID)
		if err != nil {
			slog.Debug("Error checking voucher existence", "error", err)
			return InsertVoucher500TextResponse("Internal server error"), nil
		}
		if exists {
			// Check if it's the exact same voucher (idempotent)
			existingVoucher, err := s.VoucherState.Voucher(ctx, ov.Header.Val.GUID)
			if err == nil {
				existingCBOR, _ := cbor.Marshal(existingVoucher)
				if bytes.Equal(block.Bytes, existingCBOR) {
					slog.Debug("Voucher already exists", "guid", ov.Header.Val.GUID[:])
					continue
				}
			}
			slog.Debug("Voucher guid already exists. not overwriting it", "guid", ov.Header.Val.GUID[:])
			continue
		}

		// Insert voucher into database
		slog.Debug("Inserting voucher", "GUID", ov.Header.Val.GUID)

		if err := s.VoucherState.AddVoucher(ctx, &ov); err != nil {
			slog.Debug("Error inserting into database", "error", err.Error())
			return InsertVoucher500TextResponse("Internal server error"), nil
		}
	}

	if len(bytes.TrimSpace(rest)) > 0 {
		return InsertVoucher400TextResponse("Unable to decode PEM content"), nil
	}

	return InsertVoucher200TextResponse(""), nil
}

// verifyVoucher performs comprehensive verification as per the old handler
func (s *Server) verifyVoucher(ov *fdo.Voucher) error {
	// Verify ownership
	if err := s.verifyVoucherOwnership(ov); err != nil {
		return err
	}

	// Verify integrity
	return s.verifyOwnershipVoucher(ov)
}

// verifyVoucherOwnership verifies the ownership voucher belongs to this owner
func (s *Server) verifyVoucherOwnership(ov *fdo.Voucher) error {
	if len(s.OwnerPKeys) == 0 {
		return fmt.Errorf("ownerPKeys must contain at least one owner public key")
	}

	expectedPubKey, err := ov.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("unable to parse owner public key from voucher: %w", err)
	}

	// Cast is needed to call Equal()
	if !slices.ContainsFunc(s.OwnerPKeys, expectedPubKey.(interface{ Equal(crypto.PublicKey) bool }).Equal) {
		return fmt.Errorf("voucher owner key does not match any of the server's configured keys")
	}

	return nil
}

// verifyOwnershipVoucher performs header field validation and cryptographic verification
func (s *Server) verifyOwnershipVoucher(ov *fdo.Voucher) error {
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
	if err := ov.VerifyDeviceCertChain(nil); err != nil {
		return fmt.Errorf("device certificate chain verification failed: %w", err)
	}
	if err := ov.VerifyManufacturerCertChain(nil); err != nil {
		return fmt.Errorf("manufacturer certificate chain verification failed: %w", err)
	}

	return nil
}
