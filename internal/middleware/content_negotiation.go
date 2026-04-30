package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/elnormous/contenttype"
	strictnethttp "github.com/oapi-codegen/runtime/strictmiddleware/nethttp"
)

type contextKey string

const (
	contentTypeKey contextKey = "preferred-content-type"
)

// ContentNegotiationMiddleware returns a strict middleware that negotiates the
// response content type from the request's Accept header. The caller supplies
// the set of media types the endpoint can produce and a default to use when
// negotiation fails or no Accept header is present.
func ContentNegotiationMiddleware(
	availableTypes []contenttype.MediaType,
	defaultType string,
) strictnethttp.StrictHTTPMiddlewareFunc {
	return func(f strictnethttp.StrictHTTPHandlerFunc, operationID string) strictnethttp.StrictHTTPHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
			preferred := defaultType
			if accept := r.Header.Get("Accept"); accept != "" {
				accepted, _, err := contenttype.GetAcceptableMediaType(r, availableTypes)
				if err == nil {
					preferred = strings.ToLower(accepted.String())
				}
			}
			ctx = context.WithValue(ctx, contentTypeKey, preferred)
			return f(ctx, w, r, request)
		}
	}
}

// PreferredContentType extracts the negotiated content type from the context.
func PreferredContentType(ctx context.Context) string {
	v, _ := ctx.Value(contentTypeKey).(string)
	return v
}
