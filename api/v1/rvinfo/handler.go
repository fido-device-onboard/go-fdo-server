// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
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

	rvInstructions, err := s.RvInfoState.GetRVInfo(ctx)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			slog.Error("No rvInfo found")
			return GetRendezvousInfo404TextResponse("No rvInfo found"), nil
		}
		slog.Error("Error fetching rvInfo", "error", err)
		return GetRendezvousInfo500TextResponse("Error fetching rvInfo"), nil
	}

	// Convert protocol format to V1 API format
	rvInfo, err := RVInfoV1FromProtocol(rvInstructions)
	if err != nil {
		slog.Error("Error converting rvInfo from protocol format", "error", err)
		return GetRendezvousInfo500TextResponse("Error fetching rvInfo"), nil
	}

	return GetRendezvousInfo200JSONResponse(rvInfo), nil
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
	rvInstructions, err := RVInfoV1ToProtocol(*request.Body)
	if err != nil {
		slog.Error("Error converting to protocol instructions", "error", err)
		return CreateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Try to create (will fail if already exists)
	err = s.RvInfoState.CreateRVInfo(ctx, rvInstructions)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("rvInfo already exists (constraint)", "error", err)
			return CreateRendezvousInfo409TextResponse("rvInfo already exists"), nil
		}
		if errors.Is(err, state.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			return CreateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
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

	// Check if exists first (v1 behavior: PUT fails if doesn't exist)
	_, err := s.RvInfoState.GetRVInfo(ctx)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			slog.Error("rvInfo does not exist, cannot update")
			return UpdateRendezvousInfo404TextResponse("rvInfo does not exist"), nil
		}
		slog.Error("Error checking rvInfo", "error", err)
		return UpdateRendezvousInfo500TextResponse("Error updating rvInfo"), nil
	}

	// Convert V1 API format to protocol format
	rvInstructions, err := RVInfoV1ToProtocol(*request.Body)
	if err != nil {
		slog.Error("Error converting to protocol instructions", "error", err)
		return UpdateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
	}

	// Update the configuration
	err = s.RvInfoState.UpdateRVInfo(ctx, rvInstructions)
	if err != nil {
		if errors.Is(err, state.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			return UpdateRendezvousInfo400TextResponse("Invalid rvInfo"), nil
		}
		slog.Error("Error updating rvInfo", "error", err)
		return UpdateRendezvousInfo500TextResponse("Error updating rvInfo"), nil
	}

	slog.Debug("rvInfo updated")

	return UpdateRendezvousInfo200JSONResponse(*request.Body), nil
}
