// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvto2addr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/fido-device-onboard/go-fdo-server/api/v2/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Server implements the StrictServerInterface for RVTO2Addr management
type Server struct {
	RVTO2AddrState *state.RVTO2AddrState
}

func NewServer(state *state.RVTO2AddrState) Server {
	return Server{RVTO2AddrState: state}
}

var _ StrictServerInterface = (*Server)(nil)

// GetRVTO2Addr retrieves the current RVTO2 address configuration
func (s *Server) GetRVTO2Addr(ctx context.Context, request GetRVTO2AddrRequestObject) (GetRVTO2AddrResponseObject, error) {
	protocolAddrs, err := s.RVTO2AddrState.Get(ctx)
	if err != nil {
		slog.Error("Failed to get RVTO2Addr", "error", err)
		return GetRVTO2Addr500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to retrieve RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert to API types
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		entry, err := protocolToAPIAddr(addr)
		if err != nil {
			slog.Error("Failed to convert RVTO2Addr entry", "error", err)
			return GetRVTO2Addr500JSONResponse{
				InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
					Message: "Failed to convert RVTO2Addr configuration",
				},
			}, nil
		}
		apiAddrs[i] = entry
	}

	return GetRVTO2Addr200JSONResponse(apiAddrs), nil
}

// UpdateRVTO2Addr updates the RVTO2 address configuration
func (s *Server) UpdateRVTO2Addr(ctx context.Context, request UpdateRVTO2AddrRequestObject) (UpdateRVTO2AddrResponseObject, error) {
	if request.Body == nil {
		return UpdateRVTO2Addr400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: "Request body is required",
			},
		}, nil
	}

	// Convert API types to protocol types
	protocolAddrs := make([]protocol.RvTO2Addr, len(*request.Body))
	for i, addr := range *request.Body {
		var err error
		protocolAddrs[i], err = apiToProtocolAddr(addr)
		if err != nil {
			return UpdateRVTO2Addr400JSONResponse{
				BadRequestJSONResponse: components.BadRequestJSONResponse{
					Message: fmt.Sprintf("Invalid address at index %d: %s", i, err.Error()),
				},
			}, nil
		}
	}

	err := s.RVTO2AddrState.Upsert(ctx, protocolAddrs)
	if err != nil {
		if errors.Is(err, state.ErrInvalidRVTO2Addr) {
			return UpdateRVTO2Addr400JSONResponse{
				BadRequestJSONResponse: components.BadRequestJSONResponse{
					Message: err.Error(),
				},
			}, nil
		}
		slog.Error("Failed to update RVTO2Addr", "error", err)
		return UpdateRVTO2Addr500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to update RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert back to API types for response
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		entry, err := protocolToAPIAddr(addr)
		if err != nil {
			slog.Error("Failed to convert RVTO2Addr entry", "error", err)
			return UpdateRVTO2Addr500JSONResponse{
				InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
					Message: "Failed to convert RVTO2Addr configuration",
				},
			}, nil
		}
		apiAddrs[i] = entry
	}

	return UpdateRVTO2Addr200JSONResponse(apiAddrs), nil
}

// DeleteRVTO2Addr deletes the RVTO2 address configuration
func (s *Server) DeleteRVTO2Addr(ctx context.Context, request DeleteRVTO2AddrRequestObject) (DeleteRVTO2AddrResponseObject, error) {
	protocolAddrs, err := s.RVTO2AddrState.Delete(ctx)
	if err != nil {
		slog.Error("Failed to delete RVTO2Addr", "error", err)
		return DeleteRVTO2Addr500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to delete RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert to API types
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		entry, err := protocolToAPIAddr(addr)
		if err != nil {
			slog.Error("Failed to convert RVTO2Addr entry", "error", err)
			return DeleteRVTO2Addr500JSONResponse{
				InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
					Message: "Failed to convert RVTO2Addr configuration",
				},
			}, nil
		}
		apiAddrs[i] = entry
	}

	return DeleteRVTO2Addr200JSONResponse(apiAddrs), nil
}

// protocolToAPIAddr converts a protocol.RvTO2Addr to an API RVTO2AddrEntry
func protocolToAPIAddr(addr protocol.RvTO2Addr) (RVTO2AddrEntry, error) {
	var dns *components.DNSHostname
	if addr.DNSAddress != nil {
		dns = addr.DNSAddress
	}

	var ip *components.IPv4Address
	if addr.IPAddress != nil {
		ipStr := addr.IPAddress.String()
		ip = &ipStr
	}

	tp, err := transportToAPIProtocol(addr.TransportProtocol)
	if err != nil {
		return RVTO2AddrEntry{}, err
	}

	return RVTO2AddrEntry{
		Dns:      dns,
		Ip:       ip,
		Port:     components.PortNumber(addr.Port),
		Protocol: tp,
	}, nil
}

// apiToProtocolAddr converts an API RVTO2AddrEntry to a protocol.RvTO2Addr
func apiToProtocolAddr(addr RVTO2AddrEntry) (protocol.RvTO2Addr, error) {
	// Validate that at least one of dns or ip is specified
	if (addr.Dns == nil || *addr.Dns == "") && (addr.Ip == nil || *addr.Ip == "") {
		return protocol.RvTO2Addr{}, fmt.Errorf("at least one of dns or ip must be specified")
	}

	var ipAddr *net.IP
	if addr.Ip != nil && *addr.Ip != "" {
		parsed := net.ParseIP(*addr.Ip)
		if parsed == nil {
			return protocol.RvTO2Addr{}, fmt.Errorf("invalid IP address: %s", *addr.Ip)
		}
		ipAddr = &parsed
	}

	transportProto, err := apiToTransportProtocol(addr.Protocol)
	if err != nil {
		return protocol.RvTO2Addr{}, err
	}

	return protocol.RvTO2Addr{
		IPAddress:         ipAddr,
		DNSAddress:        addr.Dns,
		Port:              uint16(addr.Port),
		TransportProtocol: transportProto,
	}, nil
}

// transportToAPIProtocol converts a protocol.TransportProtocol to TransportProtocol
func transportToAPIProtocol(tp protocol.TransportProtocol) (TransportProtocol, error) {
	switch tp {
	case protocol.TCPTransport:
		return Tcp, nil
	case protocol.TLSTransport:
		return Tls, nil
	case protocol.HTTPTransport:
		return Http, nil
	case protocol.CoAPTransport:
		return Coap, nil
	case protocol.HTTPSTransport:
		return Https, nil
	case protocol.CoAPSTransport:
		return Coaps, nil
	default:
		return "", fmt.Errorf("unsupported transport protocol: %d", tp)
	}
}

// apiToTransportProtocol converts a TransportProtocol to protocol.TransportProtocol
func apiToTransportProtocol(pt TransportProtocol) (protocol.TransportProtocol, error) {
	switch pt {
	case Tcp:
		return protocol.TCPTransport, nil
	case Tls:
		return protocol.TLSTransport, nil
	case Http:
		return protocol.HTTPTransport, nil
	case Coap:
		return protocol.CoAPTransport, nil
	case Https:
		return protocol.HTTPSTransport, nil
	case Coaps:
		return protocol.CoAPSTransport, nil
	default:
		return 0, fmt.Errorf("unsupported protocol type: %s", pt)
	}
}
