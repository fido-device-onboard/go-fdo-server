// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

func AuthNMiddleware(authenticators []Authenticator, excludedPaths []string) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExcluded(path.Clean(r.URL.Path), excludedPaths) {
				next.ServeHTTP(w, r)
				return
			}

			for _, authn := range authenticators {
				identity, err := authn.Authenticate(r.Context(), r)
				if err != nil {
					if errors.Is(err, ErrInvalidCredentials) {
						slog.Debug("Authentication failed", "authenticator", authn.Name(), "error", err)
						writeAuthError(w, http.StatusUnauthorized, "authentication failed")
					} else {
						slog.Error("Internal authentication error", "authenticator", authn.Name(), "error", err)
						writeAuthError(w, http.StatusInternalServerError, "internal server error")
					}
					return
				}
				if identity != nil {
					ctx := ContextWithIdentity(r.Context(), identity)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			writeAuthError(w, http.StatusUnauthorized, "authentication required")
		})
	}
}

func AuthZMiddleware(routeScopes map[string][]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, _ := IdentityFromContext(r.Context())
			if identity == nil {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			required := lookupRequiredScopes(r.Method, path.Clean(r.URL.Path), routeScopes)
			if len(required) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if !identity.HasAllScopes(required) {
				slog.Debug("Authorization denied",
					"subject", identity.Subject(),
					"required", required,
					"have", identity.Scopes(),
					"path", r.URL.Path,
				)
				writeAuthError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isExcluded(cleanedPath string, excluded []string) bool {
	for _, entry := range excluded {
		if strings.HasSuffix(entry, "/") {
			trimmed := strings.TrimSuffix(entry, "/")
			if cleanedPath == trimmed || strings.HasPrefix(cleanedPath, entry) {
				return true
			}
		} else {
			if cleanedPath == entry {
				return true
			}
		}
	}
	return false
}

func lookupRequiredScopes(method, reqPath string, routeScopes map[string][]string) []string {
	var bestPattern string
	var bestScopes []string
	bestWildcards := -1

	for pattern, scopes := range routeScopes {
		if !matchRoute(method, reqPath, pattern) {
			continue
		}
		wildcards := countWildcards(pattern)
		if bestWildcards == -1 || wildcards < bestWildcards || (wildcards == bestWildcards && pattern < bestPattern) {
			bestPattern = pattern
			bestScopes = scopes
			bestWildcards = wildcards
		}
	}

	return bestScopes
}

func countWildcards(pattern string) int {
	count := 0
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return 0
	}
	for _, seg := range strings.Split(parts[1], "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			count++
		}
	}
	return count
}

func matchRoute(method, path, pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		return false
	}
	patternMethod, patternPath := parts[0], parts[1]

	if method != patternMethod {
		return false
	}

	return matchPath(path, patternPath)
}

func matchPath(path, pattern string) bool {
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(pathParts) != len(patternParts) {
		return false
	}

	for i, pp := range patternParts {
		if strings.HasPrefix(pp, "{") && strings.HasSuffix(pp, "}") {
			continue
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}

type authErrorResponse struct {
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(authErrorResponse{Message: message})
}
