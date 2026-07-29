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

func ParseRouteScopes(specJSON []byte) (map[string][]string, error) {
	spec, err := openapi3.NewLoader().LoadFromData(specJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	routeScopes := make(map[string][]string)

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
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

			normalizedPath := path
			if stripped, ok := strings.CutPrefix(path, "/api/v2/"); ok {
				normalizedPath = "/" + stripped
			}

			key := method + " " + normalizedPath
			routeScopes[key] = scopeSlice
			slog.Debug("Registered route scope", "route", key, "scopes", scopeSlice)
		}
	}

	return routeScopes, nil
}

// ParsePublicPaths returns paths from the OpenAPI spec whose operations have
// security explicitly set to an empty array (security: []), meaning they
// require no authentication.
func ParsePublicPaths(specJSON []byte) ([]string, error) {
	spec, err := openapi3.NewLoader().LoadFromData(specJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	seen := make(map[string]struct{})
	var paths []string

	for specPath, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			if op.Security == nil {
				continue
			}
			if len(*op.Security) == 0 {
				if _, ok := seen[specPath]; !ok {
					paths = append(paths, specPath)
					seen[specPath] = struct{}{}
				}
			}
		}
	}

	slices.Sort(paths)
	return paths, nil
}

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
