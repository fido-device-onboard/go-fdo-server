// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/api/v2/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

type Server struct {
	RvInfo *state.RvInfoState
}

func NewServer(rvInfoState *state.RvInfoState) Server {
	return Server{RvInfo: rvInfoState}
}

// Make sure we conform to StrictServerInterface
var _ StrictServerInterface = (*Server)(nil)

// GetRendezvousInfo retrieves the current rendezvous information configuration
func (s *Server) GetRendezvousInfo(ctx context.Context, request GetRendezvousInfoRequestObject) (GetRendezvousInfoResponseObject, error) {
	slog.Debug("GetRendezvousInfo called")

	// Fetch RV info from state (returns [][]protocol.RvInstruction)
	rvInstructions, err := s.RvInfo.GetRvInfo(ctx)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			// Return empty array if no configuration set
			slog.Debug("No RV info found, returning empty array")
			return GetRendezvousInfo200JSONResponse{}, nil
		}
		slog.Error("failed to fetch RV info", "error", err)
		return GetRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to fetch rendezvous info"},
		}, nil
	}

	// Convert protocol format to V2 API format
	rvInfo, err := RVInfoFromProtocol(rvInstructions)
	if err != nil {
		slog.Error("failed to convert RV info from protocol format", "error", err)
		return GetRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to format rendezvous info"},
		}, nil
	}

	return GetRendezvousInfo200JSONResponse(rvInfo), nil
}

// UpdateRendezvousInfo updates the rendezvous information configuration
func (s *Server) UpdateRendezvousInfo(ctx context.Context, request UpdateRendezvousInfoRequestObject) (UpdateRendezvousInfoResponseObject, error) {
	slog.Debug("UpdateRendezvousInfo called")

	if request.Body == nil {
		slog.Warn("UpdateRendezvousInfo called with nil body")
		return UpdateRendezvousInfo400JSONResponse{
			components.BadRequest{Message: "request body is required"},
		}, nil
	}

	// Convert V2 API format to protocol format
	rvInstructions, err := RVInfoToProtocol(*request.Body)
	if err != nil {
		slog.Warn("invalid RV info format", "error", err)
		return UpdateRendezvousInfo400JSONResponse{
			components.BadRequest{Message: "invalid rendezvous info"},
		}, nil
	}

	// Atomically insert or update (prevents race conditions)
	if err := s.RvInfo.CreateOrUpdateRvInfo(ctx, rvInstructions); err != nil {
		slog.Error("failed to save RV info", "error", err)
		return UpdateRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to save rendezvous info"},
		}, nil
	}

	// Re-read from DB to return the persisted state (round-trip verification)
	savedInstructions, err := s.RvInfo.GetRvInfo(ctx)
	if err != nil {
		slog.Error("failed to read back saved RV info", "error", err)
		return UpdateRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to read back saved rendezvous info"},
		}, nil
	}

	savedRvInfo, err := RVInfoFromProtocol(savedInstructions)
	if err != nil {
		slog.Error("failed to convert saved RV info from protocol format", "error", err)
		return UpdateRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to format saved rendezvous info"},
		}, nil
	}

	return UpdateRendezvousInfo200JSONResponse(savedRvInfo), nil
}

// DeleteRendezvousInfo removes the rendezvous information configuration
func (s *Server) DeleteRendezvousInfo(ctx context.Context, request DeleteRendezvousInfoRequestObject) (DeleteRendezvousInfoResponseObject, error) {
	slog.Debug("DeleteRendezvousInfo called")

	// Fetch current RV info before deletion (to return it)
	rvInstructions, err := s.RvInfo.GetRvInfo(ctx)
	if err != nil {
		if errors.Is(err, state.ErrRvInfoNotFound) {
			// No configuration set, return empty array
			slog.Debug("No RV info to delete, returning empty array")
			return DeleteRendezvousInfo200JSONResponse{}, nil
		}
		slog.Error("failed to fetch RV info for deletion", "error", err)
		return DeleteRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to delete rendezvous info"},
		}, nil
	}

	// Convert protocol format to V2 API format
	rvInfo, err := RVInfoFromProtocol(rvInstructions)
	if err != nil {
		slog.Error("failed to convert RV info from protocol format", "error", err)
		return DeleteRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to format rendezvous info"},
		}, nil
	}

	// Delete from state
	if err := s.RvInfo.DeleteRvInfo(ctx); err != nil {
		slog.Error("failed to delete RV info", "error", err)
		return DeleteRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to delete rendezvous info"},
		}, nil
	}

	// Return the deleted configuration
	return DeleteRendezvousInfo200JSONResponse(rvInfo), nil
}
