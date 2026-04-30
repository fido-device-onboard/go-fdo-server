// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package manufacturer

import (
	"context"
	"crypto"
	"crypto/x509"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo"
	v1rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v1/rvinfo"
	v1voucher "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/api/v2/health"
	v2rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v2/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	swaggerui "github.com/swaggest/swgui/v5emb"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var openAPISpecJSON []byte

// Manufacturer handles HTTP requests for the manufacturer server
type Manufacturer struct {
	DB            *gorm.DB
	State         *state.ManufacturingState
	MfgKey        crypto.Signer
	DeviceKey     crypto.Signer
	DeviceCACerts []*x509.Certificate
	OwnerCert     *x509.Certificate
}

// NewManufacturer creates a new Manufacturer handler
func NewManufacturer(db *gorm.DB, mfgKey crypto.Signer, deviceKey crypto.Signer, deviceCACerts []*x509.Certificate, ownerCert *x509.Certificate) Manufacturer {
	return Manufacturer{
		DB:            db,
		MfgKey:        mfgKey,
		DeviceKey:     deviceKey,
		DeviceCACerts: deviceCACerts,
		OwnerCert:     ownerCert,
	}
}

// InitDB initializes the manufacturing database and state, and runs any
// pending one-time data migrations.
func (m *Manufacturer) InitDB() error {
	state, err := state.InitManufacturingDB(m.DB)
	if err != nil {
		return err
	}
	m.State = state

	rvInfoServer := v1rvinfo.NewServer(m.State.RvInfo)
	if err := rvInfoServer.MigrateJSONToCBOR(context.Background()); err != nil {
		return fmt.Errorf("failed to migrate rvinfo from JSON to CBOR: %w", err)
	}

	slog.Debug("Manufacturer DB initialized successfully")
	return nil
}

// Handler returns a fully configured HTTP handler for the manufacturer server
func (m *Manufacturer) Handler() http.Handler {
	manufacturerServeMux := http.NewServeMux()

	// Wire FDO protocol handler
	fdoHandler := &fdo_http.Handler{
		Tokens: m.State.Token,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               m.State.DISession,
			Vouchers:              m.State.Voucher,
			SignDeviceCertificate: custom.SignDeviceCertificate(m.DeviceKey, m.DeviceCACerts),
			DeviceInfo: func(ctx context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.PublicKey, error) {
				mfgPubKey, err := utils.EncodePublicKey(info.KeyType, info.KeyEncoding, m.MfgKey.Public(), nil)
				if err != nil {
					return "", protocol.PublicKey{}, err
				}
				return info.DeviceInfo, *mfgPubKey, nil
			},
			BeforeVoucherPersist: func(ctx context.Context, ov *fdo.Voucher) error {
				extended, err := fdo.ExtendVoucher(ov, m.MfgKey, []*x509.Certificate{m.OwnerCert}, nil)
				if err != nil {
					return err
				}
				*ov = *extended
				return nil
			},
			RvInfo: func(ctx context.Context, _ *fdo.Voucher) ([][]protocol.RvInstruction, error) {
				return m.State.RvInfo.GetRvInfo(ctx)
			},
		},
	}
	manufacturerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Register health handler
	healthServer := health.NewServer(m.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, manufacturerServeMux)

	// === V1 API (Old handlers for backward compatibility) ===
	mgmtAPIServeMuxV1 := http.NewServeMux()

	voucherServerV1 := v1voucher.NewServer(m.State.Voucher, []crypto.PublicKey{m.OwnerCert.PublicKey})
	voucherStrictHandlerV1 := v1voucher.NewStrictHandler(&voucherServerV1, nil)
	v1voucher.HandlerFromMux(voucherStrictHandlerV1, mgmtAPIServeMuxV1)

	rvInfoServerV1 := v1rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandlerV1 := v1rvinfo.NewStrictHandler(&rvInfoServerV1, nil)
	v1rvinfo.HandlerFromMux(rvInfoStrictHandlerV1, mgmtAPIServeMuxV1)

	mgmtAPIHandlerV1 := middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(10<<20, mgmtAPIServeMuxV1))

	// === V2 API (New OpenAPI handlers) ===
	mgmtAPIServeMuxV2 := http.NewServeMux()

	rvInfoServerV2 := v2rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandlerV2 := v2rvinfo.NewStrictHandler(&rvInfoServerV2, nil)
	v2rvinfo.HandlerFromMux(rvInfoStrictHandlerV2, mgmtAPIServeMuxV2)

	// TODO: Add voucher V2 API handlers here following the same pattern (tracked in PR #193)

	mgmtAPIHandlerV2 := middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(10<<20, mgmtAPIServeMuxV2))

	// Serve OpenAPI specification
	manufacturerServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if _, err := w.Write(openAPISpecJSON); err != nil {
			slog.Error("Failed to write OpenAPI spec response", "error", err)
		}
	})

	// Serve Swagger UI documentation
	manufacturerServeMux.Handle("GET /api/docs/", swaggerui.New(
		"Manufacturer",
		"/api/openapi.json",
		"/api/docs/"))

	manufacturerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))
	manufacturerServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return manufacturerServeMux
}
