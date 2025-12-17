// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/openapi"
	oapi_owner "github.com/fido-device-onboard/go-fdo-server/api/openapi/owner"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Server implements the openapi.ServerInterface
type Server struct {
	ownerPKeys []crypto.PublicKey
	to2Server  *fdo.TO2Server
	db         *db.State
}

// NewServer creates a new Server instance with defensive validation for systemd environments
func NewServer(ownerPKeys []crypto.PublicKey, to2Server *fdo.TO2Server, database *db.State) *Server {
	// Defensive validation: ensure database is properly initialized
	// This handles systemd service initialization timing issues
	if database != nil && database.DB != nil {
		// Verify database connection with a simple ping
		if sqlDB, err := database.DB.DB(); err == nil {
			if pingErr := sqlDB.Ping(); pingErr != nil {
				database = nil
			}
		} else {
			database = nil
		}
	}

	return &Server{
		ownerPKeys: ownerPKeys,
		to2Server:  to2Server,
		db:         database,
	}
}

// TEMPORARY: Backward compatibility wrappers for manufacturing server
// TODO: Remove these once manufacturing server is refactored to use OpenAPI interface
func GetVoucherHandler(w http.ResponseWriter, r *http.Request) {
	// Create a dummy GetVouchersParams from query parameters
	params := oapi_owner.GetVouchersParams{}
	if guid := r.URL.Query().Get("guid"); guid != "" {
		params.Guid = &guid
	}
	if deviceInfo := r.URL.Query().Get("device_info"); deviceInfo != "" {
		params.DeviceInfo = &deviceInfo
	}
	// Use the server method without server instance (for backward compatibility)
	s := &Server{} // Empty server for compatibility
	s.GetVouchers(w, r, params)
}

func GetVoucherByGUIDHandler(w http.ResponseWriter, r *http.Request) {
	guid := r.PathValue("guid")
	// Use the server method without server instance (for backward compatibility)
	s := &Server{} // Empty server for compatibility
	s.GetVouchersGuid(w, r, guid)
}

// TEMPORARY: Legacy function signature for tests
func InsertVoucherHandler(ownerPKeys []crypto.PublicKey) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := &Server{ownerPKeys: ownerPKeys} // Server with keys for compatibility
		s.PostOwnerVouchers(w, r)
	}
}

func (s *Server) GetVouchers(w http.ResponseWriter, r *http.Request, params oapi_owner.GetVouchersParams) {
	filters := make(map[string]interface{})

	if params.Guid != nil {
		guid, ok := ValidateAndDecodeGUID(w, r, *params.Guid)
		if !ok {
			return
		}
		filters["guid"] = guid
	}

	if params.DeviceInfo != nil {
		filters["device_info"] = *params.DeviceInfo
	}

	vouchers, err := db.QueryVouchers(filters, false)
	if err != nil {
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error querying vouchers", err.Error(), "Error querying vouchers")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	if err := json.NewEncoder(w).Encode(vouchers); err != nil {
		HandleEncodingError(w, r, "response", err)
	}
}

// GetVouchersGuid returns a voucher by path GUID in JSON or PEM format.
func (s *Server) GetVouchersGuid(w http.ResponseWriter, r *http.Request, guid string) {
	guidBytes, ok := ValidateAndDecodeGUID(w, r, guid)
	if !ok {
		return
	}
	voucher, err := db.FetchVoucher(map[string]interface{}{"guid": guidBytes})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteErrorResponse(w, r, http.StatusNotFound, "Voucher not found", "No voucher found with the specified GUID", "Voucher not found")
			return
		}
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Internal server error", err.Error(), err.Error())
		return
	}

	// Check if client wants JSON response (default) or PEM (backward compatibility)
	if ShouldReturnJSON(r) {
		if err := WriteJSONVoucher(w, voucher.CBOR, "pem", guidBytes); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "Failed to encode response", err.Error())
		}
	} else {
		// Backward compatibility: return PEM format
		w.Header().Set("Content-Type", ContentTypePEM)
		if err := pem.Encode(w, &pem.Block{Type: "OWNERSHIP VOUCHER", Bytes: voucher.CBOR}); err != nil {
			HandleEncodingError(w, r, "PEM", err)
		}
	}
}

// VerifyVoucherOwnership verifies the ownership voucher belongs to this owner.
// It checks that the voucher's owner key matches one of the server's configured keys.
func VerifyVoucherOwnership(ov *fdo.Voucher, ownerPKeys []crypto.PublicKey) error {
	if len(ownerPKeys) == 0 {
		return fmt.Errorf("ownerPKeys must contain at least one owner public key")
	}

	expectedPubKey, err := ov.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("unable to parse owner public key from voucher: %w", err)
	}

	// Cast is needed to call Equal()
	// See: https://pkg.go.dev/crypto#PublicKey
	if !slices.ContainsFunc(ownerPKeys, expectedPubKey.(interface{ Equal(crypto.PublicKey) bool }).Equal) {
		return fmt.Errorf("voucher owner key does not match any of the server's configured keys")
	}

	return nil
}

// VerifyOwnershipVoucher performs header field validation and cryptographic verification.
// Note: The following validations can be performed by the device during TO2, not by owner-server,
// so are not included in this verification:
//   - HMAC verification (ov.VerifyHeader): Owner server does not have the device HMAC secret
//   - Manufacturer key hash verification (ov.VerifyManufacturerKey): Requires trusted manufacturer
//     key hashes to be configured (owner server has no source for these hashes)
func VerifyOwnershipVoucher(ov *fdo.Voucher) error {
	// Header Field Validation
	if ov.Version != FDOProtocolVersion {
		return fmt.Errorf("unsupported protocol version: %d (expected %d)", ov.Version, FDOProtocolVersion)
	}
	if ov.Version != ov.Header.Val.Version {
		return fmt.Errorf("protocol version mismatch: voucher version=%d, header version=%d",
			ov.Version, ov.Header.Val.Version)
	}
	var zeroGUID protocol.GUID
	if ov.Header.Val.GUID == zeroGUID {
		return fmt.Errorf("invalid voucher: GUID is zero")
	}
	if ov.Header.Val.DeviceInfo == "" {
		return fmt.Errorf("invalid voucher: DeviceInfo is empty")
	}
	if ov.Header.Val.ManufacturerKey.Type == 0 {
		return fmt.Errorf("invalid voucher: ManufacturerKey is missing or invalid")
	}
	// even for rv bypass there needs to be some instruction, not empty array
	if len(ov.Header.Val.RvInfo) == 0 {
		return fmt.Errorf("invalid voucher: RvInfo is empty")
	}

	// Cryptographic Integrity Verification
	if err := ov.VerifyEntries(); err != nil {
		return fmt.Errorf("signature chain (manufacturer -> owner transfers) verification failed: %w", err)
	}
	if err := ov.VerifyCertChainHash(); err != nil {
		return fmt.Errorf("device certificate chain hash verification failed: %w", err)
	}
	if err := ov.VerifyDeviceCertChain(nil); err != nil {
		return fmt.Errorf("device certificate chain verification failed: %w", err)
	}
	if err := ov.VerifyManufacturerCertChain(nil); err != nil {
		return fmt.Errorf("manufacturer certificate chain verification failed: %w", err)
	}

	return nil
}

// VerifyVoucher performs comprehensive verification of an ownership voucher
// as per FDO spec section 3.4.6. This combines both ownership and integrity checks.
func VerifyVoucher(ov *fdo.Voucher, ownerPKeys []crypto.PublicKey) error {
	if err := VerifyVoucherOwnership(ov, ownerPKeys); err != nil {
		return err
	}

	if err := VerifyOwnershipVoucher(ov); err != nil {
		return err
	}

	return nil
}

// PostOwnerVouchers verifies and inserts vouchers. Background TO0 is handled by the owner server.
func (s *Server) PostOwnerVouchers(w http.ResponseWriter, r *http.Request) {
	PostOwnerVouchersWithKeys(w, r, s.ownerPKeys)
}

// PostOwnerVouchersWithKeys verifies and inserts vouchers with provided owner keys.
func PostOwnerVouchersWithKeys(w http.ResponseWriter, r *http.Request, ownerPKeys []crypto.PublicKey) {
	body, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	response := openapi.VoucherInsertResponse{
		Processed: 0,
		Inserted:  0,
	}

	// Handle PEM data with limited form-encoded support for CI compatibility
	var pemData []byte
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, ContentTypeForm) {
		// Check if the body looks like JSON voucher response (CI compatibility)
		if strings.HasPrefix(string(body), "{") {
			var voucherResp openapi.VoucherResponse
			if err := json.Unmarshal(body, &voucherResp); err == nil {
				// Only accept if it's a proper voucher response from manufacturer
				if voucherResp.Encoding == "pem" && voucherResp.Voucher != "" {
					if voucherBytes, err := base64.StdEncoding.DecodeString(voucherResp.Voucher); err == nil {
						pemBlock := &pem.Block{
							Type:  "OWNERSHIP VOUCHER",
							Bytes: voucherBytes,
						}
						pemData = pem.EncodeToMemory(pemBlock)
					} else {
						AppendError(&response, "Failed to decode base64 voucher from JSON response")
						WriteJSONVoucherInsertResponse(w, response)
						return
					}
				} else {
					AppendError(&response, "Invalid voucher JSON format")
					WriteJSONVoucherInsertResponse(w, response)
					return
				}
			} else {
				AppendError(&response, "Invalid JSON in form-encoded request body")
				WriteJSONVoucherInsertResponse(w, response)
				return
			}
		} else {
			// Reject non-JSON form data - this addresses Miguel's feedback about complexity
			WriteErrorResponse(w, r, http.StatusBadRequest,
				"Invalid form data",
				"Form-encoded data must contain JSON voucher response from manufacturer",
				"Invalid form data")
			return
		}
	} else if contentType != "" && contentType != ContentTypePEM {
		// Reject any content type other than PEM or form (per Miguel's feedback)
		WriteErrorResponse(w, r, http.StatusUnsupportedMediaType,
			"Unsupported content type",
			fmt.Sprintf("Expected %s or %s, got %s", ContentTypePEM, ContentTypeForm, contentType),
			"Unsupported content type")
		return
	} else {
		// Direct PEM data (preferred per Miguel's feedback)
		pemData = body
	}

	block, rest := pem.Decode(pemData)
	for ; block != nil; block, rest = pem.Decode(rest) {
		response.Processed++

		if block.Type != "OWNERSHIP VOUCHER" {
			AppendError(&response, "Unknown block type: %s (expected OWNERSHIP VOUCHER)", block.Type)
			continue
		}

		var ov fdo.Voucher
		if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
			AppendError(&response, "Unable to decode CBOR in voucher %d: %v", response.Processed, err)
			continue
		}

		// Ov Verification
		if err := VerifyVoucher(&ov, ownerPKeys); err != nil {
			AppendError(&response, "Voucher verification failed for GUID %s: %v", hex.EncodeToString(ov.Header.Val.GUID[:]), err)
			continue
		}

		// Check for duplicate vouchers in database
		if dbOv, err := db.FetchVoucher(map[string]interface{}{"guid": ov.Header.Val.GUID[:]}); err == nil {
			if bytes.Equal(block.Bytes, dbOv.CBOR) {
				response.Inserted++ // Count as successful even if it already existed
				continue
			}
			AppendError(&response, "Voucher with GUID %s already exists (not overwriting)", hex.EncodeToString(ov.Header.Val.GUID[:]))
			continue
		}

		// Insert voucher into database

		if err := db.InsertVoucher(db.Voucher{GUID: ov.Header.Val.GUID[:], CBOR: block.Bytes, DeviceInfo: ov.Header.Val.DeviceInfo, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			AppendError(&response, "Database insertion failed for GUID %s: %v", hex.EncodeToString(ov.Header.Val.GUID[:]), err)
			continue
		}

		response.Inserted++
	}

	if len(bytes.TrimSpace(rest)) > 0 {
		AppendError(&response, "Unable to decode remaining PEM content after processing all blocks")
	}

	// Return JSON response or fall back to legacy behavior
	if ShouldReturnJSON(r) {
		WriteJSONVoucherInsertResponse(w, response)
	} else {
		// Backward compatibility: return text/plain
		if response.Errors != nil && len(*response.Errors) > 0 {
			http.Error(w, fmt.Sprintf("Processed %d vouchers, inserted %d, %d errors", response.Processed, response.Inserted, len(*response.Errors)), http.StatusBadRequest)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Successfully processed %d vouchers, inserted %d", response.Processed, response.Inserted)
		}
	}
}

func (s *Server) PostOwnerResellGuid(w http.ResponseWriter, r *http.Request, guid string) {
	PostOwnerResellGuidWithServer(w, r, guid, s.to2Server)
}

// PostOwnerResellGuidWithServer handles voucher reselling with provided TO2 server.
func PostOwnerResellGuidWithServer(w http.ResponseWriter, r *http.Request, guid string, to2Server *fdo.TO2Server) {
	guidBytes, ok := ValidateAndDecodeGUID(w, r, guid)
	if !ok {
		return
	}

	var guidProtocol protocol.GUID
	copy(guidProtocol[:], guidBytes)

	body, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}
	blk, _ := pem.Decode(body)
	if blk == nil {
		WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid PEM content", "Request body must contain valid PEM-encoded public key", "Invalid PEM content")
		return
	}
	nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid public key", err.Error(), "Error parsing x.509 public key")
		return
	}

	// Get the underlying *db.State to access the *gorm.DB for transactions
	state, ok := to2Server.VouchersForExtension.(*db.State)
	if !ok {
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Internal server error", "Invalid state type", "Internal server error: invalid state type")
		return
	}

	// Wrap Resell in a transaction to ensure atomicity
	// If Resell fails after RemoveVoucher, the transaction will rollback
	// and restore the original voucher
	var extended *fdo.Voucher
	err = state.DB.Transaction(func(tx *gorm.DB) error {
		// Create a transactional state wrapper for VouchersForExtension only
		txVouchersForExtension := &db.State{DB: tx}

		// Create a minimal TO2Server copy with only VouchersForExtension replaced
		txTO2Server := *to2Server
		txTO2Server.VouchersForExtension = txVouchersForExtension

		// Call Resell on the copy - it will use the transactional wrapper
		var resellErr error
		extended, resellErr = txTO2Server.Resell(r.Context(), guidProtocol, nextOwner, nil)
		return resellErr
	})

	if err != nil {
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error reselling voucher", err.Error(), "Error reselling voucher")
		// Transaction already rolled back, restoring the original voucher
		// No need to manually add it back
		return
	}

	ovBytes, err := cbor.Marshal(extended)
	if err != nil {
		WriteErrorResponse(w, r, http.StatusInternalServerError, "Error marshaling voucher", err.Error(), "Error marshaling voucher")
		return
	}

	// Check if client wants JSON response (default) or PEM (backward compatibility)
	if ShouldReturnJSON(r) {
		if err := WriteJSONVoucher(w, ovBytes, "pem", guidBytes); err != nil {
			WriteJSONError(w, http.StatusInternalServerError, "Failed to encode response", err.Error())
		}
	} else {
		// Backward compatibility: return PEM format
		w.Header().Set("Content-Type", ContentTypePEM)
		if err := pem.Encode(w, &pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: ovBytes,
		}); err != nil {
			HandleEncodingError(w, r, "PEM", err)
			return
		}
	}
}
