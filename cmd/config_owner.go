package cmd

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

// OwnerServerConfig represents owner server configuration
// which includes common options shared with other servers
// and specific owner configuration
type OwnerServerConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	DeviceCAConfig  DeviceCAConfig `mapstructure:"device_ca"`
	OwnerConfig     OwnerConfig    `mapstructure:"owner"`
}

// OwnerConfig represents the configuration specific to the owner
type OwnerConfig struct {
	OwnerCertificate string `mapstructure:"cert"`
	OwnerPrivateKey  string `mapstructure:"key"`
	ReuseCred        bool   `mapstructure:"reuse_credentials"`
	TO0InsecureTLS   bool   `mapstructure:"to0_insecure_tls"`
}

// validate checks that required configuration is present
func (o *OwnerServerConfig) validate() error {
	slog.Debug("Validating owner server configuration")

	slog.Debug("Validating HTTP configuration")
	if err := o.HTTP.validate(); err != nil {
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

	// Validate FSIM parameters
	slog.Debug("Validating FSIM parameters")
	if err := validateFSIMParameters(); err != nil {
		slog.Error("FSIM parameters validation failed", "err", err)
		return err
	}

	slog.Info("Owner server configuration validated successfully")
	return nil
}

func (o *OwnerServerConfig) getState() (*db.State, error) {
	slog.Debug("Getting database state for owner server")
	state, err := o.DB.getState()
	if err != nil {
		slog.Error("Failed to get database state", "err", err)
		return nil, fmt.Errorf("failed to get database state: %w", err)
	}
	slog.Debug("Database state retrieved successfully")
	return state, nil
}

func (o *OwnerServerConfig) getOwnerSigner() (crypto.Signer, error) {
	slog.Debug("Loading owner private key", "path", o.OwnerConfig.OwnerPrivateKey)
	ownerKey, err := parsePrivateKey(o.OwnerConfig.OwnerPrivateKey)
	if err != nil {
		slog.Error("Failed to parse owner private key", "path", o.OwnerConfig.OwnerPrivateKey, "err", err)
		return nil, fmt.Errorf("failed to parse owner private key from %s: %w", o.OwnerConfig.OwnerPrivateKey, err)
	}
	slog.Debug("Owner private key loaded successfully", "path", o.OwnerConfig.OwnerPrivateKey)
	return ownerKey, nil
}

func (o *OwnerServerConfig) getPrivateKeyType() (protocol.KeyType, error) {
	slog.Debug("Determining owner private key type")
	ownerKey, err := o.getOwnerSigner()
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

func (o *OwnerServerConfig) getDeviceCACerts() ([]*x509.Certificate, error) {
	slog.Debug("Loading device CA certificates", "path", o.DeviceCAConfig.CertPath)
	certs, err := loadCertificateFromFile(o.DeviceCAConfig.CertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA certificates: %w", err)
	}
	return certs, nil
}

func validateFSIMParameters() error {
	// Only validate if FSIM parameters are actually being used
	if !hasFSIMParameters() {
		return nil // No FSIM parameters to validate
	}

	// Parse and validate wget URLs
	wgetURLs = make([]*url.URL, 0, len(wgets))
	for _, urlString := range wgets {
		parsedURL, err := url.Parse(urlString)
		if err != nil {
			return fmt.Errorf("invalid wget URL %q: %w", urlString, err)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("wget URL %q must use http or https scheme, got %q", urlString, parsedURL.Scheme)
		}
		if parsedURL.Host == "" {
			return fmt.Errorf("wget URL %q missing host", urlString)
		}
		wgetURLs = append(wgetURLs, parsedURL)
	}

	// Validate and store cleaned download file paths
	downloadPaths = make([]string, 0, len(downloads))
	for _, filePath := range downloads {
		cleanPath := filepath.Clean(filePath)
		if _, err := os.Stat(cleanPath); err != nil {
			return fmt.Errorf("cannot access download file %q: %w", filePath, err)
		}
		downloadPaths = append(downloadPaths, cleanPath)
	}

	if len(uploads) > 0 && uploadDir == "" {
		return fmt.Errorf("upload directory must be specified when using --command-upload")
	}

	if uploadDir != "" {
		info, err := os.Stat(uploadDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("upload directory %q does not exist", uploadDir)
			}
			return fmt.Errorf("cannot access upload directory %q: %w", uploadDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("upload path %q is not a directory", uploadDir)
		}

		testFile, err := os.CreateTemp(uploadDir, ".fdo-write-test-*")
		if err != nil {
			return fmt.Errorf("upload directory %q is not writable: %w", uploadDir, err)
		}

		// Best effort cleanup after validation
		testFile.Close()
		os.Remove(testFile.Name())
	}

	return nil
}

func hasFSIMParameters() bool {
	return len(wgets) > 0 || len(downloads) > 0 || len(uploads) > 0 || uploadDir != "" || date
}
