// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0
package handlersTest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
)

func TestRVTO2Addr_PostConflictOnDuplicate(t *testing.T) {
	setupTestDB(t)

	handler := http.HandlerFunc(handlers.RVTO2AddrHandler)
	body := []byte(`[{"dns":"owner.example","port":"8082","protocol":"http"}]`)

	// First POST should create (201)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/owner/redirect", bytes.NewReader(body))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected 201 on first POST, got %d", rec1.Code)
	}

	// Second POST should conflict (409)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/owner/redirect", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 on second POST, got %d", rec2.Code)
	}
}

func TestRVTO2Addr_Put404ThenCreateThenUpdateAndGet(t *testing.T) {
	setupTestDB(t)

	get := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/owner/redirect", nil)
		rec := httptest.NewRecorder()
		handlers.RVTO2AddrHandler(rec, req)
		return rec
	}
	put := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/owner/redirect", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handlers.RVTO2AddrHandler(rec, req)
		return rec
	}
	post := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/owner/redirect", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handlers.RVTO2AddrHandler(rec, req)
		return rec
	}

	// PUT before create -> 404
	putBody := []byte(`[{"dns":"owner.example","port":"8082","protocol":"http"}]`)
	if rec := put(putBody); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on PUT before create, got %d", rec.Code)
	}

	// POST create -> 201
	postBody := []byte(`[{"dns":"owner.example","port":"8082","protocol":"http"}]`)
	if rec := post(postBody); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on POST create, got %d", rec.Code)
	}

	// PUT update -> 200
	updateBody := []byte(`[{"dns":"owner-updated","port":"9090","protocol":"http"}]`)
	if rec := put(updateBody); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT update, got %d", rec.Code)
	}

	// GET -> updated value present
	rec := get()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != string(updateBody) {
		t.Fatalf("expected body %q, got %q", string(updateBody), got)
	}

}
