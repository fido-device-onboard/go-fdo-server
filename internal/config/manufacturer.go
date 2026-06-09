package config

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/fido-device-onboard/go-fdo/protocol"
)

// The manufacturer server configuration
type ManufacturingConfig struct {
	ManufacturerKeyPath string `mapstructure:"key"`
}

// Manufacturer server configuration file structure
type ManufacturingServerConfig struct {
	ServerConfig `mapstructure:",squash"`
	DeviceCA     DeviceCAConfig      `mapstructure:"device_ca"`
	Manufacturer ManufacturingConfig `mapstructure:"manufacturing"`
	Owner        OwnerConfig         `mapstructure:"owner"`
}

// String returns a string representation of ManufacturingServerConfig with sensitive data redacted
func (m ManufacturingServerConfig) String() string {
	return fmt.Sprintf("ManufacturingServerConfig{DB: %s, HTTP: %+v, DeviceCA: %+v, Manufacturer: %+v, Owner: %+v, Log: %+v}",
		m.ServerConfig.DB.String(), m.ServerConfig.HTTP, m.DeviceCA, m.Manufacturer, m.Owner, m.ServerConfig.Log)
}

// validateCertFile checks that a certificate file exists and returns a helpful error if not
func validateCertFile(path, name, contextLine string) error {
	if path == "" {
		errContext := ""
		if contextLine != "" {
			errContext = contextLine + "\n"
		}
		return fmt.Errorf("%s is required\n%s"+
			"run 'generate-go-fdo-server-certs.sh' for single-host setup\n"+
			"see docs/user-guide/certificates.md for multi-host deployment", name, errContext)
	}
	if _, err := os.Stat(path); err != nil {
		errContext := ""
		if contextLine != "" {
			errContext = contextLine + "\n"
		}
		return fmt.Errorf("%s is required (configured: %s): %w\n%s"+
			"run 'generate-go-fdo-server-certs.sh' for single-host setup\n"+
			"see docs/user-guide/certificates.md for multi-host deployment", name, path, err, errContext)
	}
	return nil
}

// Validate checks that required configuration is present
func (m *ManufacturingServerConfig) Validate() error {
	slog.Debug("Validating manufacturing server configuration")

	if err := m.ServerConfig.HTTP.Validate(); err != nil {
		return err
	}
	// Validate manufacturing key exists
	if err := validateCertFile(m.Manufacturer.ManufacturerKeyPath, "manufacturing key", ""); err != nil {
		return err
	}
	// Validate device CA key exists
	if err := validateCertFile(m.DeviceCA.KeyPath, "device CA key", "this key must be shared between manufacturer and owner servers"); err != nil {
		return err
	}
	// Validate device CA certificate exists
	if err := validateCertFile(m.DeviceCA.CertPath, "device CA certificate", "this certificate must be shared between manufacturer and owner servers"); err != nil {
		return err
	}
	// Owner certificate is optional — when absent, vouchers are not
	// auto-extended during DI and must be extended via the API.

	slog.Info("Manufacturing server configuration validated successfully")
	return nil
}

func loadKeyWithType(path, name string) (crypto.Signer, protocol.KeyType, error) {
	slog.Debug("Loading private key", "name", name, "path", path)
	key, err := parsePrivateKey(path)
	if err != nil {
		slog.Error("Failed to parse private key", "name", name, "path", path, "error", err)
		return nil, 0, fmt.Errorf("failed to parse %s from %s: %w", name, path, err)
	}
	keytype, err := getPrivateKeyType(key)
	if err != nil {
		slog.Error("Failed to determine key type", "name", name, "error", err)
		return nil, 0, fmt.Errorf("failed to determine key type for %s: %w", name, err)
	}
	slog.Debug("Private key loaded successfully", "name", name, "path", path, "keyType", keytype)
	return key, keytype, nil
}

// GetManufacturerKey loads the manufacturer private key
func (m *ManufacturingServerConfig) GetManufacturerKey() (crypto.Signer, protocol.KeyType, error) {
	return loadKeyWithType(m.Manufacturer.ManufacturerKeyPath, "manufacturer key")
}

// GetDeviceCAKey loads the device CA private key
func (m *ManufacturingServerConfig) GetDeviceCAKey() (crypto.Signer, protocol.KeyType, error) {
	return loadKeyWithType(m.DeviceCA.KeyPath, "device CA key")
}

// GetDeviceCACerts loads the device CA certificate chain
func (m *ManufacturingServerConfig) GetDeviceCACerts() ([]*x509.Certificate, error) {
	slog.Debug("Loading device CA certificates", "path", m.DeviceCA.CertPath)
	certs, err := loadCertificateFromFile(m.DeviceCA.CertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA certificates: %w", err)
	}
	return certs, nil
}

// GetOwnerCertificate loads the owner certificate
func (m *ManufacturingServerConfig) GetOwnerCertificate() (*x509.Certificate, error) {
	if m.Owner.OwnerCertificate == "" {
		return nil, nil
	}
	slog.Debug("Loading owner certificate", "path", m.Owner.OwnerCertificate)

	ownerPublicKey, err := os.ReadFile(m.Owner.OwnerCertificate)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(ownerPublicKey))
	if block == nil {
		return nil, errors.New("unable to decode owner public key")
	}

	ownerCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	slog.Debug("Owner certificate loaded successfully", "path", m.Owner.OwnerCertificate)
	return ownerCert, nil
}
