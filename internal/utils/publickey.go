// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

// EncodePublicKey converts a public key to FDO protocol format.
func EncodePublicKey(keyType protocol.KeyType, keyEncoding protocol.KeyEncoding, pub crypto.PublicKey, chain []*x509.Certificate) (*protocol.PublicKey, error) {
	if pub == nil && len(chain) > 0 {
		pub = chain[0].PublicKey
	}
	if pub == nil {
		return nil, fmt.Errorf("no key to encode")
	}

	switch keyEncoding {
	case protocol.X509KeyEnc, protocol.CoseKeyEnc:
		switch keyType {
		case protocol.Secp256r1KeyType, protocol.Secp384r1KeyType:
			ecKey, ok := pub.(*ecdsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("key type %s requires *ecdsa.PublicKey, got %T", keyType, pub)
			}
			return protocol.NewPublicKey(keyType, ecKey, keyEncoding == protocol.CoseKeyEnc)
		case protocol.Rsa2048RestrKeyType, protocol.RsaPkcsKeyType, protocol.RsaPssKeyType:
			rsaKey, ok := pub.(*rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("key type %s requires *rsa.PublicKey, got %T", keyType, pub)
			}
			return protocol.NewPublicKey(keyType, rsaKey, keyEncoding == protocol.CoseKeyEnc)
		default:
			return nil, fmt.Errorf("unsupported key type: %s", keyType)
		}
	case protocol.X5ChainKeyEnc:
		return protocol.NewPublicKey(keyType, chain, false)
	default:
		return nil, fmt.Errorf("unsupported key encoding: %s", keyEncoding)
	}
}
