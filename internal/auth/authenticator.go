// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"net/http"
)

// Authenticator extracts and validates credentials from an HTTP request.
//
// Authenticate returns:
//   - (*Identity, nil) on successful authentication
//   - (nil, nil) when no credentials are present (the request is unauthenticated
//     but another authenticator may handle it)
//   - (nil, error) when credentials are present but invalid or an internal error occurs
type Authenticator interface {
	Name() string
	Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}
