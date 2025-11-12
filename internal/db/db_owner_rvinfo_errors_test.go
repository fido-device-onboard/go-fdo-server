package db

import (
	"errors"
	"testing"
)

func setupTestDBForOwnerRv(t *testing.T) {
	t.Helper()
	_, err := InitDb("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
}

func TestInsertRVTO2Addr_Invalid_ReturnsErrInvalidRVTO2Addr(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`{bad}`)
	err := InsertRVTO2Addr(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRVTO2Addr) {
		t.Fatalf("expected ErrInvalidRVTO2Addr, got %v", err)
	}
}

func TestUpdateRVTO2Addr_Invalid_ReturnsErrInvalidRVTO2Addr(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`{bad}`)
	err := UpdateRVTO2Addr(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRVTO2Addr) {
		t.Fatalf("expected ErrInvalidRVTO2Addr, got %v", err)
	}
}

func TestInsertRvInfo_Invalid_ReturnsErrInvalidRvInfo(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`[{bad}]`)
	err := InsertRvInfo(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRvInfo) {
		t.Fatalf("expected ErrInvalidRvInfo, got %v", err)
	}
}

func TestUpdateRvInfo_Invalid_ReturnsErrInvalidRvInfo(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`[{bad}]`)
	err := UpdateRvInfo(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRvInfo) {
		t.Fatalf("expected ErrInvalidRvInfo, got %v", err)
	}
}


