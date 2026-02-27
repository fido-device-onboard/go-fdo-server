package config

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"

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

// validate checks that required configuration is present
func (o *OwnerServerConfig) Validate() error {
	slog.Debug("Validating owner server configuration")

	slog.Debug("Validating HTTP configuration")
	if err := o.HTTP.Validate(); err != nil {
		slog.Error("HTTP configuration validation failed", "err", err)
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
		slog.Error("FSIM parameters validation failed", "err", err)
		return err
	}

	slog.Info("Owner server configuration validated successfully")
	return nil
}

func (o *OwnerServerConfig) GetOwnerSigner() (crypto.Signer, error) {
	slog.Debug("Loading owner private key", "path", o.OwnerConfig.OwnerPrivateKey)
	ownerKey, err := parsePrivateKey(o.OwnerConfig.OwnerPrivateKey)
	if err != nil {
		slog.Error("Failed to parse owner private key", "path", o.OwnerConfig.OwnerPrivateKey, "err", err)
		return nil, fmt.Errorf("failed to parse owner private key from %s: %w", o.OwnerConfig.OwnerPrivateKey, err)
	}
	slog.Debug("Owner private key loaded successfully", "path", o.OwnerConfig.OwnerPrivateKey)
	return ownerKey, nil
}

func (o *OwnerServerConfig) GetPrivateKeyType() (protocol.KeyType, error) {
	slog.Debug("Determining owner private key type")
	ownerKey, err := o.GetOwnerSigner()
	if err != nil {
		return 0, err
	}
	ownerKeyType, err := getPrivateKeyType(ownerKey)
	if err != nil {
		slog.Error("Failed to determine key type", "err", err)
		return 0, fmt.Errorf("failed to determine key type: %w", err)
	}
	slog.Debug("Owner key type determined", "keyType", ownerKeyType)
	return ownerKeyType, nil
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
	OwnerCertificate string            `mapstructure:"cert"`
	OwnerPrivateKey  string            `mapstructure:"key"`
	ReuseCred        bool              `mapstructure:"reuse_credentials"`
	TO0InsecureTLS   bool              `mapstructure:"to0_insecure_tls"`
	ServiceInfo      ServiceInfoConfig `mapstructure:"service_info"`
}
