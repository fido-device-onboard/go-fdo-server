// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package health

import (
	"context"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

type Server struct {
	State *state.HealthState
}

func NewServer(state *state.HealthState) Server {
	return Server{State: state}
}

// Make sure we conform to StrictServerInterface
var _ StrictServerInterface = (*Server)(nil)

// GetHealth responds with the version and status
func (s *Server) GetHealth(ctx context.Context, request GetHealthRequestObject) (GetHealthResponseObject, error) {
	if err := s.State.Ping(); err != nil {
		slog.Error("database error", "err", err)
		return GetHealth500JSONResponse{components.InternalServerError{Message: "database error"}}, nil
	}
	return GetHealth200JSONResponse{HealthStatusJSONResponse{Version: version.VERSION, Status: "OK", Message: "the service is up and running"}}, nil
}
