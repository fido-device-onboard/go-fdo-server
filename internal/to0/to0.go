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
	newTO0Client = func(vouchers fdo.OwnerVoucherPersistentState, keys fdo.OwnerKeyPersistentState, defaultTTL uint32) to0Client {
		return &fdo.TO0Client{Vouchers: vouchers, OwnerKeys: keys, TTL: defaultTTL}
	}
	makeTransport  = tls.TlsTransport
	fetchOwnerInfo = db.FetchOwnerInfo
)

func RegisterRvBlob(rvInfo [][]protocol.RvInstruction, to0Guid string, voucherState fdo.OwnerVoucherPersistentState, keyState fdo.OwnerKeyPersistentState, insecureTLS bool, defaultTTL uint32) (uint32, error) { // Parse to0-guid flag
	guidBytes, err := hex.DecodeString(to0Guid)
	if err != nil {
		return 0, fmt.Errorf("error parsing hex GUID of device to register RV blob: %w", err)
	}
	if len(guidBytes) != 16 {
		return 0, fmt.Errorf("error parsing hex GUID of device to register RV blob: must be 16 bytes")
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Retrieve owner info from DB
	to2Addrs, err := fetchOwnerInfo()
	if err != nil {
		return 0, fmt.Errorf("error fetching ownerinfo: %w", err)
	}

	ownerRvInfo := protocol.ParseOwnerRvInfo(rvInfo)
	if len(ownerRvInfo) == 0 {
		return 0, fmt.Errorf("no RV info found that is usable for the owner")
	}

	// TODO: This bypass handling should be moved to protocol.ParseOwnerRvInfo() in go-fdo library.
	// Per FIDO 1.1 spec Table 3.6, RVBypass applies to Device only and shouldn't be returned
	// as a directive to the Owner server. See: https://github.com/fido-device-onboard/go-fdo-server/issues/166
	// Track if all directives are bypass
	allBypass := true
	for _, rv := range ownerRvInfo {
		// Skip RV bypass directives - device connects directly to Owner, no TO0 needed
		if rv.Bypass {
			slog.Debug("skipping TO0 registration for RV bypass directive")
			continue
		}
		allBypass = false
		if len(rv.URLs) == 0 {
			slog.Error("no usable rendezvous URLs were found for RV directive", "rv", rv)
			continue
		}
		for _, url := range rv.URLs {
			refresh, err := newTO0Client(voucherState, keyState, defaultTTL).RegisterBlob(
				context.Background(), makeTransport(url.String(), nil, insecureTLS), guid, to2Addrs,
			)
			if err != nil {
				slog.Error("failed registering 'RVTO2Addr' to rendezvous server", "url", url.String(), "error", err)
				continue
			}
			slog.Info("successfully registered 'RVTO2Addr' to rendezvous server", "url", url.String(), "duration", time.Duration(refresh)*time.Second)
			return refresh, nil
		}
	}

	// If all directives were bypass, that's success (no registration needed)
	if allBypass {
		slog.Debug("all RV directives are bypass - no TO0 registration needed")
		return 0, nil
	}

	// Had non-bypass directives but all failed
	return 0, fmt.Errorf("unable to register any 'RVTO2Addr' URL for guid='%s'", guid)
}
