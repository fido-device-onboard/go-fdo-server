// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package device

import (
	"context"
	"encoding/hex"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/api/v2/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
)

// Server implements the StrictServerInterface for Device listing
var _ StrictServerInterface = (*Server)(nil)

type Server struct {
	VoucherState *state.VoucherPersistentState
}

func NewServer(voucherState *state.VoucherPersistentState) Server {
	return Server{
		VoucherState: voucherState,
	}
}

// ListDevices implements GET /v2/devices
func (s *Server) ListDevices(ctx context.Context, request ListDevicesRequestObject) (ListDevicesResponseObject, error) {
	slog.Debug("Listing owner devices")

	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	var filter state.DeviceFilter

	// Redundant with OpenAPI validation middleware (pattern: ^[0-9a-fA-F]{32}$)
	if request.Params.OldGuid != nil {
		decoded, err := hex.DecodeString(*request.Params.OldGuid)
		if err != nil {
			slog.Warn("Invalid GUID format", "guid", *request.Params.OldGuid, "error", err)
			return ListDevices400JSONResponse{
				BadRequestJSONResponse: components.BadRequestJSONResponse{
					Message: "Invalid GUID format",
				},
			}, nil
		}
		filter.OldGUID = decoded
	}

	devices, total, err := s.VoucherState.ListDevices(ctx, filter, limit, offset)
	if err != nil {
		slog.Error("Error listing devices", "error", err)
		return ListDevices500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Internal server error",
			},
		}, nil
	}

	// Convert state.Device to generated Device type
	result := make([]Device, len(devices))
	for i, d := range devices {
		device := Device{
			Guid:         hex.EncodeToString(d.GUID),
			OldGuid:      hex.EncodeToString(d.OldGUID),
			DeviceInfo:   d.DeviceInfo,
			CreatedAt:    d.CreatedAt,
			UpdatedAt:    d.UpdatedAt,
			To2Completed: d.TO2Completed,
		}
		if d.TO2CompletedAt != nil {
			device.To2CompletedAt = d.TO2CompletedAt
		}
		result[i] = device
	}

	return ListDevices200JSONResponse(DevicesPaginated{
		Limit:   limit,
		Offset:  offset,
		Total:   int(total),
		Devices: result,
	}), nil
}
