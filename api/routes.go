// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package api

import (
	"io"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	transport "github.com/fido-device-onboard/go-fdo/http"
)

// HTTPHandler handles HTTP requests
type HTTPHandler struct {
	handler *transport.Handler
	state   *gorm.DB
}

func rateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func bodySizeMiddleware(limitBytes int64, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.LimitReader(r.Body, limitBytes),
			Closer: r.Body,
		}
		next.ServeHTTP(w, r)
	}
}

// NewHTTPHandler creates a new HTTPHandler
func NewHTTPHandler(handler *transport.Handler, state *gorm.DB) *HTTPHandler {
	return &HTTPHandler{handler: handler, state: state}
}

// RegisterRoutes registers the routes for the HTTP server
func (h *HTTPHandler) RegisterRoutes(apiRouter *http.ServeMux) *http.ServeMux {
	handler := http.NewServeMux()
	handler.Handle("POST /fdo/101/msg/{msg}", h.handler)
	if apiRouter != nil {
		apiHandler := rateLimitMiddleware(rate.NewLimiter(2, 10),
			bodySizeMiddleware(1<<20, /* 1MB */
				apiRouter,
			),
		)
		handler.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))

	}
	return handler
}
