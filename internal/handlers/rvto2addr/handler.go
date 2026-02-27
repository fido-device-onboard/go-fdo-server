// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvto2addr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
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
			InternalServerError: components.InternalServerError{
				Message: "Failed to retrieve RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert to API types
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return GetRVTO2Addr200JSONResponse(apiAddrs), nil
}

// UpdateRVTO2Addr updates the RVTO2 address configuration
func (s *Server) UpdateRVTO2Addr(ctx context.Context, request UpdateRVTO2AddrRequestObject) (UpdateRVTO2AddrResponseObject, error) {
	if request.Body == nil {
		return UpdateRVTO2Addr400JSONResponse{
			BadRequest: components.BadRequest{
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
				BadRequest: components.BadRequest{
					Message: fmt.Sprintf("Invalid address at index %d: %s", i, err.Error()),
				},
			}, nil
		}
	}

	err := s.RVTO2AddrState.Update(ctx, protocolAddrs)
	if err != nil {
		if errors.Is(err, state.ErrInvalidRVTO2Addr) {
			return UpdateRVTO2Addr400JSONResponse{
				BadRequest: components.BadRequest{
					Message: err.Error(),
				},
			}, nil
		}
		slog.Error("Failed to update RVTO2Addr", "error", err)
		return UpdateRVTO2Addr500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to update RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert back to API types for response
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return UpdateRVTO2Addr200JSONResponse(apiAddrs), nil
}

// DeleteRVTO2Addr deletes the RVTO2 address configuration
func (s *Server) DeleteRVTO2Addr(ctx context.Context, request DeleteRVTO2AddrRequestObject) (DeleteRVTO2AddrResponseObject, error) {
	protocolAddrs, err := s.RVTO2AddrState.Delete(ctx)
	if err != nil {
		slog.Error("Failed to delete RVTO2Addr", "error", err)
		return DeleteRVTO2Addr500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to delete RVTO2Addr configuration",
			},
		}, nil
	}

	// Convert to API types
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return DeleteRVTO2Addr200JSONResponse(apiAddrs), nil
}

// protocolToAPIAddr converts a protocol.RvTO2Addr to an API RVTO2AddrEntry
func protocolToAPIAddr(addr protocol.RvTO2Addr) RVTO2AddrEntry {
	var dns *components.DNSHostname
	if addr.DNSAddress != nil {
		dns = addr.DNSAddress
	}

	var ip *components.IPv4Address
	if addr.IPAddress != nil {
		ipStr := addr.IPAddress.String()
		ip = &ipStr
	}

	return RVTO2AddrEntry{
		Dns:      dns,
		Ip:       ip,
		Port:     components.PortNumber(addr.Port),
		Protocol: transportToAPIProtocol(addr.TransportProtocol),
	}
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

// transportToAPIProtocol converts a protocol.TransportProtocol to components.ProtocolType
func transportToAPIProtocol(tp protocol.TransportProtocol) components.ProtocolType {
	switch tp {
	case protocol.TCPTransport:
		return components.Tcp
	case protocol.TLSTransport:
		return components.Tls
	case protocol.HTTPTransport:
		return components.Http
	case protocol.CoAPTransport:
		return components.Coap
	case protocol.HTTPSTransport:
		return components.Https
	case protocol.CoAPSTransport:
		// Note: CoAPS maps to "coap" as there's no separate CoAPS in the API
		return components.Coap
	default:
		// Default to HTTPS for unknown protocols
		return components.Https
	}
}

// apiToTransportProtocol converts a components.ProtocolType to protocol.TransportProtocol
func apiToTransportProtocol(pt components.ProtocolType) (protocol.TransportProtocol, error) {
	switch pt {
	case components.Tcp:
		return protocol.TCPTransport, nil
	case components.Tls:
		return protocol.TLSTransport, nil
	case components.Http:
		return protocol.HTTPTransport, nil
	case components.Https:
		return protocol.HTTPSTransport, nil
	case components.Coap:
		return protocol.CoAPTransport, nil
	case components.CoapTcp:
		// Map coap+tcp to CoAP for now
		return protocol.CoAPTransport, nil
	case components.Rest:
		// REST typically means HTTPS
		return protocol.HTTPSTransport, nil
	default:
		return 0, fmt.Errorf("unsupported protocol type: %s", pt)
	}
}
