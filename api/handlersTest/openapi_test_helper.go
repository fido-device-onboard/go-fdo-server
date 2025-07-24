// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlersTest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/errors"
	"github.com/pb33f/libopenapi/datamodel"
)

// OpenAPITestHelper provides utilities for OpenAPI validation testing
type OpenAPITestHelper struct {
	validator validator.Validator
}

// LoadOpenAPIDocument loads and parses the OpenAPI specification document
// with proper configuration for external reference resolution
func LoadOpenAPIDocument(t *testing.T) libopenapi.Document {
	t.Helper()

	// Get the path to the OpenAPI spec relative to the test file
	schemaPath := filepath.Join("..", "schema")

	// Read the OpenAPI specification file
	specBytes, err := os.ReadFile(filepath.Join(schemaPath, "openapi.yaml"))
	if err != nil {
		t.Fatalf("Failed to read OpenAPI spec file: %v", err)
	}

	// Get absolute path for BasePath to resolve external references correctly
	absSpecPath, err := filepath.Abs(schemaPath)
	if err != nil {
		t.Fatalf("Failed to get absolute path for schema directory: %v", err)
	}

	config := datamodel.DocumentConfiguration{
		AllowFileReferences:   true,
		AllowRemoteReferences: true,
		BasePath:              absSpecPath,
	}

	// create a new document from specification bytes
	document, err := libopenapi.NewDocumentWithConfiguration(specBytes, &config)
	if err != nil {
		t.Fatalf("Failed to parse OpenAPI document: %v", err)
	}

	return document
}

// NewOpenAPITestHelper creates a new OpenAPI test helper
func NewOpenAPITestHelper(t *testing.T) *OpenAPITestHelper {
	// Load the OpenAPI document using the shared helper
	document := LoadOpenAPIDocument(t)

	// Create validator
	v, validatorErrs := validator.NewValidator(document)
	if len(validatorErrs) > 0 {
		t.Fatalf("Failed to create validator: %v", validatorErrs)
	}

	return &OpenAPITestHelper{
		validator: v,
	}
}

// ValidateRequest validates an HTTP request against the OpenAPI specification
func (h *OpenAPITestHelper) ValidateRequest(t *testing.T, req *http.Request) {
	t.Helper()

	// Validate the request
	valid, validationErrs := h.validator.ValidateHttpRequest(req)
	if !valid {
		t.Errorf("Request validation failed:")
		for _, err := range validationErrs {
			t.Errorf("  - %s: %s", err.ValidationType, err.Message)
		}
	}
}

// ValidateResponse validates an HTTP response against the OpenAPI specification
func (h *OpenAPITestHelper) ValidateResponse(t *testing.T, req *http.Request, resp *http.Response) {
	t.Helper()

	// Validate the response
	valid, validationErrs := h.validator.ValidateHttpResponse(req, resp)
	if !valid {
		t.Errorf("Response validation failed:")
		for _, err := range validationErrs {
			t.Errorf("  - %s: %s", err.ValidationType, err.Message)
		}
	}
}

// ValidateRequestResponse validates both request and response against the OpenAPI specification
func (h *OpenAPITestHelper) ValidateRequestResponse(t *testing.T, req *http.Request, resp *http.Response) {
	t.Helper()

	// Validate both request and response together
	valid, validationErrs := h.validator.ValidateHttpRequestResponse(req, resp)
	if !valid {
		t.Errorf("Request/Response validation failed:")
		for _, err := range validationErrs {
			t.Errorf("  - %s: %s", err.ValidationType, err.Message)
		}
	}
}

// CreateRequestWithBody creates an HTTP request with a body
func CreateRequestWithBody(method, url, contentType string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req, nil
}

// ReadResponseBody reads the response body and creates a new reader for validation
func ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Create a new reader with the same content for validation
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return body, nil
}

// AssertNoValidationErrors checks that there are no validation errors
func AssertNoValidationErrors(t *testing.T, errors []*errors.ValidationError, context string) {
	t.Helper()
	if len(errors) > 0 {
		t.Errorf("%s validation errors:", context)
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

// LogValidationSuccess logs successful validation
func LogValidationSuccess(t *testing.T, endpoint, method string) {
	t.Helper()
	t.Logf("✅ %s %s - OpenAPI validation passed", method, endpoint)
}

// Common helper functions for test data validation

// BuildRequestBody formats request body based on content type
func BuildRequestBody(data, contentType string) string {
	if contentType == "application/json" {
		return fmt.Sprintf(`{"value":%s}`, data)
	}
	return data
}

// CompareJSONData performs semantic comparison of JSON data
func CompareJSONData(expected, actual string) error {
	var expectedData, actualData interface{}
	if err := json.Unmarshal([]byte(expected), &expectedData); err != nil {
		return fmt.Errorf("failed to parse expected data: %v", err)
	}
	if err := json.Unmarshal([]byte(actual), &actualData); err != nil {
		return fmt.Errorf("failed to parse actual data: %v", err)
	}

	expectedJSON, _ := json.Marshal(expectedData)
	actualJSON, _ := json.Marshal(actualData)

	if string(expectedJSON) != string(actualJSON) {
		return fmt.Errorf("data mismatch:\nExpected: %s\nActual: %s", expectedJSON, actualJSON)
	}
	return nil
}

// ExtractValueFromResponse extracts and normalizes value from response body
func ExtractValueFromResponse(response *http.Response) (string, error) {
	// Read response body
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}
	// Reset response body for OpenAPI validation
	response.Body = io.NopCloser(bytes.NewReader(respBody))

	// Unmarshal into db.Data structure
	var responseData db.Data
	if err := json.Unmarshal(respBody, &responseData); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Marshal just the Value field back to JSON string
	valueJSON, err := json.Marshal(responseData.Value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value: %v", err)
	}

	return string(valueJSON), nil
}

// VerifyResponseData extracts response data and compares it with expected data
func VerifyResponseData(expected string, response *http.Response) error {
	// Extract the value from response (includes body reset)
	actualData, err := ExtractValueFromResponse(response)
	if err != nil {
		return fmt.Errorf("failed to extract response data: %v", err)
	}

	// Compare with expected data
	if err := CompareJSONData(expected, actualData); err != nil {
		return fmt.Errorf("data validation failed: %v", err)
	}

	return nil
}

// HTTP Operation Helper Functions for Phase 2

// ExecutePostRequest executes a POST request and returns validation request and response
func ExecutePostRequest(t *testing.T, server *httptest.Server, endpoint, data, contentType string) (*http.Request, *http.Response) {
	t.Helper()

	// Create request body
	postBody := BuildRequestBody(data, contentType)

	// Create validation request for OpenAPI validation
	validationReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte(postBody)))
	if err != nil {
		t.Fatalf("Failed to create POST validation request: %v", err)
	}
	validationReq.Header.Set("Content-Type", contentType)

	// Create and execute actual request
	client := &http.Client{}
	execReq, err := http.NewRequest(http.MethodPost, server.URL+endpoint, bytes.NewReader([]byte(postBody)))
	if err != nil {
		t.Fatalf("Failed to create POST execution request: %v", err)
	}
	execReq.Header.Set("Content-Type", contentType)

	response, err := client.Do(execReq)
	if err != nil {
		t.Fatalf("Failed to execute POST request: %v", err)
	}

	// Validate expected status code
	if response.StatusCode != http.StatusCreated {
		t.Errorf("POST: Expected status %d, got %d", http.StatusCreated, response.StatusCode)
	}

	return validationReq, response
}

// ExecuteGetRequest executes a GET request and returns validation request and response
func ExecuteGetRequest(t *testing.T, server *httptest.Server, endpoint string) (*http.Request, *http.Response) {
	t.Helper()

	// Create validation request for OpenAPI validation
	validationReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("Failed to create GET validation request: %v", err)
	}

	// Execute actual request
	response, err := http.Get(server.URL + endpoint)
	if err != nil {
		t.Fatalf("Failed to execute GET request: %v", err)
	}

	// Validate expected status code
	if response.StatusCode != http.StatusOK {
		t.Errorf("GET: Expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	return validationReq, response
}

// ExecutePutRequest executes a PUT request and returns validation request and response
func ExecutePutRequest(t *testing.T, server *httptest.Server, endpoint, data, contentType string) (*http.Request, *http.Response) {
	t.Helper()

	// Create request body
	putBody := BuildRequestBody(data, contentType)

	// Create validation request for OpenAPI validation
	validationReq, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader([]byte(putBody)))
	if err != nil {
		t.Fatalf("Failed to create PUT validation request: %v", err)
	}
	validationReq.Header.Set("Content-Type", contentType)

	// Create and execute actual request
	client := &http.Client{}
	execReq, err := http.NewRequest(http.MethodPut, server.URL+endpoint, bytes.NewReader([]byte(putBody)))
	if err != nil {
		t.Fatalf("Failed to create PUT execution request: %v", err)
	}
	execReq.Header.Set("Content-Type", contentType)

	response, err := client.Do(execReq)
	if err != nil {
		t.Fatalf("Failed to execute PUT request: %v", err)
	}

	// Validate expected status code
	if response.StatusCode != http.StatusOK {
		t.Errorf("PUT: Expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	return validationReq, response
}
