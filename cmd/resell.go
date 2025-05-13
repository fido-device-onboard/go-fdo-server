// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
)

var (
	guidS         string
	resaleKeyPath string
)

var resellCmd = &cobra.Command{
	Use:   "resell",
	Short: "Run the FDO resale protocol",
	Long: `Resell takes a stored voucher ID and a new owner key and runs the FDO
	resale protocol`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse resale-guid flag
		guidBytes, err := hex.DecodeString(strings.ReplaceAll(guidS, "-", ""))
		if err != nil {
			return fmt.Errorf("error parsing GUID of voucher to resell: %w", err)
		}
		if len(guidBytes) != 16 {
			return fmt.Errorf("error parsing GUID of voucher to resell: must be 16 bytes")
		}
		var guid protocol.GUID
		copy(guid[:], guidBytes)
		resaleKeyPath = args[1]
		if resaleKeyPath == "" {
			return fmt.Errorf("resale-guid depends on resale-key argument being set")
		}
		// Parse next owner key
		keyBytes, err := os.ReadFile(filepath.Clean(resaleKeyPath))
		if err != nil {
			return fmt.Errorf("error reading next owner key file: %w", err)
		}
		blk, _ := pem.Decode(keyBytes)
		if blk == nil {
			return fmt.Errorf("invalid PEM file: %s", resaleKeyPath)
		}
		nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
		if err != nil {
			return fmt.Errorf("error parsing x.509 public key: %w", err)
		}

		state, err := getState()
		if err != nil {
			return err
		}
		// Perform resale protocol
		extended, err := (&fdo.TO2Server{
			Vouchers:  state,
			OwnerKeys: state,
		}).Resell(context.TODO(), guid, nextOwner, nil)
		if err != nil {
			return fmt.Errorf("resale protocol: %w", err)
		}
		ovBytes, err := cbor.Marshal(extended)
		if err != nil {
			return fmt.Errorf("resale protocol: error marshaling voucher: %w", err)
		}
		return pem.Encode(os.Stdout, &pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: ovBytes,
		})
	},
}

func init() {
	rootCmd.AddCommand(resellCmd)
	resellCmd.Flags().StringVar(&guidS, "guid", "", "Voucher guid to extend for resale")
	resellCmd.Flags().StringVar(&resaleKeyPath, "key", "", "Path to a PEM-encoded x.509 public key for the next owner")
	// TODO(runcom): why MarkFlagsRequiredTogether doesn't work?
	resellCmd.MarkFlagRequired("guid")
	resellCmd.MarkFlagRequired("key")
}
