// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/spf13/cobra"
)

var voucherPath string

var importVoucherCmd = &cobra.Command{
	Use:   "import-voucher path",
	Short: "Import an ownership voucher",
	Long:  `Import takes a PEM encoded voucher and store it in the owner database.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		voucherPath = args[0]
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse voucher
		state, err := getState()
		if err != nil {
			return err
		}
		pemVoucher, err := os.ReadFile(filepath.Clean(voucherPath))
		if err != nil {
			return err
		}
		blk, _ := pem.Decode(pemVoucher)
		if blk == nil {
			return fmt.Errorf("invalid PEM encoded file: %s", voucherPath)
		}
		if blk.Type != "OWNERSHIP VOUCHER" {
			return fmt.Errorf("expected PEM block of ownership voucher type, found %s", blk.Type)
		}
		var ov fdo.Voucher
		if err := cbor.Unmarshal(blk.Bytes, &ov); err != nil {
			return fmt.Errorf("error parsing voucher: %w", err)
		}

		// Check that voucher owner key matches
		expectedPubKey, err := ov.OwnerPublicKey()
		if err != nil {
			return fmt.Errorf("error parsing owner public key from voucher: %w", err)
		}
		ownerKey, _, err := state.OwnerKey(context.Background(), ov.Header.Val.ManufacturerKey.Type, 3072)
		if err != nil {
			return fmt.Errorf("error getting owner key: %w", err)
		}
		if !ownerKey.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(expectedPubKey) {
			return fmt.Errorf("owner key in database does not match the owner of the voucher")
		}

		// Store voucher
		return state.AddVoucher(context.Background(), &ov)
	},
}

func init() {
	rootCmd.AddCommand(importVoucherCmd)
}
