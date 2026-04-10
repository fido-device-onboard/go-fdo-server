// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package manufacturing

import (
	"context"
	"crypto"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	fdo_lib "github.com/fido-device-onboard/go-fdo"
	v1rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v1/rvinfo"
	v1voucher "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	v2rvinfo "github.com/fido-device-onboard/go-fdo-server/internal/handlers/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Manufacturing handles HTTP requests for the manufacturing server
type Manufacturing struct {
	DB                *gorm.DB
	State             *state.ManufacturingState
	DeviceKey         crypto.Signer
	DeviceCAChain     []*x509.Certificate
	MfgKey            crypto.Signer
	OwnerCert         *x509.Certificate
	EncodePublicKeyFn func(protocol.KeyType, protocol.KeyEncoding, crypto.PublicKey, []*x509.Certificate) (*protocol.PublicKey, error)
}

// NewManufacturing creates a new Manufacturing handler
func NewManufacturing(db *gorm.DB, deviceKey crypto.Signer, deviceCAChain []*x509.Certificate, mfgKey crypto.Signer, ownerCert *x509.Certificate, encodePublicKeyFn func(protocol.KeyType, protocol.KeyEncoding, crypto.PublicKey, []*x509.Certificate) (*protocol.PublicKey, error)) *Manufacturing {
	return &Manufacturing{
		DB:                db,
		DeviceKey:         deviceKey,
		DeviceCAChain:     deviceCAChain,
		MfgKey:            mfgKey,
		OwnerCert:         ownerCert,
		EncodePublicKeyFn: encodePublicKeyFn,
	}
}

// InitDB initializes the manufacturing database and state
func (m *Manufacturing) InitDB() error {
	state, err := state.InitManufacturingDB(m.DB)
	if err != nil {
		return err
	}
	m.State = state
	slog.Debug("Manufacturer DB initialized successfully")
	return nil
}

// Handler returns a fully configured HTTP handler for the manufacturing server
func (m *Manufacturing) Handler() http.Handler {
	rootMux := http.NewServeMux()

	// Register FDO protocol handler (Device Initialization)
	fdoHandler := &fdo_http.Handler{
		Tokens: m.State.Token,
		DIResponder: &fdo_lib.DIServer[custom.DeviceMfgInfo]{
			Session:               m.State.DISession,
			Vouchers:              m.State.Voucher,
			SignDeviceCertificate: custom.SignDeviceCertificate(m.DeviceKey, m.DeviceCAChain),
			DeviceInfo: func(ctx context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.PublicKey, error) {
				// TODO: Parse manufacturer key chain (different than device CA chain)
				mfgPubKey, err := m.EncodePublicKeyFn(info.KeyType, info.KeyEncoding, m.MfgKey.Public(), nil)
				if err != nil {
					return "", protocol.PublicKey{}, err
				}
				return info.DeviceInfo, *mfgPubKey, nil
			},
			BeforeVoucherPersist: func(ctx context.Context, ov *fdo_lib.Voucher) error {
				extended, err := fdo_lib.ExtendVoucher(ov, m.MfgKey, []*x509.Certificate{m.OwnerCert}, nil)
				if err != nil {
					return err
				}
				*ov = *extended
				return nil
			},
			RvInfo: func(ctx context.Context, voucher *fdo_lib.Voucher) ([][]protocol.RvInstruction, error) {
				return m.State.RvInfo.GetRVInfo(ctx)
			},
		},
	}
	rootMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Register health handler
	healthServer := health.NewServer(m.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rootMux)

	// === V1 API (Old handlers for backward compatibility) ===
	mgmtAPIServeMuxV1 := http.NewServeMux()
	voucherServerV1 := v1voucher.NewServer(m.State.Voucher, nil)
	voucherStrictHandler := v1voucher.NewStrictHandler(&voucherServerV1, nil)
	v1voucher.HandlerFromMux(voucherStrictHandler, mgmtAPIServeMuxV1)
	rvInfoServerV1 := v1rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandlerV1 := v1rvinfo.NewStrictHandler(&rvInfoServerV1, nil)
	v1rvinfo.HandlerFromMux(rvInfoStrictHandlerV1, mgmtAPIServeMuxV1)

	mgmtAPIHandlerV1 := rateLimitMiddleware(rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, mgmtAPIServeMuxV1))
	rootMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))

	// === V2 API (New OpenAPI handlers) ===
	mgmtAPIServeMuxV2 := http.NewServeMux()
	rvInfoServerV2 := v2rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandler := v2rvinfo.NewStrictHandler(&rvInfoServerV2, nil)
	v2rvinfo.HandlerFromMux(rvInfoStrictHandler, mgmtAPIServeMuxV2)
	// TODO: Add voucher V2 API handlers here following the same pattern (tracked in PR #193)
	mgmtAPIHandlerV2 := rateLimitMiddleware(rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, mgmtAPIServeMuxV2))
	rootMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return rootMux
}

func rateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func bodySizeMiddleware(limitBytes int64, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.LimitReader(r.Body, limitBytes),
			Closer: r.Body,
		}
		next.ServeHTTP(w, r)
	}
}
