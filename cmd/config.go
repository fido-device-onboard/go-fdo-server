// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/mitchellh/mapstructure"
)

// Log configuration
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Configuration for the server's HTTP endpoint
type HTTPConfig struct {
	CertPath string `mapstructure:"cert"`
	KeyPath  string `mapstructure:"key"`
	IP       string `mapstructure:"ip"`
	Port     string `mapstructure:"port"`
}

// Device Certificate Authority
type DeviceCAConfig struct {
	CertPath string `mapstructure:"cert"` // path to certificate file
	KeyPath  string `mapstructure:"key"`  // path to key file
}

// Structure to hold the common contents of the configuration file
type FDOServerConfig struct {
	Log  LogConfig      `mapstructure:"log"`
	DB   DatabaseConfig `mapstructure:"db"`
	HTTP HTTPConfig     `mapstructure:"http"`
}

// ListenAddress returns the concatenated IP:Port address for listening
func (h *HTTPConfig) ListenAddress() string {
	return h.IP + ":" + h.Port
}

// UseTLS returns true if TLS should be used (cert and key are both set)
func (h *HTTPConfig) UseTLS() bool {
	return h.CertPath != "" && h.KeyPath != ""
}

func (h *HTTPConfig) validate() error {
	if h.IP == "" {
		return errors.New("the server's HTTP IP address is required")
	}
	if h.Port == "" {
		return errors.New("the server's HTTP port is required")
	}
	// Both cert and key must be set together or both must be unset
	if (h.CertPath == "" && h.KeyPath != "") || (h.CertPath != "" && h.KeyPath == "") {
		return errors.New("both certificate and key must be provided together, or neither")
	}
	return nil
}

// Database configuration
type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

func (dc *DatabaseConfig) getState() (*db.State, error) {
	if dc.DSN == "" {
		return nil, errors.New("database configuration error: dsn is required")
	}

	// Validate database type
	dc.Type = strings.ToLower(dc.Type)
	if dc.Type != "sqlite" && dc.Type != "postgres" {
		return nil, fmt.Errorf("unsupported database type: %s (must be 'sqlite' or 'postgres')", dc.Type)
	}

	return db.InitDb(dc.Type, dc.DSN)
}

// FSIM configuration structures

// FSIMCommandParams holds the parameters for fdo.command FSIM module
type FSIMCommandParams struct {
	Command   string   `mapstructure:"cmd"`
	Args      []string `mapstructure:"args"`
	MayFail   bool     `mapstructure:"may_fail"`
	RetStdout bool     `mapstructure:"return_stdout"`
	RetStderr bool     `mapstructure:"return_stderr"`
}

// FSIMUploadFileSpec defines a file to be uploaded
type FSIMUploadFileSpec struct {
	Src string `mapstructure:"src"`
	Dst string `mapstructure:"dst"`
}

// FSIMUploadParams holds the parameters for fdo.upload FSIM module
type FSIMUploadParams struct {
	Dir   string               `mapstructure:"dir"`
	Files []FSIMUploadFileSpec `mapstructure:"files"`
}

// FSIMDownloadFileSpec defines a file to be downloaded
type FSIMDownloadFileSpec struct {
	Src     string `mapstructure:"src"`
	Dst     string `mapstructure:"dst"`
	MayFail bool   `mapstructure:"may_fail"`
}

// FSIMDownloadParams holds the parameters for fdo.download FSIM module
type FSIMDownloadParams struct {
	Dir   string                 `mapstructure:"dir"`
	Files []FSIMDownloadFileSpec `mapstructure:"files"`
}

// FSIMWgetFileSpec defines a file to be downloaded via wget
type FSIMWgetFileSpec struct {
	URL      string `mapstructure:"url"`
	Dst      string `mapstructure:"dst"`
	Length   int64  `mapstructure:"length"`
	Checksum string `mapstructure:"checksum"`
}

// FSIMWgetParams holds the parameters for fdo.wget FSIM module
type FSIMWgetParams struct {
	Files []FSIMWgetFileSpec `mapstructure:"files"`
}

// ServiceInfoOperation represents a single FSIM operation in the service_info list
// Unmarshalling the configuration into this structure requires two steps: first
// the FSIM is decoded. Once we know the FSIM we can properly decode the RawParams
// into the specific command parameters.  See UnmarshalParams() below.
type ServiceInfoOperation struct {
	FSIM           string                 `mapstructure:"fsim"`
	RawParams      map[string]interface{} `mapstructure:"params"`
	CommandParams  *FSIMCommandParams
	UploadParams   *FSIMUploadParams
	DownloadParams *FSIMDownloadParams
	WgetParams     *FSIMWgetParams
}

// ServiceInfoConfig holds the service_info configuration
type ServiceInfoConfig struct {
	Fsims []ServiceInfoOperation `mapstructure:"fsims"`
}

// UnmarshalParams converts RawParams to the appropriate typed parameter field
// based on the FSIM value. This must be called after Viper unmarshaling.
func (s *ServiceInfoOperation) UnmarshalParams() error {
	if s.RawParams == nil {
		return fmt.Errorf("params field is required for fsim %q", s.FSIM)
	}

	switch s.FSIM {
	case "fdo.command":
		var params FSIMCommandParams
		if err := mapstructure.Decode(s.RawParams, &params); err != nil {
			return fmt.Errorf("failed to decode params for fdo.command: %w", err)
		}
		s.CommandParams = &params

	case "fdo.upload":
		var params FSIMUploadParams
		if err := mapstructure.Decode(s.RawParams, &params); err != nil {
			return fmt.Errorf("failed to decode params for fdo.upload: %w", err)
		}
		s.UploadParams = &params

	case "fdo.download":
		var params FSIMDownloadParams
		if err := mapstructure.Decode(s.RawParams, &params); err != nil {
			return fmt.Errorf("failed to decode params for fdo.download: %w", err)
		}
		s.DownloadParams = &params

	case "fdo.wget":
		var params FSIMWgetParams
		if err := mapstructure.Decode(s.RawParams, &params); err != nil {
			return fmt.Errorf("failed to decode params for fdo.wget: %w", err)
		}
		s.WgetParams = &params

	default:
		return fmt.Errorf("unsupported FSIM type %q", s.FSIM)
	}

	// Clear RawParams to save memory
	s.RawParams = nil
	return nil
}

// validate checks that the ServiceInfoConfig is valid
func (s *ServiceInfoConfig) validate() error {
	if s == nil {
		return nil
	}

	for i := range s.Fsims {
		// First, unmarshal the raw params into typed fields
		if err := s.Fsims[i].UnmarshalParams(); err != nil {
			return fmt.Errorf("service_info operation %d: %w", i, err)
		}

		op := &s.Fsims[i]
		if op.FSIM == "" {
			return fmt.Errorf("service_info operation %d: fsim type is required", i)
		}

		// Validate based on FSIM type
		switch op.FSIM {
		case "fdo.command":
			if op.CommandParams == nil {
				return fmt.Errorf("service_info operation %d: command parameters are required for fdo.command", i)
			}
			if op.CommandParams.Command == "" {
				return fmt.Errorf("service_info operation %d: command is required", i)
			}

		case "fdo.upload":
			if op.UploadParams == nil {
				return fmt.Errorf("service_info operation %d: upload parameters are required for fdo.upload", i)
			}
			if len(op.UploadParams.Files) == 0 {
				return fmt.Errorf("service_info operation %d: at least one file must be specified for upload", i)
			}
			for j, file := range op.UploadParams.Files {
				if file.Src == "" {
					return fmt.Errorf("service_info operation %d, file %d: src is required", i, j)
				}
			}

		case "fdo.download":
			if op.DownloadParams == nil {
				return fmt.Errorf("service_info operation %d: download parameters are required for fdo.download", i)
			}
			if op.DownloadParams.Dir == "" {
				return fmt.Errorf("service_info operation %d: dir is required for fdo.download", i)
			}
			if len(op.DownloadParams.Files) == 0 {
				return fmt.Errorf("service_info operation %d: at least one file must be specified for download", i)
			}
			for j, file := range op.DownloadParams.Files {
				if file.Src == "" {
					return fmt.Errorf("service_info operation %d, file %d: src is required", i, j)
				}
				if file.Dst == "" {
					return fmt.Errorf("service_info operation %d, file %d: dst is required", i, j)
				}
				// Determine absolute path for src to validate file exists
				var srcPath string
				if filepath.IsAbs(file.Src) {
					srcPath = file.Src
				} else {
					srcPath = filepath.Join(op.DownloadParams.Dir, file.Src)
				}
				// Validate that file exists and is readable
				if _, err := os.Stat(srcPath); err != nil {
					return fmt.Errorf("service_info operation %d, file %d: cannot access file %q: %w", i, j, srcPath, err)
				}
			}

		case "fdo.wget":
			if op.WgetParams == nil {
				return fmt.Errorf("service_info operation %d: wget parameters are required for fdo.wget", i)
			}
			if len(op.WgetParams.Files) == 0 {
				return fmt.Errorf("service_info operation %d: at least one file must be specified for wget", i)
			}
			for j, file := range op.WgetParams.Files {
				if file.URL == "" {
					return fmt.Errorf("service_info operation %d, file %d: url is required", i, j)
				}
				// Validate URL format
				parsedURL, err := url.Parse(file.URL)
				if err != nil {
					return fmt.Errorf("service_info operation %d, file %d: invalid URL %q: %w", i, j, file.URL, err)
				}
				if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
					return fmt.Errorf("service_info operation %d, file %d: URL %q must use http or https scheme", i, j, file.URL)
				}
				if parsedURL.Host == "" {
					return fmt.Errorf("service_info operation %d, file %d: URL %q missing host", i, j, file.URL)
				}
				// Validate checksum if present.
				if file.Checksum != "" {
					_, err := hex.DecodeString(file.Checksum)
					if err != nil {
						return fmt.Errorf("service_info operation %d, file %d: error decoding checksum %q: %v", i, j, file.Checksum, err)
					}
				}
			}

		default:
			return fmt.Errorf("service_info operation %d: unsupported FSIM type %q (supported: fdo.command, fdo.upload, fdo.download, fdo.wget)", i, op.FSIM)
		}
	}
	return nil
}
