// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/spf13/cobra"
)

var (
	pubKeyTypeS string
	keySize     int
)

var printOwnerPubkeyCmd = &cobra.Command{
	Use:   "print-owner-pubkey",
	Short: "Print the owner public key.",
	Long:  `Print the owner public key given the key type.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := getState()
		if err != nil {
			return err
		}
		pubKeyType, err := protocol.ParseKeyType(pubKeyTypeS)
		if err != nil {
			return err
		}
		switch pubKeyType {
		case protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
			break
		default:
			keySize = 3072
		}
		key, _, err := state.OwnerKey(context.Background(), pubKeyType, keySize)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKIXPublicKey(key.Public())
		if err != nil {
			return err
		}
		return pem.Encode(os.Stdout, &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: der,
		})
	},
}

func init() {
	rootCmd.AddCommand(printOwnerPubkeyCmd)
	printOwnerPubkeyCmd.Flags().StringVar(&pubKeyTypeS, "type", "", "Public key type")
	printOwnerPubkeyCmd.Flags().IntVar(&keySize, "key-size", 0, "Key size for RSA keys (required for PKCS/PSS, ignored otherwise)")
	printOwnerPubkeyCmd.MarkFlagRequired("type")
}
