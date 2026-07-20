package config

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-server/internal/serviceinfo"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// OwnerServerConfig represents owner server configuration
// which includes common options shared with other servers
// and specific owner configuration
type OwnerServerConfig struct {
	ServerConfig   `mapstructure:",squash"`
	DeviceCAConfig DeviceCAConfig `mapstructure:"device_ca"`
	OwnerConfig    OwnerConfig    `mapstructure:"owner"`
}

// String returns a string representation of OwnerServerConfig with sensitive data redacted
func (o OwnerServerConfig) String() string {
	return fmt.Sprintf("OwnerServerConfig{DB: %s, HTTP: %+v, DeviceCA: %+v, Owner: %+v, Log: %+v}",
		o.DB.String(), o.HTTP, o.DeviceCAConfig, o.OwnerConfig, o.Log)
}

// validate checks that required configuration is present
func (o *OwnerServerConfig) Validate() error {
	slog.Debug("Validating owner server configuration")

	slog.Debug("Validating HTTP configuration")
	if err := o.HTTP.Validate(); err != nil {
		slog.Error("HTTP configuration validation failed", "error", err)
		return err
	}

	slog.Debug("Validating owner private key", "path", o.OwnerConfig.OwnerPrivateKey)
	if o.OwnerConfig.OwnerPrivateKey == "" {
		slog.Error("Owner private key file is required but not provided")
		return errors.New("an owner private key file is required")
	}

	slog.Debug("Validating device CA certificate", "path", o.DeviceCAConfig.CertPath)
	if o.DeviceCAConfig.CertPath == "" {
		slog.Error("Device CA certificate file is required but not provided")
		return errors.New("a device CA certificate file is required")
	}

	// Validate ServiceInfo configuration.
	slog.Debug("Validating ServiceInfo parameters")
	if err := o.OwnerConfig.ServiceInfo.Validate(); err != nil {
		slog.Error("FSIM parameters validation failed", "error", err)
		return err
	}

	slog.Info("Owner server configuration validated successfully")
	return nil
}

// GetDelegateKey loads the delegate private key and certificate chain from the
// configured paths. Returns nil, nil, nil if delegation is not configured.
func (o *OwnerServerConfig) GetDelegateKey() (crypto.Signer, []*x509.Certificate, error) {
	if o.OwnerConfig.Delegate.Name == "" {
		return nil, nil, nil
	}

	if o.OwnerConfig.Delegate.KeyPath == "" {
		return nil, nil, errors.New("delegate name is set but delegate key path is empty")
	}
	if o.OwnerConfig.Delegate.CertPath == "" {
		return nil, nil, errors.New("delegate name is set but delegate cert path is empty")
	}

	slog.Debug("Loading delegate private key", "path", o.OwnerConfig.Delegate.KeyPath)
	key, err := parsePrivateKey(o.OwnerConfig.Delegate.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse delegate private key from %s: %w", o.OwnerConfig.Delegate.KeyPath, err)
	}

	slog.Debug("Loading delegate certificate chain", "path", o.OwnerConfig.Delegate.CertPath)
	chain, err := loadCertificateFromFile(o.OwnerConfig.Delegate.CertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load delegate certificate chain from %s: %w", o.OwnerConfig.Delegate.CertPath, err)
	}
	if len(chain) == 0 {
		return nil, nil, fmt.Errorf("delegate certificate chain is empty in %s", o.OwnerConfig.Delegate.CertPath)
	}

	slog.Info("Delegate key loaded",
		"name", o.OwnerConfig.Delegate.Name,
		"chain_length", len(chain),
		"leaf_subject", chain[0].Subject.String(),
	)

	return key, chain, nil
}

func (o *OwnerServerConfig) GetOwnerSigner() (crypto.Signer, protocol.KeyType, error) {
	slog.Debug("Loading owner private key", "path", o.OwnerConfig.OwnerPrivateKey)
	ownerKey, err := parsePrivateKey(o.OwnerConfig.OwnerPrivateKey)
	if err != nil {
		slog.Error("Failed to parse owner private key", "path", o.OwnerConfig.OwnerPrivateKey, "error", err)
		return nil, 0, fmt.Errorf("failed to parse owner private key from %s: %w", o.OwnerConfig.OwnerPrivateKey, err)
	}
	slog.Debug("Owner private key loaded successfully", "path", o.OwnerConfig.OwnerPrivateKey)
	ownerKeyType, err := getPrivateKeyType(ownerKey)
	if err != nil {
		slog.Error("Failed to determine key type", "error", err)
		return nil, 0, fmt.Errorf("failed to determine key type: %w", err)
	}
	slog.Debug("Owner key type determined", "keyType", ownerKeyType)
	return ownerKey, ownerKeyType, nil
}

func (o *OwnerServerConfig) GetDeviceCACerts() ([]*x509.Certificate, error) {
	slog.Debug("Loading device CA certificates", "path", o.DeviceCAConfig.CertPath)
	certs, err := loadCertificateFromFile(o.DeviceCAConfig.CertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA certificates: %w", err)
	}
	return certs, nil
}

// OwnerConfig represents the configuration specific to the owner
type OwnerConfig struct {
	OwnerCertificate string             `mapstructure:"cert"`
	OwnerPrivateKey  string             `mapstructure:"key"`
	ReuseCred        bool               `mapstructure:"reuse_credentials"`
	TO0InsecureTLS   bool               `mapstructure:"to0_insecure_tls"`
	ServiceInfo      serviceinfo.Config `mapstructure:"service_info"`
	Delegate         DelegateConfig     `mapstructure:"delegate"`
}

// DelegateConfig configures FDO 2.0 delegation. When enabled, the owner
// server uses a delegate key (instead of the owner key) to sign TO2
// messages and perform key exchange. The delegate certificate chain must
// be rooted by the owner key and must carry the appropriate FDO
// permission OIDs.
type DelegateConfig struct {
	// Name is the delegate identifier. When non-empty, delegation is
	// active and this name is used to look up the delegate key.
	Name string `mapstructure:"name"`

	// KeyPath is the path to the PEM-encoded delegate private key file.
	KeyPath string `mapstructure:"key"`

	// CertPath is the path to the PEM-encoded delegate certificate chain
	// file. The chain must be ordered leaf-first and should include all
	// certificates up to (but not including) the owner's root.
	CertPath string `mapstructure:"cert"`
}
