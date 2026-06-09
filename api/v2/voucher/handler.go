// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/api/v2/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/middleware"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

// Server implements the StrictServerInterface for ownership voucher management
type Server struct {
	VoucherState  *state.VoucherPersistentState
	DeviceCAState *state.TrustedDeviceCACertsState
	OwnerKeyState *state.OwnerKeyPersistentState
}

func NewServer(
	voucherState *state.VoucherPersistentState,
	deviceCAState *state.TrustedDeviceCACertsState,
	ownerKeyState *state.OwnerKeyPersistentState,
) Server {
	return Server{
		VoucherState:  voucherState,
		DeviceCAState: deviceCAState,
		OwnerKeyState: ownerKeyState,
	}
}

var _ StrictServerInterface = (*Server)(nil)

// ListOwnershipVouchers lists all ownership vouchers with pagination, filtering, and sorting
func (s *Server) ListOwnershipVouchers(
	ctx context.Context,
	request ListOwnershipVouchersRequestObject,
) (ListOwnershipVouchersResponseObject, error) {
	// Set defaults
	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	sortBy := "created_at"
	if request.Params.SortBy != nil {
		switch *request.Params.SortBy {
		case CreatedAt:
			sortBy = "created_at"
		case UpdatedAt:
			sortBy = "updated_at"
		case DeviceInfo:
			sortBy = "device_info"
		case Guid:
			sortBy = "guid"
		}
	}

	sortOrder := "asc"
	if request.Params.SortOrder != nil {
		switch *request.Params.SortOrder {
		case Asc:
			sortOrder = "asc"
		case Desc:
			sortOrder = "desc"
		}
	}

	// Call the database layer with all filters
	vouchers, total, err := s.VoucherState.ListVouchers(
		ctx,
		limit,
		offset,
		request.Params.Guid,
		request.Params.DeviceInfo,
		request.Params.Search,
		sortBy,
		sortOrder,
	)
	if err != nil {
		slog.Error("Failed to list ownership vouchers", "error", err)
		return ListOwnershipVouchers500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to list ownership vouchers",
			},
		}, nil
	}

	// Check preferred content type from context
	preferredContentType := middleware.PreferredContentType(ctx)

	// Return response based on content negotiation
	if preferredContentType == "application/x-pem-file" {
		// Concatenate all vouchers as PEM
		var pemData strings.Builder
		for _, v := range vouchers {
			pemData.WriteString(voucherToPEM(v))
		}

		pemBytes := pemData.String()
		pemReader := bytes.NewReader([]byte(pemBytes))
		return ListOwnershipVouchers200ApplicationxPemFileResponse{
			Body:          pemReader,
			ContentLength: int64(len(pemBytes)),
		}, nil
	}

	// Convert to API response format (JSON)
	summaries := make([]OwnershipVoucherSummaryInfo, len(vouchers))
	for i, v := range vouchers {
		var fdoVoucher fdo.Voucher
		numEntries := 0
		protocolVersion := 0
		if err := cbor.Unmarshal(v.CBOR, &fdoVoucher); err != nil {
			slog.Warn("Failed to unmarshal voucher CBOR", "guid", hex.EncodeToString(v.GUID), "error", err)
		}
		numEntries = len(fdoVoucher.Entries)
		protocolVersion = int(fdoVoucher.Header.Val.Version)

		summaries[i] = OwnershipVoucherSummaryInfo{
			CreatedAt: v.CreatedAt,
			UpdatedAt: v.UpdatedAt,
			Voucher: OwnershipVoucherSummary{
				Guid:            VoucherGuid(hex.EncodeToString(v.GUID)),
				ProtocolVersion: VoucherProtocolVersion(protocolVersion),
				DeviceInfo:      VoucherDeviceInfo(v.DeviceInfo),
				NumEntries:      numEntries,
			},
		}
	}

	return ListOwnershipVouchers200JSONResponse(OwnershipVouchersPaginated{
		Limit:    limit,
		Offset:   offset,
		Total:    int(total),
		Vouchers: summaries,
	}), nil
}

// ImportOwnershipVouchers imports one or more ownership vouchers
func (s *Server) ImportOwnershipVouchers(
	ctx context.Context,
	request ImportOwnershipVouchersRequestObject,
) (ImportOwnershipVouchersResponseObject, error) {
	// Read the body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return ImportOwnershipVouchers500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Parse PEM-encoded ownership vouchers
	type parsedVoucher struct {
		voucher  *fdo.Voucher
		position int
	}
	var parsed []parsedVoucher

	remaining := bodyBytes
	position := 0
	result := OwnershipVouchersImportResult{
		Messages: []string{},
		Vouchers: []OwnershipVoucherSummaryInfo{},
	}

	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}

		if block.Type != "OWNERSHIP VOUCHER" {
			remaining = rest
			continue
		}

		position++

		var voucher fdo.Voucher
		if err := cbor.Unmarshal(block.Bytes, &voucher); err != nil {
			result.Failed++
			result.Messages = append(result.Messages,
				fmt.Sprintf("voucher at position %d failed to parse: %s", position, err.Error()))
			remaining = rest
			continue
		}

		guid := hex.EncodeToString(voucher.Header.Val.GUID[:])

		// Verify header fields and cryptographic integrity
		var deviceCACerts *x509.CertPool
		if s.DeviceCAState != nil {
			deviceCACerts = s.DeviceCAState.CertPool()
		}
		if err := utils.VerifyOwnershipVoucher(&voucher, deviceCACerts); err != nil {
			result.Failed++
			result.Messages = append(result.Messages,
				fmt.Sprintf("voucher at position %d with GUID %s failed verification: %s", position, guid, err.Error()))
			remaining = rest
			continue
		}

		parsed = append(parsed, parsedVoucher{voucher: &voucher, position: position})
		remaining = rest
	}

	result.Detected = position

	// Import vouchers into the database
	now := time.Now()
	for _, p := range parsed {
		guid := hex.EncodeToString(p.voucher.Header.Val.GUID[:])
		err := s.VoucherState.AddVoucher(ctx, p.voucher)
		if err != nil {
			if state.IsDuplicateError(err) {
				result.Skipped++
				result.Messages = append(result.Messages,
					fmt.Sprintf("voucher at position %d with GUID %s was skipped (already exists)", p.position, guid))
			} else {
				result.Failed++
				result.Messages = append(result.Messages,
					fmt.Sprintf("voucher at position %d with GUID %s failed to import: database error", p.position, guid))
				slog.Error("voucher failed to import into the database", "position", p.position, "guid", guid, "err", err.Error())
			}
			continue
		}

		result.Imported++

		// Determine ownership: compare voucher's owner key to server's key
		owned := false
		if s.OwnerKeyState != nil {
			ownerPubKey, pubKeyErr := p.voucher.OwnerPublicKey()
			if pubKeyErr == nil {
				owned = utils.PublicKeysEqual(ownerPubKey, s.OwnerKeyState.Signer().Public())
			}
		}
		if err := s.VoucherState.SetOwnershipVerified(ctx, p.voucher.Header.Val.GUID, owned); err != nil {
			slog.Warn("Failed to set ownership_verified", "guid", guid, "error", err)
		}

		result.Messages = append(result.Messages,
			fmt.Sprintf("voucher at position %d with GUID %s was imported successfully", p.position, guid))
		result.Vouchers = append(result.Vouchers, OwnershipVoucherSummaryInfo{
			CreatedAt: now,
			UpdatedAt: now,
			Voucher: OwnershipVoucherSummary{
				Guid:            VoucherGuid(hex.EncodeToString(p.voucher.Header.Val.GUID[:])),
				ProtocolVersion: VoucherProtocolVersion(p.voucher.Header.Val.Version),
				DeviceInfo:      VoucherDeviceInfo(p.voucher.Header.Val.DeviceInfo),
				NumEntries:      len(p.voucher.Entries),
			},
		})
	}

	if result.Detected == 0 {
		return ImportOwnershipVouchers400JSONResponse(result), nil
	}
	if result.Imported > 0 {
		return ImportOwnershipVouchers201JSONResponse(result), nil
	}
	return ImportOwnershipVouchers200JSONResponse(result), nil
}

// GetOwnershipVoucherByGuid retrieves a single ownership voucher by GUID
func (s *Server) GetOwnershipVoucherByGuid(
	ctx context.Context,
	request GetOwnershipVoucherByGuidRequestObject,
) (GetOwnershipVoucherByGuidResponseObject, error) {
	// Redundant with OpenAPI validation middleware (pattern: ^[a-f0-9]{32}$)
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		slog.Warn("Invalid GUID format", "guid", request.Guid, "error", err)
		return GetOwnershipVoucherByGuid400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: fmt.Sprintf("Invalid GUID format: %s", request.Guid),
			},
		}, nil
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Get voucher from state
	voucher, err := s.VoucherState.Voucher(ctx, guid)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) {
			return GetOwnershipVoucherByGuid404JSONResponse{
				NotFoundJSONResponse: components.NotFoundJSONResponse{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		slog.Error("Failed to get ownership voucher", "error", err, "guid", request.Guid)
		return GetOwnershipVoucherByGuid500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to retrieve ownership voucher",
			},
		}, nil
	}

	// Check preferred content type from context
	preferredContentType := middleware.PreferredContentType(ctx)

	if preferredContentType == "application/x-pem-file" {
		// Marshal voucher to CBOR
		cborBytes, err := cbor.Marshal(voucher)
		if err != nil {
			slog.Error("Failed to marshal voucher to CBOR", "error", err)
			return GetOwnershipVoucherByGuid500JSONResponse{
				InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
					Message: "Failed to encode voucher",
				},
			}, nil
		}

		// Encode as PEM
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: cborBytes,
		})

		return GetOwnershipVoucherByGuid200ApplicationxPemFileResponse{
			Body:          bytes.NewReader(pemBytes),
			ContentLength: int64(len(pemBytes)),
		}, nil
	}

	// For JSON response, convert the full voucher
	apiVoucher, err := convertVoucherToAPI(voucher)
	if err != nil {
		slog.Error("Failed to convert voucher to API format", "error", err)
		return GetOwnershipVoucherByGuid500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to convert voucher format",
			},
		}, nil
	}

	return GetOwnershipVoucherByGuid200JSONResponse(*apiVoucher), nil
}

// DeleteOwnershipVoucher deletes an ownership voucher by GUID
func (s *Server) DeleteOwnershipVoucher(
	ctx context.Context,
	request DeleteOwnershipVoucherRequestObject,
) (DeleteOwnershipVoucherResponseObject, error) {
	// Redundant with OpenAPI validation middleware (pattern: ^[a-f0-9]{32}$)
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		slog.Warn("Invalid GUID format", "guid", request.Guid, "error", err)
		return DeleteOwnershipVoucher400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: fmt.Sprintf("Invalid GUID format: %s", request.Guid),
			},
		}, nil
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Delete voucher
	_, err = s.VoucherState.RemoveVoucher(ctx, guid)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) {
			return DeleteOwnershipVoucher404JSONResponse{
				NotFoundJSONResponse: components.NotFoundJSONResponse{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		slog.Error("Failed to delete ownership voucher", "error", err, "guid", request.Guid)
		return DeleteOwnershipVoucher500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to delete ownership voucher",
			},
		}, nil
	}

	return DeleteOwnershipVoucher204Response{}, nil
}

// ExtendOwnershipVoucher extends an ownership voucher with a new owner key
func (s *Server) ExtendOwnershipVoucher(
	ctx context.Context,
	request ExtendOwnershipVoucherRequestObject,
) (ExtendOwnershipVoucherResponseObject, error) {
	if s.OwnerKeyState == nil {
		return ExtendOwnershipVoucher403JSONResponse{
			ForbiddenJSONResponse: components.ForbiddenJSONResponse{
				Message: "No signing key configured",
			},
		}, nil
	}

	// Read the new owner public key from PEM
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Redundant with OpenAPI validation middleware (pattern: ^[a-f0-9]{32}$)
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		slog.Warn("Invalid GUID format", "guid", request.Guid, "error", err)
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: fmt.Sprintf("Invalid GUID format: %s", request.Guid),
			},
		}, nil
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Parse new owner public key or certificate from PEM.
	// The ownership check (voucher's owner key == server's key) is performed
	// atomically inside ExtendVoucher's transaction to prevent TOCTOU races.
	block, _ := pem.Decode(bodyBytes)
	if block == nil {
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: "Invalid PEM format for owner public key or certificate",
			},
		}, nil
	}

	var nextOwnerKey crypto.PublicKey
	switch block.Type {
	case "PUBLIC KEY":
		nextOwnerKey, err = x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return ExtendOwnershipVoucher400JSONResponse{
				BadRequestJSONResponse: components.BadRequestJSONResponse{
					Message: "Failed to parse new owner public key",
				},
			}, nil
		}
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return ExtendOwnershipVoucher400JSONResponse{
				BadRequestJSONResponse: components.BadRequestJSONResponse{
					Message: "Failed to parse new owner certificate",
				},
			}, nil
		}
		nextOwnerKey = cert.PublicKey
	default:
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequestJSONResponse: components.BadRequestJSONResponse{
				Message: fmt.Sprintf("Expected PEM block type 'PUBLIC KEY' or 'CERTIFICATE', got '%s'", block.Type),
			},
		}, nil
	}

	// Extend the voucher with the new owner's key.
	// Ownership verification and voucher replacement are performed atomically
	// inside a single transaction.
	_, cborBytes, err := s.VoucherState.ExtendVoucher(ctx, guid, s.OwnerKeyState.Signer(), s.OwnerKeyState.Signer().Public(), nextOwnerKey)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) {
			return ExtendOwnershipVoucher404JSONResponse{
				NotFoundJSONResponse: components.NotFoundJSONResponse{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		if errors.Is(err, state.ErrNotOwner) {
			return ExtendOwnershipVoucher403JSONResponse{
				ForbiddenJSONResponse: components.ForbiddenJSONResponse{
					Message: "Server does not own this voucher",
				},
			}, nil
		}
		slog.Error("Failed to extend voucher", "error", err, "guid", request.Guid)
		return ExtendOwnershipVoucher500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to extend voucher",
			},
		}, nil
	}

	// Encode as PEM and return
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: cborBytes,
	})

	return ExtendOwnershipVoucher200ApplicationxPemFileResponse{
		Body:          bytes.NewReader(pemBytes),
		ContentLength: int64(len(pemBytes)),
	}, nil
}

// VerifyOwnership re-verifies ownership of all vouchers against the server's key
func (s *Server) VerifyOwnership(
	ctx context.Context,
	_ VerifyOwnershipRequestObject,
) (VerifyOwnershipResponseObject, error) {
	if s.OwnerKeyState == nil {
		return VerifyOwnership403JSONResponse{
			ForbiddenJSONResponse: components.ForbiddenJSONResponse{
				Message: "No signing key configured",
			},
		}, nil
	}

	owned, total, err := s.VoucherState.MigrateOwnershipVerified(ctx, s.OwnerKeyState.Signer().Public())
	if err != nil {
		slog.Error("Failed to verify ownership", "error", err)
		return VerifyOwnership500JSONResponse{
			InternalServerErrorJSONResponse: components.InternalServerErrorJSONResponse{
				Message: "Failed to verify ownership",
			},
		}, nil
	}

	return VerifyOwnership200JSONResponse(OwnershipVerificationResult{
		Total:   total,
		Owned:   owned,
		Unowned: total - owned,
	}), nil
}

// Helper functions

// convertVoucherToAPI converts a go-fdo Voucher to the API OwnershipVoucher format
func convertVoucherToAPI(voucher *fdo.Voucher) (*OwnershipVoucher, error) {
	// Convert header
	header, err := convertVoucherHeaderToAPI(&voucher.Header.Val)
	if err != nil {
		return nil, fmt.Errorf("failed to convert header: %w", err)
	}

	// Convert entries
	entries := make([]VoucherEntry, len(voucher.Entries))
	for i, entry := range voucher.Entries {
		apiEntry, err := convertVoucherEntryToAPI(&entry.Payload.Val)
		if err != nil {
			return nil, fmt.Errorf("failed to convert entry %d: %w", i, err)
		}
		entries[i] = *apiEntry
	}

	// Convert HMAC
	hmac, err := convertHashToAPI(&voucher.Hmac)
	if err != nil {
		return nil, fmt.Errorf("failed to convert HMAC: %w", err)
	}

	// Convert certificate chain (if present)
	var certChain *[]string
	if voucher.CertChain != nil {
		certs := make([]string, len(*voucher.CertChain))
		for i, cert := range *voucher.CertChain {
			// Encode certificate to PEM
			pemBlock := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: cert.Raw,
			})
			certs[i] = string(pemBlock)
		}
		certChain = &certs
	}

	return &OwnershipVoucher{
		Guid:            VoucherGuid(hex.EncodeToString(voucher.Header.Val.GUID[:])),
		DeviceInfo:      VoucherDeviceInfo(voucher.Header.Val.DeviceInfo),
		ProtocolVersion: VoucherProtocolVersion(voucher.Header.Val.Version),
		NumEntries:      VoucherNumEntries(len(voucher.Entries)),
		Header:          *header,
		Entries:         entries,
		Hmac:            *hmac,
		CertChain:       certChain,
	}, nil
}

// convertVoucherHeaderToAPI converts a go-fdo VoucherHeader to the API format
func convertVoucherHeaderToAPI(header *fdo.VoucherHeader) (*VoucherHeader, error) {
	// Convert manufacturer key
	mfgKey, err := convertPublicKeyToAPI(&header.ManufacturerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert manufacturer key: %w", err)
	}

	// Convert RvInfo by marshaling to JSON and back
	// This handles the complex union type conversion
	rvInfoJSON, err := json.Marshal(header.RvInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RvInfo: %w", err)
	}
	var rvInfo components.RVInfo
	if err := json.Unmarshal(rvInfoJSON, &rvInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RvInfo: %w", err)
	}

	// Convert certificate chain hash (if present)
	var certChainHash *Hash
	if header.CertChainHash != nil {
		certChainHash, err = convertHashToAPI(header.CertChainHash)
		if err != nil {
			return nil, fmt.Errorf("failed to convert cert chain hash: %w", err)
		}
	}

	return &VoucherHeader{
		Version:         int(header.Version),
		Guid:            hex.EncodeToString(header.GUID[:]),
		DeviceInfo:      header.DeviceInfo,
		ManufacturerKey: *mfgKey,
		RvInfo:          rvInfo,
		CertChainHash:   certChainHash,
	}, nil
}

// convertVoucherEntryToAPI converts a go-fdo VoucherEntryPayload to the API format
func convertVoucherEntryToAPI(entry *fdo.VoucherEntryPayload) (*VoucherEntry, error) {
	// Convert public key
	pubKey, err := convertPublicKeyToAPI(&entry.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	// Convert hashes
	previousHash, err := convertHashToAPI(&entry.PreviousHash)
	if err != nil {
		return nil, fmt.Errorf("failed to convert previous hash: %w", err)
	}
	headerHash, err := convertHashToAPI(&entry.HeaderHash)
	if err != nil {
		return nil, fmt.Errorf("failed to convert header hash: %w", err)
	}

	// Convert extra data (if present)
	var extra *map[string]interface{}
	if entry.Extra != nil && entry.Extra.Val != nil {
		extraMap := make(map[string]interface{})
		for k, v := range entry.Extra.Val {
			// Convert integer key to string
			extraMap[fmt.Sprintf("%d", k)] = hex.EncodeToString(v)
		}
		extra = &extraMap
	}

	return &VoucherEntry{
		PreviousHash: *previousHash,
		HeaderHash:   *headerHash,
		PublicKey:    *pubKey,
		Extra:        extra,
	}, nil
}

// convertHashToAPI converts a protocol.Hash to the API Hash format
func convertHashToAPI(hash *protocol.Hash) (*Hash, error) {
	var algorithm HashAlgorithm
	switch hash.Algorithm {
	case protocol.Sha256Hash:
		algorithm = SHA256
	case protocol.Sha384Hash:
		algorithm = SHA384
	case protocol.HmacSha256Hash:
		algorithm = HMACSHA256
	case protocol.HmacSha384Hash:
		algorithm = HMACSHA384
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %v", hash.Algorithm)
	}

	return &Hash{
		Algorithm: algorithm,
		Value:     hex.EncodeToString(hash.Value),
	}, nil
}

// convertPublicKeyToAPI converts a protocol.PublicKey to the API PublicKey format
func convertPublicKeyToAPI(key *protocol.PublicKey) (*PublicKey, error) {
	var keyType PublicKeyType
	switch key.Type {
	case protocol.Rsa2048RestrKeyType:
		keyType = RSA2048RESTR
	case protocol.RsaPkcsKeyType:
		keyType = RSAPKCS
	case protocol.Secp256r1KeyType:
		keyType = SECP256R1
	case protocol.Secp384r1KeyType:
		keyType = SECP384R1
	default:
		return nil, fmt.Errorf("unsupported key type: %v", key.Type)
	}

	return &PublicKey{
		Type:  keyType,
		Value: hex.EncodeToString(key.Body),
	}, nil
}

func voucherToPEM(v state.Voucher) string {
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: v.CBOR,
	})
	return string(pemBytes)
}
