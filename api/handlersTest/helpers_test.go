// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"net/http"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
)

// createVoucherByGUIDHandler creates a test wrapper for GetVoucherByGUIDHandler
// that extracts the GUID from the URL path
func createVoucherByGUIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract GUID from URL path (simple implementation for testing)
		path := r.URL.Path
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			// Set the path value for the handler to extract
			r.SetPathValue("guid", parts[len(parts)-1])
		}
		handlers.GetVoucherByGUIDHandler(w, r)
	}
}
