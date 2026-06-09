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

	"github.com/elnormous/contenttype"
	"github.com/fido-device-onboard/go-fdo"
	v1rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v1/rvinfo"
	v1voucher "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/api/v2/health"
	v2rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v2/rvinfo"
	v2voucher "github.com/fido-device-onboard/go-fdo-server/api/v2/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
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
	MfgKeyType    protocol.KeyType
	DeviceKey     crypto.Signer
	DeviceCACerts []*x509.Certificate
	OwnerCert     *x509.Certificate
}

// NewManufacturer creates a new Manufacturer handler
func NewManufacturer(db *gorm.DB, mfgKey crypto.Signer, mfgKeyType protocol.KeyType, deviceKey crypto.Signer, deviceCACerts []*x509.Certificate, ownerCert *x509.Certificate) Manufacturer {
	return Manufacturer{
		DB:            db,
		MfgKey:        mfgKey,
		MfgKeyType:    mfgKeyType,
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
	diServer := &fdo.DIServer[custom.DeviceMfgInfo]{
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
		RvInfo: func(ctx context.Context, _ *fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return m.State.RvInfo.GetRvInfo(ctx)
		},
	}

	if m.OwnerCert != nil {
		diServer.BeforeVoucherPersist = func(ctx context.Context, ov *fdo.Voucher) error {
			extended, err := fdo.ExtendVoucher(ov, m.MfgKey, []*x509.Certificate{m.OwnerCert}, nil)
			if err != nil {
				return err
			}
			*ov = *extended
			return nil
		}
		// No AfterVoucherPersist needed: the ownership_verified column defaults
		// to false for new rows, which is correct — the manufacturer no longer
		// owns the voucher after extending it to the owner certificate.
	}

	fdoHandler := &fdo_http.Handler{
		Tokens:      m.State.Token,
		DIResponder: diServer,
	}
	manufacturerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Register health handler
	healthServer := health.NewServer(m.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, manufacturerServeMux)

	// === V1 API (Old handlers for backward compatibility) ===
	mgmtAPIServeMuxV1 := http.NewServeMux()

	var ownerPKeys []crypto.PublicKey
	if m.OwnerCert != nil {
		ownerPKeys = []crypto.PublicKey{m.OwnerCert.PublicKey}
	}
	voucherServerV1 := v1voucher.NewServer(m.State.Voucher, ownerPKeys)
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

	// Wire Voucher API with content negotiation middleware.
	mfgOwnerKeyState := state.NewOwnerKeyPersistentState(m.MfgKey, m.MfgKeyType, nil)
	voucherServerV2 := v2voucher.NewServer(m.State.Voucher, m.State.DeviceCA, mfgOwnerKeyState)
	voucherMiddlewaresV2 := []v2voucher.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware[v2voucher.StrictHandlerFunc](
			[]contenttype.MediaType{
				contenttype.NewMediaType("application/json"),
				contenttype.NewMediaType("application/x-pem-file"),
			},
			"application/json",
		),
	}
	voucherStrictHandlerV2 := v2voucher.NewStrictHandler(&voucherServerV2, voucherMiddlewaresV2)
	v2voucher.HandlerFromMux(voucherStrictHandlerV2, mgmtAPIServeMuxV2)

	validationMiddleware := middleware.OpenAPIValidationMiddleware(openAPISpecJSON)
	mgmtAPIHandlerV2 := middleware.RateLimitMiddleware(rate.NewLimiter(2, 10),
		middleware.BodySizeMiddleware(10<<20, validationMiddleware(mgmtAPIServeMuxV2)))

	middleware.ServeOpenAPI(manufacturerServeMux, "Manufacturer", openAPISpecJSON)

	manufacturerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))
	manufacturerServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return manufacturerServeMux
}
