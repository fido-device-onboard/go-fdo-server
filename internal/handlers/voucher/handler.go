// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package voucher

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/elnormous/contenttype"
	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Server implements the StrictServerInterface for ownership voucher management
type Server struct {
	VoucherState  *state.VoucherPersistentState
	DeviceCAState *state.TrustedDeviceCACertsState
	OwnerKey      *state.OwnerKeyPersistentState
}

func NewServer(voucherState *state.VoucherPersistentState, deviceCAState *state.TrustedDeviceCACertsState, ownerKey *state.OwnerKeyPersistentState) Server {
	return Server{
		VoucherState:  voucherState,
		DeviceCAState: deviceCAState,
		OwnerKey:      ownerKey,
	}
}

var _ StrictServerInterface = (*Server)(nil)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	contentTypeKey contextKey = "preferred-content-type"
)

// ContentNegotiationMiddleware extracts the Accept header from the request
// and stores the preferred content type in the context using RFC 7231-compliant
// content negotiation with quality factor support
func ContentNegotiationMiddleware(f StrictHandlerFunc, operationID string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
		// Extract Accept header
		acceptHeader := r.Header.Get("Accept")

		// Determine preferred content type based on Accept header
		// Default to application/json for all endpoints
		preferredContentType := "application/json"

		if acceptHeader != "" {
			// Available media types this endpoint can produce
			availableMediaTypes := []contenttype.MediaType{
				contenttype.NewMediaType("application/json"),
				contenttype.NewMediaType("application/x-pem-file"),
			}

			// Parse and negotiate the best match based on Accept header
			// This properly handles quality factors (q values)
			accepted, _, err := contenttype.GetAcceptableMediaType(r, availableMediaTypes)
			if err == nil {
				// Successfully negotiated a content type
				preferredContentType = strings.ToLower(accepted.String())
			}
			// If negotiation fails, keep the default (application/json)
		}

		// Add preferred content type to context
		ctx = context.WithValue(ctx, contentTypeKey, preferredContentType)

		// Call the next handler
		return f(ctx, w, r, request)
	}
}

// ListOwnershipVouchers lists all ownership vouchers with pagination, filtering, and sorting
func (s *Server) ListOwnershipVouchers(ctx context.Context, request ListOwnershipVouchersRequestObject) (ListOwnershipVouchersResponseObject, error) {
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
	vouchers, total, err := s.VoucherState.ListVouchers(ctx, limit, offset, request.Params.Guid, request.Params.DeviceInfo, request.Params.Search, sortBy, sortOrder)
	if err != nil {
		slog.Error("Failed to list ownership vouchers", "error", err)
		return ListOwnershipVouchers500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to list ownership vouchers",
			},
		}, nil
	}

	// Check preferred content type from context
	preferredContentType, _ := ctx.Value(contentTypeKey).(string)

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
		// Parse CBOR to get numEntries
		var fdoVoucher fdo.Voucher
		numEntries := 0
		if err := cbor.Unmarshal(v.CBOR, &fdoVoucher); err == nil {
			numEntries = len(fdoVoucher.Entries)
		}

		summaries[i] = OwnershipVoucherSummaryInfo{
			CreatedAt: v.CreatedAt,
			UpdatedAt: v.UpdatedAt,
			Voucher: OwnershipVoucherSummary{
				Guid:       VoucherGuid(hex.EncodeToString(v.GUID)),
				DeviceInfo: VoucherDeviceInfo(v.DeviceInfo),
				NumEntries: numEntries,
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
func (s *Server) ImportOwnershipVouchers(ctx context.Context, request ImportOwnershipVouchersRequestObject) (ImportOwnershipVouchersResponseObject, error) {
	// Read the body
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return ImportOwnershipVouchers400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// For now, assume PEM format (could be enhanced with content-type detection)
	contentType := "application/x-pem-file"

	var vouchers []*fdo.Voucher
	var importMessages []string

	if contentType == "application/x-pem-file" || contentType == "text/plain" {
		// Parse PEM format
		remaining := bodyBytes
		position := 0

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
				importMessages = append(importMessages, fmt.Sprintf("voucher at position %d is malformed: %s", position, err.Error()))
				remaining = rest
				continue
			}

			// Validate device certificate chain against trusted device CAs
			s.DeviceCAState.Mutex.RLock()
			certPool := s.DeviceCAState.TrustedDeviceCACertPool
			s.DeviceCAState.Mutex.RUnlock()

			if err := voucher.VerifyDeviceCertChain(certPool); err != nil {
				guid := hex.EncodeToString(voucher.Header.Val.GUID[:])
				importMessages = append(importMessages, fmt.Sprintf("voucher at position %d with GUID %s is signed by an untrusted device CA: %s", position, guid, err.Error()))
				remaining = rest
				continue
			}

			vouchers = append(vouchers, &voucher)
			remaining = rest
		}
	} else {
		// Try JSON format - single voucher as CBOR hex string
		return ImportOwnershipVouchers400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "JSON import not yet implemented, please use PEM format",
			},
		}, nil
	}

	// Import vouchers
	imported := 0
	skipped := 0

	for i, voucher := range vouchers {
		err := s.VoucherState.AddVoucher(ctx, voucher)
		if err != nil {
			// Check if it's a duplicate (already exists)
			if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
				skipped++
				guid := hex.EncodeToString(voucher.Header.Val.GUID[:])
				importMessages = append(importMessages, fmt.Sprintf("voucher at position %d with GUID %s was skipped (already exists)", i+1, guid))
			} else {
				importMessages = append(importMessages, fmt.Sprintf("voucher at position %d failed to import: %s", i+1, err.Error()))
			}
			continue
		}
		imported++
		guid := hex.EncodeToString(voucher.Header.Val.GUID[:])
		importMessages = append(importMessages, fmt.Sprintf("voucher at position %d with GUID %s was imported successfully", i+1, guid))
	}

	// Create response as JSON
	result := map[string]interface{}{
		"detected": len(vouchers),
		"imported": imported,
		"skipped":  skipped,
		"messages": importMessages,
	}

	resultBytes, _ := json.Marshal(result)
	return ImportOwnershipVouchers201JSONResponse{
		union: resultBytes,
	}, nil
}

// GetOwnershipVoucherByGuid retrieves a single ownership voucher by GUID
func (s *Server) GetOwnershipVoucherByGuid(ctx context.Context, request GetOwnershipVoucherByGuidRequestObject) (GetOwnershipVoucherByGuidResponseObject, error) {
	// Parse GUID from hex string
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		// Invalid GUID - return 404 (no 400 in spec)
		return GetOwnershipVoucherByGuid404JSONResponse{
			NotFound: components.NotFound{
				Message: fmt.Sprintf("Invalid or not found GUID: %s", request.Guid),
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
				NotFound: components.NotFound{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		slog.Error("Failed to get ownership voucher", "error", err, "guid", request.Guid)
		return GetOwnershipVoucherByGuid500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to retrieve ownership voucher",
			},
		}, nil
	}

	// Check preferred content type from context
	preferredContentType, _ := ctx.Value(contentTypeKey).(string)

	if preferredContentType == "application/x-pem-file" {
		// Marshal voucher to CBOR
		cborBytes, err := cbor.Marshal(voucher)
		if err != nil {
			slog.Error("Failed to marshal voucher to CBOR", "error", err)
			return GetOwnershipVoucherByGuid500JSONResponse{
				InternalServerError: components.InternalServerError{
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
			InternalServerError: components.InternalServerError{
				Message: "Failed to convert voucher format",
			},
		}, nil
	}

	return GetOwnershipVoucherByGuid200JSONResponse(*apiVoucher), nil
}

// DeleteOwnershipVoucher deletes an ownership voucher by GUID
func (s *Server) DeleteOwnershipVoucher(ctx context.Context, request DeleteOwnershipVoucherRequestObject) (DeleteOwnershipVoucherResponseObject, error) {
	// Parse GUID from hex string
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		// Invalid GUID - return 404 (no 400 in spec)
		return DeleteOwnershipVoucher404JSONResponse{
			NotFound: components.NotFound{
				Message: fmt.Sprintf("Invalid or not found GUID: %s", request.Guid),
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
				NotFound: components.NotFound{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		slog.Error("Failed to delete ownership voucher", "error", err, "guid", request.Guid)
		return DeleteOwnershipVoucher500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to delete ownership voucher",
			},
		}, nil
	}

	return DeleteOwnershipVoucher204Response{}, nil
}

// ExtendOwnershipVoucher extends an ownership voucher with a new owner key
func (s *Server) ExtendOwnershipVoucher(ctx context.Context, request ExtendOwnershipVoucherRequestObject) (ExtendOwnershipVoucherResponseObject, error) {
	// Read the new owner public key from PEM
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Failed to read request body",
			},
		}, nil
	}

	// Parse GUID from hex string
	guidBytes, err := hex.DecodeString(request.Guid)
	if err != nil || len(guidBytes) != 16 {
		return ExtendOwnershipVoucher404JSONResponse{
			NotFound: components.NotFound{
				Message: fmt.Sprintf("Invalid GUID: %s", request.Guid),
			},
		}, nil
	}

	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Parse new owner public key from PEM
	block, _ := pem.Decode(bodyBytes)
	if block == nil {
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequest: components.BadRequest{
				Message: "Invalid PEM format for owner public key",
			},
		}, nil
	}

	// Parse the public key
	nextOwnerKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		slog.Error("Failed to parse new owner public key", "error", err)
		return ExtendOwnershipVoucher400JSONResponse{
			BadRequest: components.BadRequest{
				Message: fmt.Sprintf("Failed to parse new owner public key: %v", err),
			},
		}, nil
	}

	// Extend the voucher with the new owner's key
	// This operation is performed in a transaction to ensure atomicity
	extended, err := s.VoucherState.ExtendVoucher(ctx, guid, s.OwnerKey.Signer(), nextOwnerKey)
	if err != nil {
		if errors.Is(err, fdo.ErrNotFound) {
			return ExtendOwnershipVoucher404JSONResponse{
				NotFound: components.NotFound{
					Message: fmt.Sprintf("Voucher with GUID %s not found", request.Guid),
				},
			}, nil
		}
		slog.Error("Failed to extend voucher", "error", err, "guid", request.Guid)
		return ExtendOwnershipVoucher500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: fmt.Sprintf("Failed to extend voucher: %v", err),
			},
		}, nil
	}

	// Marshal the extended voucher to CBOR
	cborBytes, err := cbor.Marshal(extended)
	if err != nil {
		slog.Error("Failed to marshal extended voucher", "error", err)
		return ExtendOwnershipVoucher500JSONResponse{
			InternalServerError: components.InternalServerError{
				Message: "Failed to encode extended voucher",
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
	hmac := convertHashToAPI(&voucher.Hmac)

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
		certChainHash = convertHashToAPI(header.CertChainHash)
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
	previousHash := convertHashToAPI(&entry.PreviousHash)
	headerHash := convertHashToAPI(&entry.HeaderHash)

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
func convertHashToAPI(hash *protocol.Hash) *Hash {
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
		// Fallback to SHA-256 for unknown algorithms
		algorithm = SHA256
	}

	return &Hash{
		Algorithm: algorithm,
		Value:     hex.EncodeToString(hash.Value),
	}
}

// convertPublicKeyToAPI converts a protocol.PublicKey to the API PublicKey format
func convertPublicKeyToAPI(key *protocol.PublicKey) (*PublicKey, error) {
	var keyType PublicKeyType
	switch key.Type {
	case protocol.Rsa2048RestrKeyType:
		keyType = RSA2048RESTR
	case protocol.Secp256r1KeyType:
		keyType = SECP256R1
	case protocol.Secp384r1KeyType:
		keyType = SECP384R1
	default:
		return nil, fmt.Errorf("unsupported key type: %v", key.Type)
	}

	// Encode the key body as base64
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
