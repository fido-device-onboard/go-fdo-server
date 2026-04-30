package middleware

import "net/http"

// BodySizeMiddleware wraps an http.Handler, limiting the request body to limitBytes.
// Uses http.MaxBytesReader so that exceeding the limit returns a 413 error
// instead of silently truncating the body.
func BodySizeMiddleware(limitBytes int64, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limitBytes)
		next.ServeHTTP(w, r)
	}
}
