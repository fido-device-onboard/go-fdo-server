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
	"github.com/fido-device-onboard/go-fdo-server/internal/ownerinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/tls"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

func RegisterRvBlob(rvInfo [][]protocol.RvInstruction, to0Guid string, voucherState fdo.OwnerVoucherPersistentState, keyState fdo.OwnerKeyPersistentState, useTLS bool) error { // Parse to0-guid flag
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
	to2Addrs, err := ownerinfo.FetchOwnerInfo()
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
			refresh, err := (&fdo.TO0Client{
				Vouchers:  voucherState,
				OwnerKeys: keyState,
			}).RegisterBlob(context.Background(), tls.TlsTransport(url.String(), nil, useTLS), guid, to2Addrs)
			if err != nil {
				slog.Error("failed to", "connect", url.String())
				continue
			}
			slog.Info("to0 refresh", "duration", time.Duration(refresh)*time.Second)
			break
		}
	}
	return nil
}
