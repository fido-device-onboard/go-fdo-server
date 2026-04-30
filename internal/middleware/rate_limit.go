package middleware

import (
	"net/http"

	"golang.org/x/time/rate"
)

// RateLimitMiddleware wraps an http.Handler with token-bucket rate limiting.
func RateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}
