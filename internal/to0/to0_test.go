package to0

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

type countingClient struct {
	calls     int
	succeedOn int
}

func (c *countingClient) RegisterBlob(ctx context.Context, transport fdo.Transport, guid protocol.GUID, to2Addrs []protocol.RvTO2Addr) (uint32, error) {
	c.calls++
	if c.calls == c.succeedOn {
		return 123, nil
	}
	return 0, errors.New("fail")
}

type mockRVTO2AddrState struct {
	addrs []protocol.RvTO2Addr
	err   error
}

func (m *mockRVTO2AddrState) Get(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	return m.addrs, m.err
}

func (m *mockRVTO2AddrState) Update(ctx context.Context, addrs []protocol.RvTO2Addr) error {
	return nil
}

func (m *mockRVTO2AddrState) Delete(ctx context.Context) ([]protocol.RvTO2Addr, error) {
	return m.addrs, nil
}

func TestRegisterRvBlob_BreaksAfterFirstSuccess(t *testing.T) {
	// Arrange: create rvInfo with three RV directives (3 potential attempts).
	dns1, _ := cbor.Marshal("a1.example.com")
	dns2, _ := cbor.Marshal("a2.example.com")
	dns3, _ := cbor.Marshal("b1.example.com")
	protHTTP, _ := cbor.Marshal(uint8(protocol.RVProtHTTP))
	rvInfo := [][]protocol.RvInstruction{
		{
			{Variable: protocol.RVDns, Value: dns1},
			{Variable: protocol.RVProtocol, Value: protHTTP},
		},
		{
			{Variable: protocol.RVDns, Value: dns3},
			{Variable: protocol.RVProtocol, Value: protHTTP},
		},
		{
			{Variable: protocol.RVDns, Value: dns2},
			{Variable: protocol.RVProtocol, Value: protHTTP},
		},
	}

	// Create mock RVTO2AddrState
	mockRVTO2Addr := &mockRVTO2AddrState{
		addrs: []protocol.RvTO2Addr{{}},
	}

	// Inject fake transport maker to avoid real URLs
	oldMakeTransport := makeTransport
	makeTransport = func(baseURL string, _ *tls.Config, _ bool) fdo.Transport { return nil }
	defer func() { makeTransport = oldMakeTransport }()

	// Inject counting client
	// Succeed on the 2nd overall attempt to ensure we stop then and do not try the 3rd.
	cc := &countingClient{succeedOn: 2}
	oldNew := newTO0Client
	newTO0Client = func(v fdo.OwnerVoucherPersistentState, k fdo.OwnerKeyPersistentState, defaultTTL uint32) to0Client {
		return cc
	}
	defer func() { newTO0Client = oldNew }()

	// Act
	refresh, err := RegisterRvBlob(rvInfo, "00112233445566778899aabbccddeeff", nil, nil, mockRVTO2Addr, false, 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refresh == 0 {
		t.Fatalf("expected non-zero refresh on success")
	}

	// Assert: calls should be exactly 2 (stop after first success despite 3 potential)
	if cc.calls != 2 {
		t.Fatalf("expected 2 RegisterBlob calls, got %d", cc.calls)
	}
}
