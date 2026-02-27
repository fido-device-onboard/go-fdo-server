// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package serviceinfo

import (
	"context"
	"encoding/hex"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/fido-device-onboard/go-fdo-server/internal/config"
	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/fsim"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
)

// ModuleStateMachines implements serviceinfo.ModuleStateMachine for the owner server
type ModuleStateMachines struct {
	OwnerState *state.OwnerState
	Config     *config.ServiceInfoConfig
	// current module state machine state for all sessions (indexed by token)
	states map[string]*moduleStateMachineState
}

// moduleStateMachineState holds the state for a single TO2 session
type moduleStateMachineState struct {
	Name string
	Impl serviceinfo.OwnerModule
	Next func() (string, serviceinfo.OwnerModule, bool)
	Stop func()
}

// NewModuleStateMachines creates a new service info module state machine
func NewModuleStateMachines(ownerState *state.OwnerState, config *config.ServiceInfoConfig) *ModuleStateMachines {
	return &ModuleStateMachines{
		OwnerState: ownerState,
		Config:     config,
		states:     make(map[string]*moduleStateMachineState),
	}
}

// Module returns the current service info module name and implementation
func (s *ModuleStateMachines) Module(ctx context.Context) (string, serviceinfo.OwnerModule, error) {
	token, ok := s.OwnerState.Token.TokenFromContext(ctx)
	if !ok {
		return "", nil, fmt.Errorf("invalid context: no token")
	}
	module, ok := s.states[token]
	if !ok {
		return "", nil, fmt.Errorf("NextModule not called")
	}
	return module.Name, module.Impl, nil
}

// NextModule progresses to the next service info module
func (s *ModuleStateMachines) NextModule(ctx context.Context) (bool, error) {
	token, ok := s.OwnerState.Token.TokenFromContext(ctx)
	if !ok {
		return false, fmt.Errorf("invalid context: no token")
	}
	module, ok := s.states[token]
	if !ok {
		// Create a new module state machine
		_, modules, _, err := s.OwnerState.TO2Session.Devmod(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting devmod: %w", err)
		}
		// Use background context for iterator since it will be used across multiple HTTP requests
		// The iterator is stored and reused, so we can't use the request context which gets canceled
		// Create a closure that captures the token from this scope
		next, stop := iter.Pull2(func(yield func(string, serviceinfo.OwnerModule) bool) {
			// Create a context with the token for database operations inside the iterator
			tokenCtx := s.OwnerState.Token.TokenContext(context.Background(), token)
			// Call the original ownerModules function with the token context
			for name, module := range ownerModules(tokenCtx, s.Config, modules, s.OwnerState) {
				if !yield(name, module) {
					return
				}
			}
		})
		module = &moduleStateMachineState{
			Next: next,
			Stop: stop,
		}
		s.states[token] = module
	}

	var valid bool
	module.Name, module.Impl, valid = module.Next()
	return valid, nil
}

// CleanupModules cleans up any internal state
func (s *ModuleStateMachines) CleanupModules(ctx context.Context) {
	token, ok := s.OwnerState.Token.TokenFromContext(ctx)
	if !ok {
		return
	}
	module, ok := s.states[token]
	if !ok {
		return
	}
	module.Stop()
	delete(s.states, token)
}

// ownerModules creates an iterator sequence of owner service info modules
func ownerModules(ctx context.Context, config *config.ServiceInfoConfig, modules []string, ownerState *state.OwnerState) iter.Seq2[string, serviceinfo.OwnerModule] { //nolint:gocyclo
	return func(yield func(string, serviceinfo.OwnerModule) bool) {
		if config == nil || len(config.Fsims) == 0 {
			return
		}

		// Process operations in order as defined in configuration
		for _, op := range config.Fsims {
			// Check if the device supports this FSIM module
			if !slices.Contains(modules, op.FSIM) {
				slog.Debug("Device does not support FSIM module, skipping", "module", op.FSIM)
				continue
			}

			switch op.FSIM {
			case "fdo.download":
				for _, file := range op.DownloadParams.Files {
					// Determine absolute path for src
					var srcPath string
					if filepath.IsAbs(file.Src) {
						srcPath = file.Src
					} else {
						srcPath = filepath.Join(op.DownloadParams.Dir, file.Src)
					}

					f, err := os.Open(srcPath) //#nosec G304 //nolint:gosec -- srcPath from service config
					if err != nil {
						slog.Error("error opening file for download FSIM", "path", srcPath, "err", err)
						continue
					}
					defer func() { _ = f.Close() }()

					if !yield("fdo.download", &fsim.DownloadContents[*os.File]{
						Name:         file.Dst,
						Contents:     f,
						MustDownload: !file.MayFail,
					}) {
						return
					}
				}

			case "fdo.upload":
				// Create a per-device upload directory under the configured destination directory. This per-device
				// directory uses the value of the device's replacement GUID as its name. If a per-upload
				// destination is configured (file.Dst) create any subdirectories it requires under the per-device
				// directory.
				for _, file := range op.UploadParams.Files {
					var uploadDir, rename string

					uploadDir = op.UploadParams.Dir
					if uploadDir == "" {
						uploadDir = "."
					}
					replacementGUID, err := ownerState.TO2Session.ReplacementGUID(ctx)
					if err != nil {
						slog.Error("fdo.upload: failed to get per device upload directory name", "err", err)
						return
					}
					uploadDir = filepath.Join(uploadDir, hex.EncodeToString(replacementGUID[:]))

					// note: file.Dst has been validated as a relative path.
					if file.Dst != "" {
						subDir := filepath.Dir(file.Dst)
						if subDir != "." {
							uploadDir = filepath.Join(uploadDir, subDir)
						}
						rename = filepath.Base(file.Dst)
					}
					if err = os.MkdirAll(uploadDir, 0o750); err != nil {
						slog.Error("fdo.upload: failed to create device upload directory", "dir", uploadDir, "err", err)
						continue
					}

					if !yield("fdo.upload", &fsim.UploadRequest{
						Dir:    uploadDir,
						Name:   file.Src,
						Rename: rename,
						CreateTemp: func() (*os.File, error) {
							return os.CreateTemp(uploadDir, ".fdo-upload_*")
						},
					}) {
						return
					}
				}

			case "fdo.wget":
				for _, file := range op.WgetParams.Files {
					parsedURL, err := url.Parse(file.URL)
					if err != nil {
						slog.Error("error parsing wget URL", "url", file.URL, "err", err)
						continue
					}

					// Determine the destination path on the device
					var wgetName string
					if file.Dst != "" {
						// Dst is provided - can be absolute or relative
						if filepath.IsAbs(file.Dst) {
							// Absolute path - use as-is
							wgetName = file.Dst
						} else {
							// Relative path - join with dir if available
							if op.WgetParams.Dir != "" {
								wgetName = filepath.Join(op.WgetParams.Dir, file.Dst)
							} else {
								wgetName = file.Dst
							}
						}
					} else {
						// Dst not provided - use basename of URL, potentially with dir
						basename := path.Base(parsedURL.Path)
						if op.WgetParams.Dir != "" {
							wgetName = filepath.Join(op.WgetParams.Dir, basename)
						} else {
							wgetName = basename
						}
					}

					wgetCmd := &fsim.WgetCommand{
						Name: wgetName,
						URL:  parsedURL,
					}

					if file.Length > 0 {
						wgetCmd.Length = file.Length
					}

					if file.Checksum != "" {
						checksum, err := hex.DecodeString(file.Checksum)
						if err != nil {
							slog.Error("error decoding checksum", "checksum", file.Checksum, "err", err)
							continue
						}
						wgetCmd.Checksum = checksum
					}

					if !yield("fdo.wget", wgetCmd) {
						return
					}
				}

			case "fdo.command":
				cmd := &fsim.RunCommand{
					Command: op.CommandParams.Command,
					Args:    op.CommandParams.Args,
					MayFail: op.CommandParams.MayFail,
				}

				if op.CommandParams.RetStdout {
					cmd.Stdout = os.Stdout
				}

				if op.CommandParams.RetStderr {
					cmd.Stderr = os.Stderr
				}

				if !yield("fdo.command", cmd) {
					return
				}
			}
		}
	}
}
