// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

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

// BodySizeMiddleware wraps an http.Handler, limiting the request body to limitBytes.
// Uses http.MaxBytesReader so that exceeding the limit returns a 413 error
// instead of silently truncating the body.
func BodySizeMiddleware(limitBytes int64, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limitBytes)
		next.ServeHTTP(w, r)
	}
}
