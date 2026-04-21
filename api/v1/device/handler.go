// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package device

import (
	"context"
	"encoding/hex"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// Server implements the StrictServerInterface for Device listing (v1 - legacy behavior)
type Server struct {
	VoucherState *state.VoucherPersistentState
}

func NewServer(voucherState *state.VoucherPersistentState) Server {
	return Server{
		VoucherState: voucherState,
	}
}

var _ StrictServerInterface = (*Server)(nil)

// ListDevices implements GET /v1/owner/devices
func (s *Server) ListDevices(ctx context.Context, request ListDevicesRequestObject) (ListDevicesResponseObject, error) {
	slog.Debug("Listing owner devices")

	filters := make(map[string]interface{})

	// Handle old_guid filter
	if request.Params.OldGuid != nil {
		guidHex := *request.Params.OldGuid
		if !utils.IsValidGUID(guidHex) {
			return ListDevices400TextResponse("Invalid GUID"), nil
		}

		decoded, err := hex.DecodeString(guidHex)
		if err != nil {
			return ListDevices400TextResponse("Invalid GUID format"), nil
		}
		filters["old_guid"] = decoded
	}

	devices, err := s.VoucherState.ListDevices(ctx, filters)
	if err != nil {
		slog.Error("Error listing devices", "error", err)
		return ListDevices500TextResponse("Internal server error"), nil
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

	return ListDevices200JSONResponse(result), nil
}
