package config

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// The manufacturer server configuration
type ManufacturingConfig struct {
	ManufacturerKeyPath string `mapstructure:"key"`
}

// Manufacturer server configuration file structure
type ManufacturingServerConfig struct {
	ServerConfig ServerConfig        `mapstructure:",squash"`
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
	if path == "" || func() bool { _, err := os.Stat(path); return err != nil }() {
		detail := ""
		if path != "" {
			detail = fmt.Sprintf(" (configured: %s)", path)
		}
		context := ""
		if contextLine != "" {
			context = contextLine + "\n"
		}
		return fmt.Errorf("%s is required%s\n%s"+
			"run 'generate-go-fdo-server-certs.sh' for single-host setup\n"+
			"see CERTIFICATE_SETUP.md for multi-host deployment", name, detail, context)
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
	// Validate owner certificate exists
	if err := validateCertFile(m.Owner.OwnerCertificate, "owner certificate", "this certificate must come from the owner server deployment"); err != nil {
		return err
	}

	slog.Info("Manufacturing server configuration validated successfully")
	return nil
}

// GetManufacturerKey loads the manufacturer private key
func (m *ManufacturingServerConfig) GetManufacturerKey() (crypto.Signer, error) {
	slog.Debug("Loading manufacturer private key", "path", m.Manufacturer.ManufacturerKeyPath)
	key, err := parsePrivateKey(m.Manufacturer.ManufacturerKeyPath)
	if err != nil {
		slog.Error("Failed to parse manufacturer private key", "path", m.Manufacturer.ManufacturerKeyPath, "err", err)
		return nil, fmt.Errorf("failed to parse manufacturer private key from %s: %w", m.Manufacturer.ManufacturerKeyPath, err)
	}
	slog.Debug("Manufacturer private key loaded successfully", "path", m.Manufacturer.ManufacturerKeyPath)
	return key, nil
}

// GetDeviceCAKey loads the device CA private key
func (m *ManufacturingServerConfig) GetDeviceCAKey() (crypto.Signer, error) {
	slog.Debug("Loading device CA private key", "path", m.DeviceCA.KeyPath)
	key, err := parsePrivateKey(m.DeviceCA.KeyPath)
	if err != nil {
		slog.Error("Failed to parse device CA private key", "path", m.DeviceCA.KeyPath, "err", err)
		return nil, fmt.Errorf("failed to parse device CA private key from %s: %w", m.DeviceCA.KeyPath, err)
	}
	slog.Debug("Device CA private key loaded successfully", "path", m.DeviceCA.KeyPath)
	return key, nil
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
