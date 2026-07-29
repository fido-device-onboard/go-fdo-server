// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package owner

import (
	"context"
	"crypto"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/elnormous/contenttype"
	"github.com/fido-device-onboard/go-fdo"
	v1device "github.com/fido-device-onboard/go-fdo-server/api/v1/device"
	v1deviceca "github.com/fido-device-onboard/go-fdo-server/api/v1/deviceca"
	v1ownerinfo "github.com/fido-device-onboard/go-fdo-server/api/v1/ownerinfo"
	v1resell "github.com/fido-device-onboard/go-fdo-server/api/v1/resell"
	v1voucher "github.com/fido-device-onboard/go-fdo-server/api/v1/voucher"
	v2device "github.com/fido-device-onboard/go-fdo-server/api/v2/device"
	v2deviceca "github.com/fido-device-onboard/go-fdo-server/api/v2/deviceca"
	v2health "github.com/fido-device-onboard/go-fdo-server/api/v2/health"
	v2rvto2addr "github.com/fido-device-onboard/go-fdo-server/api/v2/rvto2addr"
	v2voucher "github.com/fido-device-onboard/go-fdo-server/api/v2/voucher"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/serviceinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	fdohttp "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Embedded OpenAPI specification
//
//go:embed openapi.json
var OpenAPISpecJSON []byte

// Owner handles FDO protocol HTTP requests
type Owner struct {
	DB                 *gorm.DB
	State              *state.OwnerState
	ReuseCred          bool
	ServiceInfoModules *serviceinfo.ModuleStateMachines
}

// NewOwner creates a new Owner instance
func NewOwner(
	db *gorm.DB,
	reuseCreds bool,
) Owner {
	return Owner{
		DB:        db,
		ReuseCred: reuseCreds,
	}
}

func (o *Owner) InitDB() error {
	state, err := state.InitOwnerDB(o.DB)
	if err != nil {
		return err
	}
	o.State = state

	ownerInfoServer := v1ownerinfo.NewServer(state.RVTO2Addr)
	if err := ownerInfoServer.MigrateOwnerInfo(context.Background()); err != nil {
		return fmt.Errorf("failed to migrate owner_info: %w", err)
	}

	return nil
}

func (o *Owner) Handler() http.Handler {
	ownerServeMux := http.NewServeMux()

	to2Server := &fdo.TO2Server{
		Session:              o.State.TO2Session,
		Modules:              o.ServiceInfoModules,
		Vouchers:             o.State.Voucher,
		VouchersForExtension: o.State.Voucher,
		OwnerKeys:            o.State.OwnerKey,
		RvInfo: func(ctx context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return o.ReuseCred, nil },
		VerifyVoucher: func(ctx context.Context, voucher fdo.Voucher) error {
			return VerifyVoucher(ctx, voucher, o.State, o.ReuseCred)
		},
	}

	deviceCACertContentTypes := []contenttype.MediaType{
		contenttype.NewMediaType("application/json"),
		contenttype.NewMediaType("application/x-pem-file"),
	}

	deviceCACertDefaultContentType := "application/json"

	// Wire FDO owner handler
	fdoHandler := &fdohttp.Handler{
		Tokens:       o.State.Token,
		TO2Responder: to2Server,
	}
	ownerServeMux.Handle("POST /fdo/101/msg/{msg}", fdoHandler)

	// Wire Health API
	healthServer := v2health.NewServer(o.State.Health)
	healthStrictHandler := v2health.NewStrictHandler(&healthServer, nil)
	v2health.HandlerFromMux(healthStrictHandler, ownerServeMux)

	// Wire mgmt APIs
	mgmtAPIServeMuxV1 := http.NewServeMux()

	// Wire Device API
	deviceServerV1 := v1device.NewServer(o.State.Voucher)
	deviceStrictHandlerV1 := v1device.NewStrictHandler(&deviceServerV1, nil)
	v1device.HandlerFromMux(deviceStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire the Device CA API
	deviceCAServerV1 := v1deviceca.NewServer(o.State.DeviceCA)
	deviceCAMiddlewaresV1 := []v1deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware[v1deviceca.StrictHandlerFunc](deviceCACertContentTypes, deviceCACertDefaultContentType),
	}
	deviceCAStrictHandlerV1 := v1deviceca.NewStrictHandler(&deviceCAServerV1, deviceCAMiddlewaresV1)
	v1deviceca.HandlerFromMux(deviceCAStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire RVTO2 Address API
	ownerinfoServerV1 := v1ownerinfo.NewServer(o.State.RVTO2Addr)
	ownerinfoStrictHandlerV1 := v1ownerinfo.NewStrictHandler(&ownerinfoServerV1, nil)
	v1ownerinfo.HandlerFromMux(ownerinfoStrictHandlerV1, mgmtAPIServeMuxV1)

	// Wire Resell API
	resellServerV1 := v1resell.NewServer(o.State.Voucher, o.State.OwnerKey)
	resellStrictHandlerV1 := v1resell.NewStrictHandler(&resellServerV1, nil)
	v1resell.HandlerWithOptions(resellStrictHandlerV1, v1resell.StdHTTPServerOptions{BaseRouter: mgmtAPIServeMuxV1, BaseURL: "/owner"})

	// Wire Voucher API
	voucherServerV1 := v1voucher.NewServer(o.State.Voucher, []crypto.PublicKey{o.State.OwnerKey.Signer().Public()})
	voucherStrictHandlerV1 := v1voucher.NewStrictHandler(&voucherServerV1, nil)
	v1voucher.HandlerWithOptions(voucherStrictHandlerV1, v1voucher.StdHTTPServerOptions{BaseRouter: mgmtAPIServeMuxV1, BaseURL: "/owner"})

	mgmtHandlerV1 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10), // 2 req/s, burst of 10
		middleware.BodySizeMiddleware(10<<20, /* 10MB */
			mgmtAPIServeMuxV1,
		),
	)

	// Wire mgmt APIs
	mgmtAPIServeMuxV2 := http.NewServeMux()

	// Wire Device API
	deviceServerV2 := v2device.NewServer(o.State.Voucher)
	deviceStrictHandlerV2 := v2device.NewStrictHandler(&deviceServerV2, nil)
	v2device.HandlerFromMux(deviceStrictHandlerV2, mgmtAPIServeMuxV2)

	// Wire Device CA API with content negotiation middleware
	deviceCAServerV2 := v2deviceca.NewServer(o.State.DeviceCA)
	deviceCAMiddlewaresV2 := []v2deviceca.StrictMiddlewareFunc{
		middleware.ContentNegotiationMiddleware[v2deviceca.StrictHandlerFunc](deviceCACertContentTypes, deviceCACertDefaultContentType),
	}
	deviceCAStrictHandlerV2 := v2deviceca.NewStrictHandler(&deviceCAServerV2, deviceCAMiddlewaresV2)
	v2deviceca.HandlerFromMux(deviceCAStrictHandlerV2, mgmtAPIServeMuxV2)

	// Wire RVTO2 Address API
	rvto2addrServerV2 := v2rvto2addr.NewServer(o.State.RVTO2Addr)
	rvto2addrStrictHandlerV2 := v2rvto2addr.NewStrictHandler(&rvto2addrServerV2, nil)
	v2rvto2addr.HandlerFromMux(rvto2addrStrictHandlerV2, mgmtAPIServeMuxV2)

	// Wire Voucher API with content negotiation middleware
	voucherServerV2 := v2voucher.NewServer(o.State.Voucher, o.State.DeviceCA, o.State.OwnerKey)
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

	validationMiddleware := middleware.OpenAPIValidationMiddleware(OpenAPISpecJSON)
	mgmtHandlerV2 := middleware.RateLimitMiddleware(
		rate.NewLimiter(2, 10), // 2 req/s, burst of 10
		middleware.BodySizeMiddleware(10<<20, /* 10MB */
			validationMiddleware(mgmtAPIServeMuxV2),
		),
	)

	middleware.ServeOpenAPI(ownerServeMux, "Owner", OpenAPISpecJSON)

	ownerServeMux.Handle("/api/v1/", http.StripPrefix("/api/v1", mgmtHandlerV1))
	ownerServeMux.Handle("/api/v2/", http.StripPrefix("/api/v2", mgmtHandlerV2))

	return ownerServeMux
}
