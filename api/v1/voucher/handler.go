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
	"net/http"

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
		slog.Error("Error querying vouchers", "error", err)
		return ListVouchers500TextResponse("Internal server error"), nil
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
		slog.Error("Error fetching voucher", "error", err)
		return GetVoucherByGUID500TextResponse("Internal server error"), nil
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
	body, err := io.ReadAll(request.Body)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return InsertVoucher413TextResponse("Request payload too large"), nil
		}
		return InsertVoucher500TextResponse("Failure to read the request body"), nil
	}

	var processed int
	block, rest := pem.Decode(body)
	for ; block != nil; block, rest = pem.Decode(rest) {
		if block.Type != "OWNERSHIP VOUCHER" {
			slog.Debug("Got unknown label type", "type", block.Type)
			continue
		}
		processed++
		var ov fdo.Voucher
		if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
			slog.Debug("Failed to unmarshal CBOR voucher")
			return InsertVoucher400TextResponse("Unable to decode cbor"), nil
		}

		// Verify voucher
		if err := s.verifyVoucher(&ov); err != nil {
			slog.Error("Ownership voucher verification failed", "guid", ov.Header.Val.GUID[:], "error", err)
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
				existingCBOR, err := cbor.Marshal(existingVoucher)
				if err != nil {
					slog.Error("Failed to marshal existing voucher for comparison", "guid", ov.Header.Val.GUID[:], "error", err)
				} else if bytes.Equal(block.Bytes, existingCBOR) {
					slog.Debug("Voucher already exists", "guid", ov.Header.Val.GUID[:])
					continue
				}
			}
			slog.Debug("Voucher guid already exists. not overwriting it", "guid", ov.Header.Val.GUID[:])
			continue
		}

		// Insert voucher into database
		slog.Debug("Inserting voucher", "guid", ov.Header.Val.GUID)

		if err := s.VoucherState.AddVoucher(ctx, &ov); err != nil {
			slog.Debug("Error inserting into database", "error", err.Error())
			return InsertVoucher500TextResponse("Internal server error"), nil
		}
	}

	if len(bytes.TrimSpace(rest)) > 0 {
		return InsertVoucher400TextResponse("Unable to decode PEM content"), nil
	}

	if processed == 0 {
		return InsertVoucher400TextResponse("No valid ownership vouchers found"), nil
	}

	return InsertVoucher200TextResponse(""), nil
}

// verifyVoucher performs comprehensive verification as per the old handler
func (s *Server) verifyVoucher(ov *fdo.Voucher) error {
	// Verify ownership
	if err := utils.VerifyVoucherOwnership(ov, s.OwnerPKeys); err != nil {
		return err
	}

	// Verify integrity — nil certPool skips device CA check inside VerifyOwnershipVoucher
	if err := utils.VerifyOwnershipVoucher(ov, nil); err != nil {
		return err
	}

	// V1 backward compat: validate device cert chain against system trust roots.
	// V2 validates against explicitly configured device CAs instead.
	if ov.CertChain != nil {
		if err := ov.VerifyDeviceCertChain(nil); err != nil {
			return fmt.Errorf("device certificate chain verification failed: %w", err)
		}
	}

	return nil
}
