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

	fdo_lib "github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Manufacturing handles HTTP requests for the manufacturing server
type Manufacturing struct {
	DBState            *db.State
	RvInfoState        *state.RvInfoState
	HealthState        *state.HealthState
	DeviceKey          crypto.Signer
	DeviceCAChain      []*x509.Certificate
	MfgKey             crypto.Signer
	OwnerCert          *x509.Certificate
	EncodePublicKeyFn  func(protocol.KeyType, protocol.KeyEncoding, crypto.PublicKey, []*x509.Certificate) (*protocol.PublicKey, error)
}

// NewManufacturing creates a new Manufacturing handler
func NewManufacturing(dbState *db.State, deviceKey crypto.Signer, deviceCAChain []*x509.Certificate, mfgKey crypto.Signer, ownerCert *x509.Certificate, encodePublicKeyFn func(protocol.KeyType, protocol.KeyEncoding, crypto.PublicKey, []*x509.Certificate) (*protocol.PublicKey, error)) *Manufacturing {
	return &Manufacturing{
		DBState:            dbState,
		DeviceKey:          deviceKey,
		DeviceCAChain:      deviceCAChain,
		MfgKey:             mfgKey,
		OwnerCert:          ownerCert,
		EncodePublicKeyFn:  encodePublicKeyFn,
	}
}

// InitDB initializes the manufacturing database and state
func (m *Manufacturing) InitDB() error {
	// Initialize RvInfo state (needed by DI handler and V2 API)
	rvInfoState, err := state.InitRvInfoDB(m.DBState.DB)
	if err != nil {
		slog.Error("failed to initialize RvInfo state", "err", err)
		return err
	}
	m.RvInfoState = rvInfoState
	slog.Debug("RvInfo state initialized successfully")

	// Initialize Health state
	healthState, err := state.InitHealthDB(m.DBState.DB)
	if err != nil {
		slog.Error("failed to initialize health database", "err", err)
		return err
	}
	m.HealthState = healthState
	slog.Debug("Health state initialized successfully")

	return nil
}

// Handler returns a fully configured HTTP handler for the manufacturing server
func (m *Manufacturing) Handler() http.Handler {
	rootMux := http.NewServeMux()

	// Register FDO protocol handler (Device Initialization)
	fdoHandler := &fdo_http.Handler{
		Tokens: m.DBState,
		DIResponder: &fdo_lib.DIServer[custom.DeviceMfgInfo]{
			Session:               m.DBState,
			Vouchers:              m.DBState,
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
			RvInfo: func(ctx context.Context, _ *fdo_lib.Voucher) ([][]protocol.RvInstruction, error) {
				// Use unified rvinfo table (supports both V1 and V2 APIs)
				// Handles automatic migration from JSON to CBOR format
				return db.FetchRvInfo()
			},
		},
	}
	rootMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Register health handler
	healthServer := health.NewServer(m.HealthState)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, rootMux)

	// === V1 API (Old handlers for backward compatibility) ===
	mgmtAPIServeMuxV1 := http.NewServeMux()
	mgmtAPIServeMuxV1.HandleFunc("GET /vouchers", handlers.GetVoucherHandler)
	mgmtAPIServeMuxV1.HandleFunc("GET /vouchers/{guid}", handlers.GetVoucherByGUIDHandler)
	mgmtAPIServeMuxV1.Handle("/rvinfo", handlers.RvInfoHandler())
	mgmtAPIHandlerV1 := rateLimitMiddleware(rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, mgmtAPIServeMuxV1))
	rootMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))

	// === V2 API (New OpenAPI handlers) ===
	mgmtAPIServeMuxV2 := http.NewServeMux()
	rvInfoServer := rvinfo.NewServer(m.RvInfoState)
	rvInfoStrictHandler := rvinfo.NewStrictHandler(&rvInfoServer, nil)
	rvinfo.HandlerFromMux(rvInfoStrictHandler, mgmtAPIServeMuxV2)
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
