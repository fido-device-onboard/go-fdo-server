package config

import (
	"os"
	"testing"
)

func TestManufacturerConfig_Validate_OwnerCertOptional(t *testing.T) {
	// Create temp files for required certs/keys
	mfgKeyFile, _ := os.CreateTemp("", "mfg-key-*.pem")
	defer os.Remove(mfgKeyFile.Name())
	mfgKeyFile.Close()

	deviceCAKeyFile, _ := os.CreateTemp("", "device-ca-key-*.pem")
	defer os.Remove(deviceCAKeyFile.Name())
	deviceCAKeyFile.Close()

	deviceCACertFile, _ := os.CreateTemp("", "device-ca-cert-*.pem")
	defer os.Remove(deviceCACertFile.Name())
	deviceCACertFile.Close()

	cfg := ManufacturingServerConfig{
		ServerConfig: ServerConfig{
			HTTP: HTTPConfig{IP: "127.0.0.1", Port: "8080"},
		},
		Manufacturer: ManufacturingConfig{ManufacturerKeyPath: mfgKeyFile.Name()},
		DeviceCA:     DeviceCAConfig{KeyPath: deviceCAKeyFile.Name(), CertPath: deviceCACertFile.Name()},
		Owner:        OwnerConfig{OwnerCertificate: ""},
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() should succeed with empty owner cert, got: %v", err)
	}
}

func TestManufacturerConfig_GetOwnerCertificate_Empty(t *testing.T) {
	cfg := ManufacturingServerConfig{
		Owner: OwnerConfig{OwnerCertificate: ""},
	}

	cert, err := cfg.GetOwnerCertificate()
	if err != nil {
		t.Fatalf("GetOwnerCertificate() should return nil error for empty path, got: %v", err)
	}
	if cert != nil {
		t.Fatal("GetOwnerCertificate() should return nil cert for empty path")
	}
}
