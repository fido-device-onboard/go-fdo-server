// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package health

import (
	"context"

	"github.com/fido-device-onboard/go-fdo-server/internal/version"
)

type ServerHealth struct{}

// Make sure we conform to StrictServerInterface
var _ StrictServerInterface = (*ServerHealth)(nil)

// GetHealth responds with the version and status
// TODO: make sure the database is online and responds to queries.
func (*ServerHealth) GetHealth(ctx context.Context, request GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200JSONResponse{Version: version.VERSION, Status: "OK"}, nil
}
