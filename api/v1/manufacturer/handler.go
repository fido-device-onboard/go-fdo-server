// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package manufacturer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/health"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/custom"
	fdo_http "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	swagger_ui "github.com/swaggest/swgui/v5emb"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var openAPISpecJSON []byte

// Manufacturer handles FDO protocol HTTP requests for manufacturing
type Manufacturer struct {
	DB     *gorm.DB
	State  *state.ManufacturingState
	Config *config.ManufacturingServerConfig
}

// NewManufacturer creates a new Manufacturer instance
func NewManufacturer(db *gorm.DB, config *config.ManufacturingServerConfig) Manufacturer {
	return Manufacturer{DB: db, Config: config}
}

func (m *Manufacturer) InitDB() error {
	state, err := state.InitManufacturingDB(m.DB)
	if err != nil {
		return err
	}
	m.State = state
	return nil
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

func (m *Manufacturer) Handler() http.Handler {
	manufacturerServeMux := http.NewServeMux()

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
	manufacturerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health API
	healthServer := health.NewServer(m.State.Health)
	healthStrictHandler := health.NewStrictHandler(&healthServer, nil)
	health.HandlerFromMux(healthStrictHandler, manufacturerServeMux)

	// Wire Management APIs
	mgmtAPIServeMux := http.NewServeMux()

	// Wire Voucher API - manufacturer only reads vouchers, doesn't insert
	voucherServer := voucher.NewServer(m.State.Voucher, nil)
	voucherStrictHandler := voucher.NewStrictHandler(&voucherServer, nil)
	voucher.HandlerFromMux(voucherStrictHandler, mgmtAPIServeMux)

	// Wire RvInfo API
	rvInfoServer := rvinfo.NewServer(m.State.RvInfo)
	rvInfoStrictHandler := rvinfo.NewStrictHandler(&rvInfoServer, nil)
	rvinfo.HandlerFromMux(rvInfoStrictHandler, mgmtAPIServeMux)

	mgmtAPIHandler := rateLimitMiddleware(
		rate.NewLimiter(2, 10),
		bodySizeMiddleware(1<<20, // 1MB
			mgmtAPIServeMux,
		),
	)

	// Serve OpenAPI specification
	mgmtAPIServeMux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(openAPISpecJSON)
	})

	// Serve Swagger UI documentation
	mgmtAPIServeMux.Handle("GET /api/docs", swagger_ui.New(
		"Manufacturer",
		"/api/openapi.json",
		"/api/docs/"))

	manufacturerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtAPIHandler))

	return manufacturerServeMux
}
