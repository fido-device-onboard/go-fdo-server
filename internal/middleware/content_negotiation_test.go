package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elnormous/contenttype"
)

func TestContentNegotiationMiddleware(t *testing.T) {
	availableTypes := []contenttype.MediaType{
		contenttype.NewMediaType("application/json"),
		contenttype.NewMediaType("application/x-pem-file"),
	}

	tests := []struct {
		name         string
		acceptHeader string
		operationID  string
		expectedType string
	}{
		{
			name:         "No Accept header defaults to JSON",
			acceptHeader: "",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: application/json",
			acceptHeader: "application/json",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/json",
		},
		{
			name:         "Accept: application/x-pem-file",
			acceptHeader: "application/x-pem-file",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: application/json with quality",
			acceptHeader: "application/json;q=0.9",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: multiple types, JSON first",
			acceptHeader: "application/json, application/x-pem-file",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/json",
		},
		{
			name:         "Accept: multiple types, PEM first",
			acceptHeader: "application/x-pem-file, application/json",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: wildcard",
			acceptHeader: "*/*",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: quality factors - PEM preferred",
			acceptHeader: "application/json;q=0.8, application/x-pem-file;q=0.9",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: quality factors - JSON preferred",
			acceptHeader: "application/json;q=0.9, application/x-pem-file;q=0.8",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
		{
			name:         "Accept: quality factors with spaces",
			acceptHeader: "application/json; q=0.5, application/x-pem-file; q=1.0",
			operationID:  "GetTrustedDeviceCACertByFingerprint",
			expectedType: "application/x-pem-file",
		},
		{
			name:         "Accept: unsupported type falls back to default",
			acceptHeader: "text/html, application/xml",
			operationID:  "ListTrustedDeviceCACerts",
			expectedType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedContentType string
			mockHandler := func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
				capturedContentType = PreferredContentType(ctx)
				return nil, nil
			}

			mw := ContentNegotiationMiddleware(availableTypes, "application/json")
			wrappedHandler := mw(mockHandler, tt.operationID)

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}
			w := httptest.NewRecorder()

			_, _ = wrappedHandler(context.Background(), w, req, nil)

			if capturedContentType != tt.expectedType {
				t.Errorf("Expected content type %q, got %q", tt.expectedType, capturedContentType)
			}
		})
	}
}
