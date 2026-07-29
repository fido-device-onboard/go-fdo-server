// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"slices"
	"testing"
)

func TestParseRouteScopes(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/vouchers": {
				"get": {
					"operationId": "ListVouchers",
					"x-required-scopes": ["vouchers:read"],
					"responses": {"200": {"description": "OK"}}
				},
				"post": {
					"operationId": "ImportVouchers",
					"x-required-scopes": ["vouchers:write"],
					"responses": {"201": {"description": "Created"}}
				}
			},
			"/api/v2/vouchers/{guid}": {
				"delete": {
					"operationId": "DeleteVoucher",
					"x-required-scopes": ["vouchers:delete"],
					"responses": {"204": {"description": "Deleted"}}
				}
			},
			"/health": {
				"get": {
					"operationId": "Health",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}

	tests := []struct {
		key  string
		want []string
	}{
		{"GET /vouchers", []string{"vouchers:read"}},
		{"POST /vouchers", []string{"vouchers:write"}},
		{"DELETE /vouchers/{guid}", []string{"vouchers:delete"}},
	}
	for _, tt := range tests {
		got, ok := scopes[tt.key]
		if !ok {
			t.Errorf("missing key %q", tt.key)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("scopes[%q] = %v, want %v", tt.key, got, tt.want)
		}
	}
	if _, ok := scopes["GET /health"]; ok {
		t.Error("health endpoint should not have scopes")
	}
}

func TestParseRouteScopes_NoExtension(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/resource": {
				"get": {
					"operationId": "GetResource",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}
	if len(scopes) != 0 {
		t.Errorf("got %d scope entries, want 0", len(scopes))
	}
}

func TestParseRouteScopes_MultipleScopes(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/api/v2/admin": {
				"post": {
					"operationId": "AdminAction",
					"x-required-scopes": ["vouchers:read", "vouchers:delete"],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`)

	scopes, err := ParseRouteScopes(spec)
	if err != nil {
		t.Fatalf("ParseRouteScopes: %v", err)
	}
	got := scopes["POST /admin"]
	want := []string{"vouchers:read", "vouchers:delete"}
	if !slices.Equal(got, want) {
		t.Errorf("scopes[POST /admin] = %v, want %v", got, want)
	}
}

func TestParseRouteScopes_InvalidJSON(t *testing.T) {
	spec := []byte(`{invalid json`)
	_, err := ParseRouteScopes(spec)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestCollectAllScopes(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers":           {"vouchers:read"},
		"POST /vouchers":          {"vouchers:write"},
		"DELETE /vouchers/{guid}": {"vouchers:delete"},
		"GET /devices":            {"devices:read"},
	}
	got := CollectAllScopes(routeScopes)
	want := []string{"devices:read", "vouchers:delete", "vouchers:read", "vouchers:write"}
	if !slices.Equal(got, want) {
		t.Errorf("CollectAllScopes = %v, want %v", got, want)
	}
}

func TestCollectAllScopes_Deduplication(t *testing.T) {
	routeScopes := map[string][]string{
		"GET /vouchers":  {"vouchers:read", "vouchers:write"},
		"POST /vouchers": {"vouchers:write"},
	}
	got := CollectAllScopes(routeScopes)
	want := []string{"vouchers:read", "vouchers:write"}
	if !slices.Equal(got, want) {
		t.Errorf("CollectAllScopes = %v, want %v (should deduplicate)", got, want)
	}
}

func TestCollectAllScopes_Empty(t *testing.T) {
	routeScopes := map[string][]string{}
	got := CollectAllScopes(routeScopes)
	if len(got) != 0 {
		t.Errorf("CollectAllScopes = %v, want empty slice", got)
	}
}
