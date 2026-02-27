package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
)

// ServiceInfoConfig holds the service_info configuration
type ServiceInfoConfig struct {
	Defaults []DefaultEntry         `mapstructure:"defaults"`
	Fsims    []ServiceInfoOperation `mapstructure:"fsims"`
}

// DefaultEntry defines a default directory for an FSIM operation
type DefaultEntry struct {
	FSIM string `mapstructure:"fsim"`
	Dir  string `mapstructure:"dir"`
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
	Dir   string             `mapstructure:"dir"`
	Files []FSIMWgetFileSpec `mapstructure:"files"`
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

// getDefaultDir returns the default directory for the given FSIM, or empty string if not found
func (s *ServiceInfoConfig) getDefaultDir(fsimName string) string {
	for _, def := range s.Defaults {
		if def.FSIM == fsimName {
			return def.Dir
		}
	}
	return ""
}

// Validate checks that the ServiceInfoConfig is valid
func (s *ServiceInfoConfig) Validate() error {
	if s == nil {
		return nil
	}

	// Validate defaults
	seenFsims := make(map[string]bool)
	for i, def := range s.Defaults {
		// A valid "fsim" field is required
		if def.FSIM == "" {
			return fmt.Errorf("defaults entry %d: fsim field is required", i)
		}
		if def.FSIM != "fdo.download" && def.FSIM != "fdo.upload" && def.FSIM != "fdo.wget" {
			return fmt.Errorf("defaults entry %d: fsim must be one of: fdo.download, fdo.upload, fdo.wget", i)
		}

		// Check for duplicates
		if seenFsims[def.FSIM] {
			return fmt.Errorf("defaults entry %d: duplicate fsim value %q", i, def.FSIM)
		}
		seenFsims[def.FSIM] = true

		// Validate dir is an absolute path
		if def.Dir != "" && !filepath.IsAbs(def.Dir) {
			return fmt.Errorf("defaults entry %d: dir must be an absolute path, got %q", i, def.Dir)
		}

		// For server-side operations, verify base directory exists
		if def.FSIM == "fdo.download" || def.FSIM == "fdo.upload" {
			info, err := os.Stat(def.Dir)
			if err != nil {
				return fmt.Errorf("defaults entry %d: cannot access directory %q: %w", i, def.Dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("defaults entry %d: path %q is not a directory", i, def.Dir)
			}
		}
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

		// Apply defaults if dir is not specified
		switch op.FSIM {
		case "fdo.download":
			if op.DownloadParams != nil && op.DownloadParams.Dir == "" {
				op.DownloadParams.Dir = s.getDefaultDir("fdo.download")
			}
		case "fdo.upload":
			if op.UploadParams != nil && op.UploadParams.Dir == "" {
				op.UploadParams.Dir = s.getDefaultDir("fdo.upload")
			}
		case "fdo.wget":
			if op.WgetParams != nil && op.WgetParams.Dir == "" {
				op.WgetParams.Dir = s.getDefaultDir("fdo.wget")
			}
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
				// Validate that dst (if provided) is not an absolute path
				if file.Dst != "" && filepath.IsAbs(file.Dst) {
					return fmt.Errorf("service_info operation %d, file %d: dst must be a relative path, got %q", i, j, file.Dst)
				}
			}

		case "fdo.download":
			if op.DownloadParams == nil {
				return fmt.Errorf("service_info operation %d: download parameters are required for fdo.download", i)
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
					decoded, err := hex.DecodeString(file.Checksum)
					if err != nil {
						return fmt.Errorf("service_info operation %d, file %d: error decoding checksum %q: %v", i, j, file.Checksum, err)
					}
					const expectedChecksumLength = 48 // SHA-384
					if len(decoded) != expectedChecksumLength {
						return fmt.Errorf("service_info operation %d, file %d: checksum has invalid length, must be a 96-character hex-encoded SHA-384 hash", i, j)
					}
				}
			}

		default:
			return fmt.Errorf("service_info operation %d: unsupported FSIM type %q (supported: fdo.command, fdo.upload, fdo.download, fdo.wget)", i, op.FSIM)
		}
	}
	return nil
}
