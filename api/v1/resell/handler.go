// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package resell

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Server implements the StrictServerInterface for Voucher Resell operations
type Server struct {
	VoucherState *state.VoucherPersistentState  // Voucher state for database operations
	OwnerKey     *state.OwnerKeyPersistentState // Owner key for resell operations
}

func NewServer(voucherState *state.VoucherPersistentState, ownerKey *state.OwnerKeyPersistentState) Server {
	return Server{
		VoucherState: voucherState,
		OwnerKey:     ownerKey,
	}
}

var _ StrictServerInterface = (*Server)(nil)

// ResellVoucher implements POST /resell/{guid}
func (s *Server) ResellVoucher(ctx context.Context, request ResellVoucherRequestObject) (ResellVoucherResponseObject, error) {
	guidHex := request.Guid

	if !utils.IsValidGUID(guidHex) {
		return ResellVoucher400TextResponse("GUID is not a valid GUID"), nil
	}

	guidBytes, err := hex.DecodeString(guidHex)
	if err != nil {
		return ResellVoucher400TextResponse("Invalid GUID format"), nil
	}

	var guid protocol.GUID
	copy(guid[:], guidBytes)

	body, err := io.ReadAll(request.Body)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return ResellVoucher413TextResponse("Request payload too large"), nil
		}
		slog.Error("Failed to read request body", "error", err)
		return ResellVoucher500TextResponse("Failure to read the request body"), nil
	}

	blk, rest := pem.Decode(body)
	if blk == nil {
		return ResellVoucher400TextResponse("Invalid PEM content"), nil
	}
	if len(rest) > 0 {
		slog.Debug("Extra data after PEM block ignored", "extra_bytes", len(rest))
	}

	nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		slog.Error("Error parsing x.509 public key", "error", err)
		return ResellVoucher400TextResponse("Error parsing x.509 public key"), nil
	}

	// Use the state's ExtendVoucher method which handles the transaction.
	// It returns the extended voucher and the exact CBOR bytes persisted to
	// the database, so we use those directly instead of re-marshaling.
	_, extendedCBOR, err := s.VoucherState.ExtendVoucher(ctx, guid, s.OwnerKey.Signer(), nextOwner)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) {
			slog.Warn("Voucher not found for resell", "guid", guidHex)
			return ResellVoucher404TextResponse("Voucher not found for the specified GUID"), nil
		}
		if errors.Is(err, state.ErrUnsupportedKeyType) {
			slog.Warn("Unsupported public key type for resell", "guid", guidHex, "error", err)
			return ResellVoucher400TextResponse("Unsupported public key type"), nil
		}
		slog.Error("ExtendVoucher failed", "guid", guidHex, "error", err)
		return ResellVoucher500TextResponse("Error reselling voucher"), nil
	}

	// Encode as PEM using the CBOR bytes returned by ExtendVoucher
	pemBlock := &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedCBOR,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	return ResellVoucher200ApplicationxPemFileResponse{
		Body: bytes.NewReader(pemBytes),
	}, nil
}
