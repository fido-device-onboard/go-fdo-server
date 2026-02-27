package state

import (
	"context"
	"crypto"
	"crypto/x509"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.OwnerKeyPersistentState
} = (*OwnerKeyPersistentState)(nil)

// OwnerKeyPersistentState implements fdo.OwnerKeyPersistentState
type OwnerKeyPersistentState struct {
	signer  crypto.Signer
	keyType protocol.KeyType
	chain   []*x509.Certificate
}

// NewOwnerKeyPersistentState creates a new OwnerKeyPersistentState
func NewOwnerKeyPersistentState(signer crypto.Signer, keyType protocol.KeyType, chain []*x509.Certificate) *OwnerKeyPersistentState {
	return &OwnerKeyPersistentState{
		signer:  signer,
		keyType: keyType,
		chain:   chain,
	}
}

// OwnerKey implements fdo.OwnerKeyPersistentState
func (s *OwnerKeyPersistentState) OwnerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	return s.signer, s.chain, nil
}

// Signer returns the owner signing key (useful for verification)
func (s *OwnerKeyPersistentState) Signer() crypto.Signer {
	return s.signer
}
