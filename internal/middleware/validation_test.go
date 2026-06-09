package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Test", "version": "1.0.0"},
  "paths": {
    "/health": {
      "get": {
        "operationId": "GetHealth",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/api/v1/widgets": {
      "get": {
        "operationId": "ListWidgetsV1",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/api/v2/widgets": {
      "get": {
        "operationId": "ListWidgets",
        "parameters": [
          {
            "name": "limit",
            "in": "query",
            "schema": {"type": "integer", "minimum": 1, "maximum": 100}
          }
        ],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/api/v2/widgets/{id}": {
      "get": {
        "operationId": "GetWidget",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "schema": {"type": "string", "pattern": "^[a-f0-9]{8}$"}
          }
        ],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/api/v2/widgets/{id}/settings": {
      "put": {
        "operationId": "UpdateWidgetSettings",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "schema": {"type": "string"}
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["port"],
                "properties": {
                  "port": {"type": "integer", "minimum": 1, "maximum": 65535}
                }
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestOpenAPIValidationMiddleware_FiltersV2Paths(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	// V2 path (after prefix strip) should be routed
	req := httptest.NewRequest(http.MethodGet, "/widgets?limit=10", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid V2 path, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIValidationMiddleware_RejectsOutOfRangeQueryParam(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	tests := []struct {
		name  string
		query string
	}{
		{"limit too low", "/widgets?limit=0"},
		{"limit too high", "/widgets?limit=999"},
		{"limit negative", "/widgets?limit=-1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestOpenAPIValidationMiddleware_RejectsInvalidPathParam(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/widgets/NOT-VALID", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid path param, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIValidationMiddleware_AcceptsValidPathParam(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/widgets/deadbeef", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid path param, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIValidationMiddleware_RejectsInvalidRequestBody(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	body := `{"port": 0}`
	req := httptest.NewRequest(http.MethodPut, "/widgets/abc/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for out-of-range port in body, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIValidationMiddleware_AcceptsValidRequestBody(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	body := `{"port": 8443}`
	req := httptest.NewRequest(http.MethodPut, "/widgets/abc/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid body, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIValidationMiddleware_ErrorFormat(t *testing.T) {
	mw := OpenAPIValidationMiddleware([]byte(testSpec))
	handler := mw(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/widgets?limit=0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content type, got %q", ct)
	}

	var errResp errorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestOpenAPIValidationMiddleware_PanicsOnInvalidSpec(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid spec")
		}
	}()
	OpenAPIValidationMiddleware([]byte(`not valid json`))
}
