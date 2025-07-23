// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package api

import (
	"io"
	"net/http"

	"golang.org/x/time/rate"

	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
)

// HTTPHandler handles HTTP requests
type HTTPHandler struct {
	handler *transport.Handler
	rvInfo  *[][]protocol.RvInstruction
	state   *sqlite.DB
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
func NewHTTPHandler(handler *transport.Handler, rvInfo *[][]protocol.RvInstruction, state *sqlite.DB) *HTTPHandler {
	return &HTTPHandler{handler: handler, rvInfo: rvInfo, state: state}
}

// RegisterRoutes registers the routes for the HTTP server
func (h *HTTPHandler) RegisterRoutes() *http.ServeMux {
	apiRouter := http.NewServeMux()
	apiRouter.Handle("/rvinfo", handlers.RvInfoHandler(h.rvInfo))
	apiRouter.HandleFunc("/owner/redirect", handlers.OwnerInfoHandler)
	apiRouter.Handle("GET /to0/{guid}", handlers.To0Handler(h.rvInfo, h.state))
	apiRouter.HandleFunc("GET /vouchers", handlers.GetVoucherHandler)
	apiRouter.Handle("POST /owner/vouchers", handlers.InsertVoucherHandler(h.rvInfo))

	apiHandler := rateLimitMiddleware(rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, /* 1MB */
			apiRouter,
		),
	)

	handler := http.NewServeMux()
	handler.Handle("POST /fdo/101/msg/{msg}", h.handler)
	handler.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))
	handler.HandleFunc("/health", handlers.HealthHandler)
	return handler
}
