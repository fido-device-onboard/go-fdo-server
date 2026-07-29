// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockAuthenticator struct {
	name     string
	identity *Identity
	err      error
}

func (m *mockAuthenticator) Name() string { return m.name }
func (m *mockAuthenticator) Authenticate(_ context.Context, _ *http.Request) (*Identity, error) {
	return m.identity, m.err
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d", w.Code, want)
	}
}

func TestAuthNMiddleware_ExcludedPath(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/health"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathExactMatch(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/health"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/health-check", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_ExcludedPathPrefixMatch(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docs/index.html", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathTrailingSlash(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docs/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathNoFalsePrefix(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/api/docs/"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/docsextra", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_NoAuthenticators(t *testing.T) {
	mw := AuthNMiddleware(nil, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_SuccessfulAuth(t *testing.T) {
	id := NewIdentity("user-1", "", "", nil, []string{"vouchers:read"}, nil)
	authn := &mockAuthenticator{name: "mock", identity: id}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)

	var gotIdentity *Identity
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, _ = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
	if gotIdentity == nil || gotIdentity.Subject() != "user-1" {
		t.Errorf("expected identity with subject 'user-1' on context, got %v", gotIdentity)
	}
}

func TestAuthNMiddleware_InvalidCredentials(t *testing.T) {
	authn := &mockAuthenticator{name: "mock", err: ErrInvalidCredentials}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_InternalError(t *testing.T) {
	authn := &mockAuthenticator{name: "mock", err: errors.New("database connection failed")}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestAuthNMiddleware_FallThrough(t *testing.T) {
	skip := &mockAuthenticator{name: "skip", identity: nil, err: nil}
	id := NewIdentity("user-1", "", "", nil, nil, nil)
	match := &mockAuthenticator{name: "match", identity: id}
	mw := AuthNMiddleware([]Authenticator{skip, match}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_Allowed(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers": {"vouchers:read"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{"vouchers:read", "vouchers:write", "vouchers:delete"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_Forbidden(t *testing.T) {
	routeScopes := map[string][]string{
		"DELETE /vouchers/{guid}": {"vouchers:delete"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{"vouchers:read"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodDelete, "/vouchers/abc123", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusForbidden)
}

func TestAuthZMiddleware_NoScopesRequired(t *testing.T) {
	mw := AuthZMiddleware(map[string][]string{})

	id := NewIdentity("", "", "", nil, []string{}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/unprotected", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthZMiddleware_DoubleSlashNormalized(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers/{guid}": {"vouchers:read"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers//abc123", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusForbidden)
}

func TestAuthNMiddleware_WrappedInvalidCredentials(t *testing.T) {
	wrappedErr := fmt.Errorf("bad API key: %w", ErrInvalidCredentials)
	authn := &mockAuthenticator{name: "mock", err: wrappedErr}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthNMiddleware_ErrorResponseFormat(t *testing.T) {
	authn := &mockAuthenticator{name: "mock", err: ErrInvalidCredentials}
	mw := AuthNMiddleware([]Authenticator{authn}, nil)
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/v2/vouchers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp authErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "authentication failed" {
		t.Errorf("message = %q, want %q", resp.Message, "authentication failed")
	}
}

func TestAuthZMiddleware_ForbiddenResponseFormat(t *testing.T) {
	routeScopes := map[string][]string{
		"DELETE /vouchers/{guid}": {"vouchers:delete"},
	}
	mw := AuthZMiddleware(routeScopes)
	id := NewIdentity("", "", "", nil, []string{"vouchers:read"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodDelete, "/vouchers/abc123", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp authErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != "insufficient permissions" {
		t.Errorf("message = %q, want %q", resp.Message, "insufficient permissions")
	}
}

func TestAuthZMiddleware_NoIdentityOnContext(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers": {"vouchers:read"},
	}
	mw := AuthZMiddleware(routeScopes)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers", nil)
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestAuthZMiddleware_LiteralRoutePreferredOverParameterized(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers/{guid}":  {"vouchers:read"},
		"GET /vouchers/summary": {"vouchers:summary"},
	}
	mw := AuthZMiddleware(routeScopes)

	id := NewIdentity("", "", "", nil, []string{"vouchers:summary"}, nil)
	inner := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/vouchers/summary", nil)
	r = r.WithContext(ContextWithIdentity(r.Context(), id))
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}

func TestAuthNMiddleware_ExcludedPathTraversalNormalized(t *testing.T) {
	mw := AuthNMiddleware(nil, []string{"/health"})
	handler := mw(okHandler())

	r := httptest.NewRequest(http.MethodGet, "/api/../health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assertStatus(t, w, http.StatusOK)
}
