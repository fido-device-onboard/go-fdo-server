package voucher

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

// Server implements voucher HTTP handlers
type Server struct {
	db         *db.State
	ownerPKeys []crypto.PublicKey
}

// NewServer creates a new voucher server instance
func NewServer(database *db.State) *Server {
	return &Server{
		db: database,
	}
}

// NewServerWithKeys creates a new voucher server instance with owner keys for testing
func NewServerWithKeys(database *db.State, ownerPKeys []crypto.PublicKey) *Server {
	return &Server{
		db:         database,
		ownerPKeys: ownerPKeys,
	}
}

// Handler creates an HTTP handler for voucher endpoints
func Handler(s *Server) http.Handler {
	mux := http.NewServeMux()

	// Manual HTTP handler registration following Miguel's pattern
	// Note: These will be mounted at /api/v1/vouchers/ with prefix stripping
	mux.HandleFunc("POST /owner/vouchers", s.handleUploadOwnerVouchers)
	mux.HandleFunc("GET /", s.handleGetVouchers)            // Handle GET /api/v1/vouchers/
	mux.HandleFunc("GET /{guid}", s.handleGetVoucherByGUID) // Handle GET /api/v1/vouchers/{guid}

	return mux
}

// HTTP handler implementations
func (s *Server) handleUploadOwnerVouchers(w http.ResponseWriter, r *http.Request) {
	// Read request body following Miguel's pattern
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read request body", err.Error())
		return
	}

	response := components.VoucherInsertResponse{
		Processed: 0,
		Inserted:  0,
	}

	// Handle PEM data with limited form-encoded support for CI compatibility
	var pemData []byte
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Check if the body looks like JSON voucher response (CI compatibility)
		if strings.HasPrefix(string(body), "{") {
			var voucherResp components.VoucherResponse
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
						s.writeJSONResponse(w, response)
						return
					}
				} else {
					s.writeJSONResponse(w, response)
					return
				}
			} else {
				s.writeJSONResponse(w, response)
				return
			}
		} else if strings.Contains(string(body), "-----BEGIN OWNERSHIP VOUCHER-----") {
			// Handle raw PEM data sent as form-encoded (curl --data-binary compatibility)
			pemData = body
		} else {
			// Reject invalid form data
			s.writeErrorResponse(w, http.StatusBadRequest, "Invalid form data", "Form-encoded data must contain JSON voucher response or raw PEM data")
			return
		}
	} else if contentType != "" && contentType != "application/x-pem-file" {
		// Reject any content type other than PEM or form
		s.writeErrorResponse(w, http.StatusUnsupportedMediaType, "Unsupported content type", fmt.Sprintf("Expected application/x-pem-file or application/x-www-form-urlencoded, got %s", contentType))
		return
	} else {
		// Direct PEM data (preferred)
		pemData = body
	}

	block, rest := pem.Decode(pemData)
	for ; block != nil; block, rest = pem.Decode(rest) {
		response.Processed++

		if block.Type != "OWNERSHIP VOUCHER" {
			continue
		}

		var ov fdo.Voucher
		if err := cbor.Unmarshal(block.Bytes, &ov); err != nil {
			continue
		}

		// Voucher Verification using owner keys if available
		if s.ownerPKeys != nil {
			if err := VerifyVoucher(&ov, s.ownerPKeys); err != nil {
				continue
			}
		}

		// Check for duplicate vouchers in database
		if s.db != nil {
			if dbOv, err := db.FetchVoucher(map[string]interface{}{"guid": ov.Header.Val.GUID[:]}); err == nil {
				if bytes.Equal(block.Bytes, dbOv.CBOR) {
					response.Inserted++ // Count as successful even if it already existed
					continue
				}
				continue
			}

			// Insert voucher into database
			if err := db.InsertVoucher(db.Voucher{
				GUID:       ov.Header.Val.GUID[:],
				CBOR:       block.Bytes,
				DeviceInfo: ov.Header.Val.DeviceInfo,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}); err != nil {
				continue
			}
		}

		response.Inserted++
	}

	s.writeJSONResponse(w, response)
}

// Helper methods following Miguel's pattern
func (s *Server) writeErrorResponse(w http.ResponseWriter, statusCode int, errorMsg, details string) {
	response := components.Error{
		Error:   errorMsg,
		Details: &details,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) writeJSONResponse(w http.ResponseWriter, response components.VoucherInsertResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleGetVouchers(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement voucher listing logic
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"Not implemented yet"}`))
}

func (s *Server) handleGetVoucherByGUID(w http.ResponseWriter, r *http.Request) {
	// Extract GUID from URL path parameter
	guidStr := r.PathValue("guid")
	if guidStr == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Missing voucher GUID", "GUID not provided in URL")
		return
	}

	// Validate and decode GUID
	if !utils.IsValidGUID(guidStr) {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid GUID format", guidStr)
		return
	}

	guidBytes, err := hex.DecodeString(guidStr)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to decode GUID", err.Error())
		return
	}

	// Fetch voucher from database
	voucher, err := db.FetchVoucher(map[string]interface{}{"guid": guidBytes})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.writeErrorResponse(w, http.StatusNotFound, "Voucher not found", guidStr)
			return
		}
		s.writeErrorResponse(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// Return voucher as PEM-encoded data
	pemBlock := &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: voucher.CBOR,
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.WriteHeader(http.StatusOK)
	pem.Encode(w, pemBlock)
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
func VerifyOwnershipVoucher(ov *fdo.Voucher) error {
	const FDOProtocolVersion uint16 = 101 // FDO spec v1.1

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
