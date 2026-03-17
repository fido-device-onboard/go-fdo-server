// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
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

	// Fetch RV info from state
	rvInfoJSON, err := s.RvInfo.FetchRvInfoJSON(ctx)
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

	// Unmarshal JSON to components.RVInfo
	var rvInfo components.RVInfo
	if err := json.Unmarshal(rvInfoJSON, &rvInfo); err != nil {
		slog.Error("failed to unmarshal RV info", "error", err)
		return GetRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to parse rendezvous info"},
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

	// Marshal request body to JSON
	rvInfoJSON, err := json.Marshal(request.Body)
	if err != nil {
		slog.Error("failed to marshal RV info", "error", err)
		return UpdateRendezvousInfo400JSONResponse{
			components.BadRequest{Message: "invalid rendezvous info format"},
		}, nil
	}

	// Try to update first
	err = s.RvInfo.UpdateRvInfo(ctx, rvInfoJSON)
	if errors.Is(err, state.ErrRvInfoNotFound) {
		// No existing record, insert instead
		if err := s.RvInfo.InsertRvInfo(ctx, rvInfoJSON); err != nil {
			if errors.Is(err, state.ErrInvalidRvInfo) {
				slog.Warn("invalid RV info provided", "error", err)
				return UpdateRendezvousInfo400JSONResponse{
					components.BadRequest{Message: "invalid rendezvous info: " + err.Error()},
				}, nil
			}
			slog.Error("failed to insert RV info", "error", err)
			return UpdateRendezvousInfo500JSONResponse{
				components.InternalServerError{Message: "failed to save rendezvous info"},
			}, nil
		}
	} else if err != nil {
		if errors.Is(err, state.ErrInvalidRvInfo) {
			slog.Warn("invalid RV info provided", "error", err)
			return UpdateRendezvousInfo400JSONResponse{
				components.BadRequest{Message: "invalid rendezvous info: " + err.Error()},
			}, nil
		}
		slog.Error("failed to update RV info", "error", err)
		return UpdateRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to update rendezvous info"},
		}, nil
	}

	// Return the updated RV info
	return UpdateRendezvousInfo200JSONResponse(*request.Body), nil
}

// DeleteRendezvousInfo removes the rendezvous information configuration
func (s *Server) DeleteRendezvousInfo(ctx context.Context, request DeleteRendezvousInfoRequestObject) (DeleteRendezvousInfoResponseObject, error) {
	slog.Debug("DeleteRendezvousInfo called")

	// Fetch current RV info before deletion (to return it)
	rvInfoJSON, err := s.RvInfo.FetchRvInfoJSON(ctx)
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

	// Unmarshal to return in response
	var rvInfo components.RVInfo
	if err := json.Unmarshal(rvInfoJSON, &rvInfo); err != nil {
		slog.Error("failed to unmarshal RV info", "error", err)
		return DeleteRendezvousInfo500JSONResponse{
			components.InternalServerError{Message: "failed to parse rendezvous info"},
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
