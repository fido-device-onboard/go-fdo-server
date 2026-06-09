package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

// OpenAPIValidationMiddleware returns HTTP middleware that validates incoming
// requests against the V2 paths in the given aggregate OpenAPI spec.
//
// The aggregate spec contains paths prefixed with /api/v2/ (and possibly /api/v1/
// and /health). This function filters to only /api/v2/ paths and strips the
// prefix so that path matching works on a mux mounted behind
// http.StripPrefix("/api/v2", ...).
//
// Panics if the spec cannot be loaded — the spec is embedded at compile time,
// so a load failure indicates a build defect.
func OpenAPIValidationMiddleware(specJSON []byte) func(http.Handler) http.Handler {
	spec, err := openapi3.NewLoader().LoadFromData(specJSON)
	if err != nil {
		panic("middleware: failed to load embedded OpenAPI spec: " + err.Error())
	}

	filtered := openapi3.NewPaths()
	for path, item := range spec.Paths.Map() {
		if stripped, ok := strings.CutPrefix(path, "/api/v2"); ok {
			filtered.Set(stripped, item)
		}
	}
	spec.Paths = filtered
	spec.Servers = nil

	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		SilenceServersWarning: true,
		DoNotValidateServers:  true,
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandlerWithOpts: validationErrorHandler,
	})
}

type errorResponse struct {
	Message string `json:"message"`
}

func validationErrorHandler(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	status := opts.StatusCode
	if status == 0 {
		status = http.StatusBadRequest
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Message: err.Error()})
}
