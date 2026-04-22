// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package ownerinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo-server/api/v1/components"
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

// MigrateOwnerInfo performs a one-time migration from the legacy owner_info
// table (JSON-encoded) to the new rvto2addr table (CBOR-encoded).
// Should be called once after database initialization, before serving requests.
// JSON parsing is done here in the API layer using the same RVTO2AddrEntry
// types defined in the OpenAPI spec, keeping the state layer free of
// presentation-format concerns.
func (s *Server) MigrateOwnerInfo(ctx context.Context) error {
	rawJSON, err := s.RVTO2AddrState.ReadLegacyOwnerInfoJSON(ctx)
	if err != nil {
		return fmt.Errorf("failed to read legacy owner_info: %w", err)
	}
	if rawJSON == nil {
		return nil // nothing to migrate
	}

	slog.Info("Migrating owner_info from JSON to rvto2addr CBOR format")

	var entries []RVTO2AddrEntry
	if err := json.Unmarshal(rawJSON, &entries); err != nil {
		return fmt.Errorf("failed to parse legacy owner_info JSON: %w", err)
	}

	addrs := make([]protocol.RvTO2Addr, 0, len(entries))
	for i, entry := range entries {
		addr, err := apiToProtocolAddr(entry)
		if err != nil {
			return fmt.Errorf("owner_info[%d]: %w", i, err)
		}
		addrs = append(addrs, addr)
	}

	if err := s.RVTO2AddrState.Upsert(ctx, addrs); err != nil {
		return fmt.Errorf("failed to save migrated rvto2addr: %w", err)
	}

	slog.Info("Successfully migrated owner_info to rvto2addr")
	s.RVTO2AddrState.DropLegacyOwnerInfo()
	return nil
}

// GetRVTO2Addr retrieves the current RVTO2 address configuration
func (s *Server) GetRVTO2Addr(ctx context.Context, request GetRVTO2AddrRequestObject) (GetRVTO2AddrResponseObject, error) {
	slog.Debug("Fetching ownerInfo")
	protocolAddrs, err := s.RVTO2AddrState.Get(ctx)
	if err != nil {
		slog.Error("Error fetching ownerInfo", "error", err)
		return GetRVTO2Addr500TextResponse("Error fetching ownerInfo"), nil
	}

	// Get returns empty array when no config exists - check for this
	if len(protocolAddrs) == 0 {
		slog.Error("No ownerInfo found")
		return GetRVTO2Addr404TextResponse("No ownerInfo found"), nil
	}

	// Convert to API types
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return GetRVTO2Addr200JSONResponse(apiAddrs), nil
}

// CreateRVTO2Addr creates the RVTO2 address configuration
func (s *Server) CreateRVTO2Addr(ctx context.Context, request CreateRVTO2AddrRequestObject) (CreateRVTO2AddrResponseObject, error) {
	if request.Body == nil {
		return CreateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
	}

	// Convert API types to protocol types
	protocolAddrs := make([]protocol.RvTO2Addr, len(*request.Body))
	for i, addr := range *request.Body {
		var convErr error
		protocolAddrs[i], convErr = apiToProtocolAddr(addr)
		if convErr != nil {
			slog.Error("Invalid ownerInfo payload", "error", convErr)
			return CreateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
		}
	}

	// Atomic create — the database enforces the uniqueness constraint
	err := s.RVTO2AddrState.Create(ctx, protocolAddrs)
	if err != nil {
		if errors.Is(err, state.ErrRVTO2AddrExists) {
			return CreateRVTO2Addr409TextResponse("ownerInfo already exists"), nil
		}
		if errors.Is(err, state.ErrInvalidRVTO2Addr) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			return CreateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
		}
		slog.Error("Error inserting ownerInfo", "error", err)
		return CreateRVTO2Addr500TextResponse("Error inserting ownerInfo"), nil
	}

	slog.Debug("ownerInfo created")

	// Convert back to API types for response
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return CreateRVTO2Addr201JSONResponse(apiAddrs), nil
}

// UpdateRVTO2Addr updates the RVTO2 address configuration
func (s *Server) UpdateRVTO2Addr(ctx context.Context, request UpdateRVTO2AddrRequestObject) (UpdateRVTO2AddrResponseObject, error) {
	if request.Body == nil {
		return UpdateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
	}

	// Convert API types to protocol types
	protocolAddrs := make([]protocol.RvTO2Addr, len(*request.Body))
	for i, addr := range *request.Body {
		var convErr error
		protocolAddrs[i], convErr = apiToProtocolAddr(addr)
		if convErr != nil {
			slog.Error("Invalid ownerInfo payload", "error", convErr)
			return UpdateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
		}
	}

	// Atomic update — returns ErrRVTO2AddrNotFound if no row exists
	err := s.RVTO2AddrState.Update(ctx, protocolAddrs)
	if err != nil {
		if errors.Is(err, state.ErrRVTO2AddrNotFound) {
			return UpdateRVTO2Addr404TextResponse("ownerInfo does not exist"), nil
		}
		if errors.Is(err, state.ErrInvalidRVTO2Addr) {
			slog.Error("Invalid ownerInfo payload", "error", err)
			return UpdateRVTO2Addr400TextResponse("Invalid ownerInfo"), nil
		}
		slog.Error("Error updating ownerInfo", "error", err)
		return UpdateRVTO2Addr500TextResponse("Error updating ownerInfo"), nil
	}

	slog.Debug("ownerInfo updated")

	// Convert back to API types for response
	apiAddrs := make([]RVTO2AddrEntry, len(protocolAddrs))
	for i, addr := range protocolAddrs {
		apiAddrs[i] = protocolToAPIAddr(addr)
	}

	return UpdateRVTO2Addr200JSONResponse(apiAddrs), nil
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
		Port:     fmt.Sprintf("%d", addr.Port),
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

	port, err := strconv.Atoi(addr.Port)
	if err != nil {
		return protocol.RvTO2Addr{}, err
	}
	if port < 1 || port > 65535 {
		return protocol.RvTO2Addr{}, fmt.Errorf("port %d out of valid range (1-65535)", port)
	}

	return protocol.RvTO2Addr{
		IPAddress:         ipAddr,
		DNSAddress:        addr.Dns,
		Port:              uint16(port),
		TransportProtocol: transportProto,
	}, nil
}

// transportToAPIProtocol converts a protocol.TransportProtocol to TransportProtocol
func transportToAPIProtocol(tp protocol.TransportProtocol) TransportProtocol {
	switch tp {
	case protocol.TCPTransport:
		return Tcp
	case protocol.TLSTransport:
		return Tls
	case protocol.HTTPTransport:
		return Http
	case protocol.CoAPTransport:
		return Coap
	case protocol.HTTPSTransport:
		return Https
	case protocol.CoAPSTransport:
		slog.Warn("V1 API does not distinguish CoAPS from CoAP, TLS information will be lost")
		return Coap
	default:
		// Default to HTTPS for unknown protocols
		return Https
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
	case Https:
		return protocol.HTTPSTransport, nil
	case Coap:
		return protocol.CoAPTransport, nil
	case Rest:
		// REST typically means HTTPS
		return protocol.HTTPSTransport, nil
	default:
		return 0, fmt.Errorf("unsupported protocol type: %s", pt)
	}
}
