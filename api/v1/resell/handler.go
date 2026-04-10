// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package resell

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
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

	// Read PEM body
	body, err := io.ReadAll(request.Body)
	if err != nil {
		slog.Debug(err.Error())
		return ResellVoucher500TextResponse("Failure to read the request body"), nil
	}

	blk, _ := pem.Decode(body)
	if blk == nil {
		return ResellVoucher500TextResponse("Invalid PEM content"), nil
	}

	nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		slog.Debug(err.Error())
		return ResellVoucher500TextResponse("Error parsing x.509 public key"), nil
	}

	// Use the state's ExtendVoucher method which handles the transaction
	extended, err := s.VoucherState.ExtendVoucher(ctx, guid, s.OwnerKey.Signer(), nextOwner)
	if err != nil {
		slog.Debug("ExtendVoucher failed", "error", err)
		return ResellVoucher500TextResponse("Error reselling voucher"), nil
	}

	ovBytes, err := cbor.Marshal(extended)
	if err != nil {
		slog.Debug(err.Error())
		return ResellVoucher500TextResponse("Error marshaling voucher"), nil
	}

	// Encode as PEM
	pemBlock := &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: ovBytes,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	return ResellVoucher200ApplicationxPemFileResponse{
		Body: bytes.NewReader(pemBytes),
	}, nil
}
