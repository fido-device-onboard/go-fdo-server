// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Server implements the StrictServerInterface for RvInfo management (v1 - legacy behavior)
type Server struct {
	RvInfoState *state.RvInfoState
}

func NewServer(rvInfoState *state.RvInfoState) Server {
	return Server{RvInfoState: rvInfoState}
}

var _ StrictServerInterface = (*Server)(nil)

// GetRendezvousInfo retrieves the current RvInfo configuration
// Returns 404 if no configuration exists (v1 legacy behavior)
func (s *Server) GetRendezvousInfo(ctx context.Context, request GetRendezvousInfoRequestObject) (GetRendezvousInfoResponseObject, error) {
	slog.Debug("Fetching rvInfo")

	rvInstructions, err := s.RvInfoState.GetRvInfo(ctx)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			slog.Error("No rvInfo found")
			return GetRendezvousInfo404TextResponse("No rvInfo found"), nil
		}
		slog.Error("Error fetching rvInfo", "error", err)
		return GetRendezvousInfo500TextResponse("Error fetching rvInfo"), nil
	}

	// Convert protocol format to V1 API format
	rendezvousInfo, err := RendezvousInfoFromProtocol(rvInstructions)
	if err != nil {
		slog.Error("Error converting rvInfo from protocol format", "error", err)
		return GetRendezvousInfo500TextResponse("Error fetching rvInfo"), nil
	}

	return GetRendezvousInfo200JSONResponse(rendezvousInfo), nil
}

// CreateRendezvousInfo creates the RvInfo configuration
// Returns 409 if configuration already exists (v1 legacy behavior)
func (s *Server) CreateRendezvousInfo(ctx context.Context, request CreateRendezvousInfoRequestObject) (CreateRendezvousInfoResponseObject, error) {
	slog.Debug("Creating rvInfo")
	if request.Body == nil {
		slog.Error("no rvInfo received")
		return CreateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Convert V1 API format to protocol format
	rvInstructions, err := RendezvousInfoToProtocol(*request.Body)
	if err != nil {
		slog.Error("Error converting to protocol instructions", "error", err)
		return CreateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Try to create (will fail if already exists)
	err = s.RvInfoState.CreateRvInfo(ctx, rvInstructions)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("rvInfo already exists (constraint)", "error", err)
			return CreateRendezvousInfo409TextResponse("rvInfo already exists"), nil
		}
		slog.Error("Error inserting rvInfo", "error", err)
		return CreateRendezvousInfo500TextResponse("Error inserting rvInfo"), nil
	}

	slog.Debug("rvInfo created")

	return CreateRendezvousInfo201JSONResponse(*request.Body), nil
}

// UpdateRendezvousInfo updates the RvInfo configuration
// Returns 404 if configuration doesn't exist (v1 legacy behavior - requires POST first)
func (s *Server) UpdateRendezvousInfo(ctx context.Context, request UpdateRendezvousInfoRequestObject) (UpdateRendezvousInfoResponseObject, error) {
	slog.Debug("Updating rvInfo")
	if request.Body == nil {
		return UpdateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Convert V1 API format to protocol format
	rvInstructions, err := RendezvousInfoToProtocol(*request.Body)
	if err != nil {
		slog.Error("Error converting to protocol instructions", "error", err)
		return UpdateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Atomic update — returns ErrRvInfoNotFound if no row exists
	err = s.RvInfoState.UpdateRvInfo(ctx, rvInstructions)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			return UpdateRendezvousInfo404TextResponse("rvInfo does not exist"), nil
		}
		slog.Error("Error updating rvInfo", "error", err)
		return UpdateRendezvousInfo500TextResponse("Error updating rvInfo"), nil
	}

	slog.Debug("rvInfo updated")

	return UpdateRendezvousInfo200JSONResponse(*request.Body), nil
}

// MigrateJSONToCBOR performs a one-time migration of V1 JSON RvInfo to CBOR format.
// Should be called once after database initialization, before serving requests.
// If data is already in CBOR format or no data exists, it does nothing.
func (s *Server) MigrateJSONToCBOR(ctx context.Context) error {
	rawValue, err := s.RvInfoState.ReadRawRvInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to check rvinfo for migration: %w", err)
	}
	if rawValue == nil {
		slog.Debug("No RvInfo to migrate")
		return nil
	}

	// Try to unmarshal as CBOR - if successful, already migrated
	var testCBOR [][]protocol.RvInstruction
	if err = cbor.Unmarshal(rawValue, &testCBOR); err == nil {
		slog.Debug("RvInfo already in CBOR format, no migration needed")
		return nil
	}

	slog.Info("Migrating RvInfo from V1 JSON to CBOR format")

	// Parse as V1 JSON
	var rendezvousInfo RendezvousInfo
	if err = json.Unmarshal(rawValue, &rendezvousInfo); err != nil {
		return fmt.Errorf("rvinfo is neither valid CBOR nor V1 JSON: %w", err)
	}

	// Convert V1 format to protocol format
	rvInstructions, err := RendezvousInfoToProtocol(rendezvousInfo)
	if err != nil {
		return fmt.Errorf("failed to convert V1 JSON to protocol format: %w", err)
	}

	// Marshal to CBOR
	rvInfoCBOR, err := cbor.Marshal(rvInstructions)
	if err != nil {
		return fmt.Errorf("failed to marshal rvinfo to CBOR: %w", err)
	}

	// Update database with CBOR format
	if err := s.RvInfoState.UpdateRawRvInfo(ctx, rvInfoCBOR); err != nil {
		return fmt.Errorf("failed to update rvinfo to CBOR: %w", err)
	}

	slog.Info("Successfully migrated RvInfo from V1 JSON to CBOR format")
	return nil
}
