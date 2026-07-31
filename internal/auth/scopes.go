// SPDX-FileCopyrightText: (C) 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package auth

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const requiredScopesExtension = "x-required-scopes"

// ParseRouteScopes parses an OpenAPI spec and extracts x-required-scopes for each operation.
// Returns a map of "METHOD /path" -> []string{scopes}.
// Paths are normalized by stripping /api/v2 prefix if present.
func ParseRouteScopes(specJSON []byte) (map[string][]string, error) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	routeScopes := make(map[string][]string)

	for _, path := range spec.Paths.InMatchingOrder() {
		pathItem := spec.Paths.Find(path)
		if pathItem == nil {
			continue
		}

		for method, op := range pathItem.Operations() {
			if op == nil {
				continue
			}

			ext, ok := op.Extensions[requiredScopesExtension]
			if !ok {
				continue
			}

			scopeSlice, err := parseStringSliceExtension(ext)
			if err != nil {
				return nil, fmt.Errorf("invalid %s on %s %s: %w", requiredScopesExtension, method, path, err)
			}

			if len(scopeSlice) == 0 {
				continue
			}

			// Normalize path by stripping /api/v2 prefix
			normalizedPath := path
			if stripped, ok := strings.CutPrefix(path, "/api/v2"); ok {
				normalizedPath = stripped
			}

			key := method + " " + normalizedPath
			routeScopes[key] = scopeSlice
			slog.Debug("Registered route scope", "route", key, "scopes", scopeSlice)
		}
	}

	return routeScopes, nil
}

// CollectAllScopes returns a sorted, deduplicated list of all unique scopes from the route scopes map.
func CollectAllScopes(routeScopes map[string][]string) []string {
	seen := make(map[string]struct{})
	for _, scopes := range routeScopes {
		for _, s := range scopes {
			seen[s] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	slices.Sort(result)
	return result
}

// parseStringSliceExtension parses an OpenAPI extension value as []string.
func parseStringSliceExtension(ext interface{}) ([]string, error) {
	raw, ok := ext.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", ext)
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string in array, got %T", v)
		}
		result = append(result, s)
	}
	return result, nil
}
