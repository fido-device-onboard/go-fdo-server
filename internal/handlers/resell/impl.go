package resell

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/handlers/components"
	"github.com/fido-device-onboard/go-fdo/cbor"
)

// Server implements resell HTTP handlers
type Server struct {
	db        *db.State
	to2Server *fdo.TO2Server
}

// NewServer creates a new resell server instance
func NewServer(database *db.State, to2Server *fdo.TO2Server) *Server {
	return &Server{
		db:        database,
		to2Server: to2Server,
	}
}

// Handler creates an HTTP handler for resell endpoints following Miguel's pattern
func Handler(s *Server) http.Handler {
	mux := http.NewServeMux()

	// Manual HTTP handler registration following Miguel's pattern
	mux.HandleFunc("POST /owner/resell/{guid}", s.handleResellVoucher)

	return mux
}

// handleResellVoucher handles voucher reselling following the old implementation pattern
func (s *Server) handleResellVoucher(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Handling voucher resell request")

	// 1. Extract GUID from path parameter
	guidStr := r.PathValue("guid")
	if guidStr == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Missing GUID parameter")
		return
	}

	// 2. Validate and decode GUID
	guidBytes, err := hex.DecodeString(guidStr)
	if err != nil || len(guidBytes) != 16 {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid GUID format")
		return
	}

	// 3. Parse ResellRequest from request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var resellReq ResellRequest
	if err := json.Unmarshal(body, &resellReq); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON in request body")
		return
	}

	// 4. Parse NextOwnerKey from JWK format to crypto.PublicKey
	nextOwnerKey, err := parseJWKPublicKey(resellReq.NextOwnerKey)
	if err != nil {
		slog.Debug("Failed to parse JWK", "error", err, "keyData", resellReq.NextOwnerKey)
		s.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid next owner key: %v", err))
		return
	}

	// 5. Call resell method with GUID and parsed next owner key
	var guidArray [16]byte
	copy(guidArray[:], guidBytes)

	slog.Debug("Calling resell with parsed nextOwner", "guid", guidStr)
	extended, err := s.to2Server.Resell(r.Context(), guidArray, nextOwnerKey, nil)
	if err != nil {
		slog.Debug("Resell failed", "error", err)
		s.writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to resell voucher: %v", err))
		return
	}

	// 7. Encode extended voucher as PEM (following old pattern)
	extendedBytes, err := cbor.Marshal(extended)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to marshal extended voucher")
		return
	}

	pemBlock := &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: extendedBytes,
	}
	pemData := pem.EncodeToMemory(pemBlock)

	// 8. Return successful response
	response := ResellResponse{
		Success:         true,
		ExtendedVoucher: pemData,
		Guid:            &guidStr,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (s *Server) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	errorResp := components.Error{
		Error:   "resell_error",
		Message: message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}

// parseJWKPublicKey parses a JWK (JSON Web Key) format ECDSA public key
func parseJWKPublicKey(keyMap map[string]interface{}) (*ecdsa.PublicKey, error) {
	// Validate key type
	kty, ok := keyMap["kty"].(string)
	if !ok || kty != "EC" {
		return nil, fmt.Errorf("unsupported key type: %v (expected 'EC')", kty)
	}

	// Get curve
	crvStr, ok := keyMap["crv"].(string)
	if !ok {
		return nil, fmt.Errorf("missing curve parameter")
	}

	var curve elliptic.Curve
	var coordinateSize int
	switch crvStr {
	case "P-256":
		curve = elliptic.P256()
		coordinateSize = 32
	case "P-384":
		curve = elliptic.P384()
		coordinateSize = 48
	case "P-521":
		curve = elliptic.P521()
		coordinateSize = 66
	default:
		return nil, fmt.Errorf("unsupported curve: %s", crvStr)
	}

	// Get coordinates
	xStr, ok := keyMap["x"].(string)
	if !ok {
		return nil, fmt.Errorf("missing x coordinate")
	}
	yStr, ok := keyMap["y"].(string)
	if !ok {
		return nil, fmt.Errorf("missing y coordinate")
	}

	// Decode coordinates from base64url
	xBytes, err := base64.RawURLEncoding.DecodeString(xStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode x coordinate: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode y coordinate: %w", err)
	}

	// Validate coordinate length
	if len(xBytes) != coordinateSize {
		return nil, fmt.Errorf("invalid x coordinate length: got %d, expected %d", len(xBytes), coordinateSize)
	}
	if len(yBytes) != coordinateSize {
		return nil, fmt.Errorf("invalid y coordinate length: got %d, expected %d", len(yBytes), coordinateSize)
	}

	// Convert to big.Int
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	// Create ECDSA public key
	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	return pubKey, nil
}
