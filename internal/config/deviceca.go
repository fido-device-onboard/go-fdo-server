package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// DeviceCAConfig Device Certificate Authority configuration
type DeviceCAConfig struct {
	CertPath string `mapstructure:"cert"` // path to certificate file
	KeyPath  string `mapstructure:"key"`  // path to key file
}

func (o *DeviceCAConfig) GetDeviceCACertsAsPEM() (string, error) {
	slog.Debug("Reading device CA certificate file", "path", o.CertPath)
	if o.CertPath == "" {
		slog.Warn("Device CA certificate path is empty")
		return "", errors.New("device CA certificate path is empty")
	}
	deviceCA, err := os.ReadFile(o.CertPath)
	if err != nil {
		slog.Error("Failed to read device CA certificate file", "path", o.CertPath, "err", err)
		return "", fmt.Errorf("failed to read device CA cert from %s: %w", o.CertPath, err)
	}
	slog.Debug("Device CA certificate file read successfully", "path", o.CertPath, "size", len(deviceCA))
	return string(deviceCA), nil
}
