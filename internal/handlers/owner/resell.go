package owner

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
)

func ResellHandler(to2Server *fdo.TO2Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidHex := r.PathValue("guid")

		if !utils.IsValidGUID(guidHex) {
			http.Error(w, "GUID is not a valid GUID", http.StatusBadRequest)
			return
		}

		guidBytes, err := hex.DecodeString(guidHex)
		if err != nil {
			http.Error(w, "Invalid GUID format", http.StatusBadRequest)
			slog.Debug(err.Error())
			return
		}

		var guid protocol.GUID
		copy(guid[:], guidBytes)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failure to read the request body", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}
		blk, _ := pem.Decode(body)
		if blk == nil {
			http.Error(w, "Invalid PEM content", http.StatusInternalServerError)
			return
		}
		nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
		if err != nil {
			http.Error(w, "Error parsing x.509 public key", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		// Get the underlying *db.State to access the *gorm.DB for transactions
		state, ok := to2Server.VouchersForExtension.(*db.State)
		if !ok {
			http.Error(w, "Internal server error: invalid state type", http.StatusInternalServerError)
			slog.Error("VouchersForExtension is not *db.State", "type", fmt.Sprintf("%T", to2Server.VouchersForExtension))
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
			extended, resellErr = txTO2Server.Resell(context.TODO(), guid, nextOwner, nil)
			return resellErr
		})
		if err != nil {
			http.Error(w, "Error reselling voucher", http.StatusInternalServerError)
			slog.Debug("Resell failed", "error", err)
			// Transaction already rolled back, restoring the original voucher
			// No need to manually add it back
			return
		}

		ovBytes, err := cbor.Marshal(extended)
		if err != nil {
			http.Error(w, "Error marshaling voucher", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/x-pem-file")
		if err := pem.Encode(w, &pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: ovBytes,
		}); err != nil {
			slog.Debug("Error encoding voucher", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
