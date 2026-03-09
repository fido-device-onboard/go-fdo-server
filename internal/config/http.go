package config

import (
	"errors"
	"log/slog"
)

// Configuration for the server's HTTP endpoint
type HTTPConfig struct {
	CertPath string `mapstructure:"cert"`
	KeyPath  string `mapstructure:"key"`
	IP       string `mapstructure:"ip"`
	Port     string `mapstructure:"port"`
}

// ListenAddress returns the concatenated IP:Port address for listening
func (h *HTTPConfig) ListenAddress() string {
	return h.IP + ":" + h.Port
}

// UseTLS returns true if TLS should be used (cert and key are both set)
func (h *HTTPConfig) UseTLS() bool {
	return h.CertPath != "" && h.KeyPath != ""
}

func (h *HTTPConfig) Validate() error {
	slog.Debug("Validating HTTP configuration", "ip", h.IP, "port", h.Port, "tls", h.UseTLS())
	if h.IP == "" {
		slog.Error("HTTP IP address is required but not provided")
		return errors.New("the server's HTTP IP address is required")
	}
	if h.Port == "" {
		slog.Error("HTTP port is required but not provided")
		return errors.New("the server's HTTP port is required")
	}
	// Both cert and key must be set together or both must be unset
	if (h.CertPath == "" && h.KeyPath != "") || (h.CertPath != "" && h.KeyPath == "") {
		slog.Error("Invalid TLS configuration: cert and key must both be provided or both be empty",
			"cert", h.CertPath, "key", h.KeyPath)
		return errors.New("both certificate and key must be provided together, or neither")
	}
	if h.UseTLS() {
		slog.Debug("TLS enabled for HTTP server", "cert", h.CertPath, "key", h.KeyPath)
	} else {
		slog.Debug("TLS not enabled for HTTP server")
	}
	return nil
}
