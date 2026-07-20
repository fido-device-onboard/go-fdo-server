package state

import (
	"crypto"
	"crypto/x509"
	"fmt"

	"github.com/fido-device-onboard/go-fdo"
)

// Compile-time check for interface implementation correctness
var _ interface {
	fdo.DelegateKeyPersistentState
} = (*DelegateKeyPersistentState)(nil)

// DelegateKeyPersistentState implements fdo.DelegateKeyPersistentState.
// It holds a named delegate key and certificate chain, loaded at startup
// from configuration.
type DelegateKeyPersistentState struct {
	name  string
	key   crypto.Signer
	chain []*x509.Certificate
}

// NewDelegateKeyPersistentState creates a new DelegateKeyPersistentState.
func NewDelegateKeyPersistentState(name string, key crypto.Signer, chain []*x509.Certificate) *DelegateKeyPersistentState {
	return &DelegateKeyPersistentState{
		name:  name,
		key:   key,
		chain: chain,
	}
}

// DelegateKey implements fdo.DelegateKeyPersistentState.
// Returns the delegate private key and certificate chain for the given
// delegate name. The certificate chain is ordered leaf-first.
func (s *DelegateKeyPersistentState) DelegateKey(name string) (crypto.Signer, []*x509.Certificate, error) {
	if name != s.name {
		return nil, nil, fmt.Errorf("delegate %q not found (only %q is configured)", name, s.name)
	}
	chainCopy := make([]*x509.Certificate, len(s.chain))
	copy(chainCopy, s.chain)
	return s.key, chainCopy, nil
}
