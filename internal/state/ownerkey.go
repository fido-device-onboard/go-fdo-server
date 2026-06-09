package state

import (
	"context"
	"crypto"
	"crypto/x509"
	"fmt"

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

// NewOwnerKeyPersistentState creates a new OwnerKeyPersistentState.
// The struct is immutable after construction — fields are never modified.
func NewOwnerKeyPersistentState(signer crypto.Signer, keyType protocol.KeyType, chain []*x509.Certificate) *OwnerKeyPersistentState {
	return &OwnerKeyPersistentState{
		signer:  signer,
		keyType: keyType,
		chain:   chain,
	}
}

// OwnerKey implements fdo.OwnerKeyPersistentState.
// The rsaBits parameter is required by the interface but unused — the key is
// pre-loaded at construction time.
// Returns a defensive copy of the certificate chain.
func (s *OwnerKeyPersistentState) OwnerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	if keyType != s.keyType {
		return nil, nil, fmt.Errorf("requested key type %d does not match configured key type %d", keyType, s.keyType)
	}
	chainCopy := make([]*x509.Certificate, len(s.chain))
	copy(chainCopy, s.chain)
	return s.signer, chainCopy, nil
}

// Signer returns the owner signing key (useful for verification)
func (s *OwnerKeyPersistentState) Signer() crypto.Signer {
	return s.signer
}
