// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package to0

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/tls"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// to0Client is the minimal interface used from the TO0 client.
type to0Client interface {
	RegisterBlob(ctx context.Context, transport fdo.Transport, guid protocol.GUID, to2Addrs []protocol.RvTO2Addr) (uint32, error)
}

// Allow test-time injection of dependencies.
var (
	newTO0Client = func(vouchers fdo.OwnerVoucherPersistentState, keys fdo.OwnerKeyPersistentState) to0Client {
		return &fdo.TO0Client{Vouchers: vouchers, OwnerKeys: keys}
	}
	makeTransport  = tls.TlsTransport
	fetchOwnerInfo = db.FetchOwnerInfo
)

func RegisterRvBlob(rvInfo [][]protocol.RvInstruction, to0Guid string, voucherState fdo.OwnerVoucherPersistentState, keyState fdo.OwnerKeyPersistentState, insecureTLS bool) error { // Parse to0-guid flag
	guidBytes, err := hex.DecodeString(to0Guid)
	if err != nil {
		return fmt.Errorf("error parsing hex GUID of device to register RV blob: %w", err)
	}
	if len(guidBytes) != 16 {
		return fmt.Errorf("error parsing hex GUID of device to register RV blob: must be 16 bytes")
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Retrieve owner info from DB
	to2Addrs, err := fetchOwnerInfo()
	if err != nil {
		return fmt.Errorf("error fetching ownerinfo: %w", err)
	}

	ownerRvInfo := protocol.ParseOwnerRvInfo(rvInfo)
	if len(ownerRvInfo) == 0 {
		return fmt.Errorf("no RV info found that is usable for the owner")
	}

	for _, rv := range ownerRvInfo {
		if len(rv.URLs) == 0 {
			slog.Error("no usable rendezvous URLs were found for RV directive", "rv", rv)
			continue
		}
		for _, url := range rv.URLs {
			refresh, err := newTO0Client(voucherState, keyState).RegisterBlob(
				context.Background(), makeTransport(url.String(), nil, insecureTLS), guid, to2Addrs,
			)
			if err != nil {
				slog.Error("failed to", "connect", url.String())
				continue
			}
			slog.Info("to0 refresh", "duration", time.Duration(refresh)*time.Second)
			return nil
		}
	}
	return nil
}
