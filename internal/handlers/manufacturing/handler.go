// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package manufacturing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo"
	v1rvinfo "github.com/fido-device-onboard/go-fdo-server/api/v1/rvinfo"
	v1voucher "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/health"
	v2rvinfo "github.com/fido-device-onboard/go-fdo-server/internal/handlers/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Manufacturing handles HTTP requests for the manufacturing server
type Manufacturing struct {
	DB     *gorm.DB
	State  *state.ManufacturingState
	Config *config.ManufacturingServerConfig
}

// NewManufacturing creates a new Manufacturing handler
func NewManufacturing(db *gorm.DB, config *config.ManufacturingServerConfig) Manufacturing {
	return Manufacturing{DB: db, Config: config}
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

// encodePublicKey converts a public key to protocol format
func encodePublicKey(keyType protocol.KeyType, keyEncoding protocol.KeyEncoding, pub crypto.PublicKey, chain []*x509.Certificate) (*protocol.PublicKey, error) {
	if pub == nil && len(chain) > 0 {
		pub = chain[0].PublicKey
	}
	if pub == nil {
		return nil, fmt.Errorf("no key to encode")
	}

	switch keyEncoding {
	case protocol.X509KeyEnc, protocol.CoseKeyEnc:
		// Intentionally panic if pub is not the correct key type
		switch keyType {
		case protocol.Secp256r1KeyType, protocol.Secp384r1KeyType:
			return protocol.NewPublicKey(keyType, pub.(*ecdsa.PublicKey), keyEncoding == protocol.CoseKeyEnc)
		case protocol.Rsa2048RestrKeyType, protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
			return protocol.NewPublicKey(keyType, pub.(*rsa.PublicKey), keyEncoding == protocol.CoseKeyEnc)
		default:
			return nil, fmt.Errorf("unsupported key type: %s", keyType)
		}
	case protocol.X5ChainKeyEnc:
		return protocol.NewPublicKey(keyType, chain, false)
	default:
		return nil, fmt.Errorf("unsupported key encoding: %s", keyEncoding)
	}
}

// Handler returns a fully configured HTTP handler for the manufacturing server
func (m *Manufacturing) Handler() http.Handler {
	// Load keys and certificates
	mfgKey, err := m.Config.GetManufacturerKey()
	if err != nil {
		slog.Error("Failed to load manufacturer key", "err", err)
		panic(fmt.Sprintf("failed to load manufacturer key: %v", err))
	}

	deviceKey, err := m.Config.GetDeviceCAKey()
	if err != nil {
		slog.Error("Failed to load device CA key", "err", err)
		panic(fmt.Sprintf("failed to load device CA key: %v", err))
	}

	deviceCACerts, err := m.Config.GetDeviceCACerts()
	if err != nil {
		slog.Error("Failed to load device CA certificates", "err", err)
		panic(fmt.Sprintf("failed to load device CA certificates: %v", err))
	}

	ownerCert, err := m.Config.GetOwnerCertificate()
	if err != nil {
		slog.Error("Failed to load owner certificate", "err", err)
		panic(fmt.Sprintf("failed to load owner certificate: %v", err))
	}

	manufacturingServeMux := http.NewServeMux()

	// Wire FDO protocol handler
	fdoHandler := &fdo_http.Handler{
		Tokens: m.State.Token,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               m.State.DISession,
			Vouchers:              m.State.Voucher,
			SignDeviceCertificate: custom.SignDeviceCertificate(deviceKey, deviceCACerts),
			DeviceInfo: func(ctx context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.PublicKey, error) {
				mfgPubKey, err := encodePublicKey(info.KeyType, info.KeyEncoding, mfgKey.Public(), nil)
				if err != nil {
					return "", protocol.PublicKey{}, err
				}
				return info.DeviceInfo, *mfgPubKey, nil
			},
			BeforeVoucherPersist: func(ctx context.Context, ov *fdo.Voucher) error {
				extended, err := fdo.ExtendVoucher(ov, mfgKey, []*x509.Certificate{ownerCert}, nil)
				if err != nil {
					return err
				}
				*ov = *extended
				return nil
			},
			RvInfo: func(ctx context.Context, _ *fdo.Voucher) ([][]protocol.RvInstruction, error) {
				return m.State.RvInfo.GetRVInfo(ctx)
			},
		},
	}

	manufacturingServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Register health handler
	healthServer := health.NewServer(m.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, manufacturingServeMux)

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
	manufacturingServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandlerV1))

	// === V2 API (New OpenAPI handlers) ===
	mgmtAPIServeMuxV2 := http.NewServeMux()

	rvInfoServerV2 := v2rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandler := v2rvinfo.NewStrictHandler(&rvInfoServerV2, nil)
	v2rvinfo.HandlerFromMux(rvInfoStrictHandler, mgmtAPIServeMuxV2)

	// TODO: Add voucher V2 API handlers here following the same pattern (tracked in PR #193)
	mgmtAPIHandlerV2 := rateLimitMiddleware(rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, mgmtAPIServeMuxV2))
	manufacturingServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtAPIHandlerV2))

	return manufacturingServeMux
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
