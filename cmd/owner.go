// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	oapi_owner "github.com/fido-device-onboard/go-fdo-server/api/openapi/owner"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/fsim"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The owner server configuration
type OwnerConfig struct {
	OwnerCertificate string `mapstructure:"cert"`
	OwnerPrivateKey  string `mapstructure:"key"`
	ReuseCred        bool   `mapstructure:"reuse_credentials"`
	TO0InsecureTLS   bool   `mapstructure:"to0_insecure_tls"`
}

// Owner server configuration file structure
type OwnerServerConfig struct {
	FDOServerConfig `mapstructure:",squash"`
	DeviceCA        DeviceCAConfig `mapstructure:"device_ca"`
	Owner           OwnerConfig    `mapstructure:"owner"`
}

// validate checks that required configuration is present
func (o *OwnerServerConfig) validate() error {
	if err := o.HTTP.validate(); err != nil {
		return err
	}
	if o.Owner.OwnerPrivateKey == "" {
		return errors.New("an owner private key file is required")
	}
	if o.DeviceCA.CertPath == "" {
		return errors.New("a device CA certificate file is required")
	}

	// Validate FSIM parameters
	if err := validateFSIMParameters(); err != nil {
		return err
	}

	return nil
}

var (
	// FSIM command line flags
	date          bool
	wgets         []string
	wgetURLs      []*url.URL // Parsed wget URLs
	uploads       []string
	uploadDir     string
	downloads     []string
	downloadPaths []string // Cleaned download file paths
	defaultTo0TTL uint32   = 300
)

// ownerCmd represents the owner command
var ownerCmd = &cobra.Command{
	Use:   "owner http_address",
	Short: "Serve an instance of the owner server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Rebind only those keys needed by the owner command. This is
		// necessary because Viper cannot bind the same key twice and
		// the other sub commands use the same keys.
		if err := viper.BindPFlag("owner.reuse_credentials", cmd.Flags().Lookup("reuse-credentials")); err != nil {
			return err
		}
		if err := viper.BindPFlag("device_ca.cert", cmd.Flags().Lookup("device-ca-cert")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.key", cmd.Flags().Lookup("owner-key")); err != nil {
			return err
		}
		if err := viper.BindPFlag("owner.to0_insecure_tls", cmd.Flags().Lookup("to0-insecure-tls")); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var ownerConfig OwnerServerConfig
		if err := viper.Unmarshal(&ownerConfig); err != nil {
			return fmt.Errorf("failed to unmarshal owner config: %w", err)
		}
		if err := ownerConfig.validate(); err != nil {
			return err
		}
		return serveOwner(&ownerConfig)
	},
}

// Server represents the HTTP server
type OwnerServer struct {
	handler http.Handler
	config  HTTPConfig
}

// NewServer creates a new Server
func NewOwnerServer(config HTTPConfig, handler http.Handler) *OwnerServer {
	return &OwnerServer{handler: handler, config: config}
}

// Start starts the HTTP server
func (s *OwnerServer) Start() error {
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 3 * time.Second,
	}

	// Channel to listen for interrupt or terminate signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to listen for signals and gracefully shut down the server
	go func() {
		<-stop
		slog.Debug("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Debug("Server forced to shutdown:", "err", err)
		}
	}()

	// Listen and serve
	lis, err := net.Listen("tcp", s.config.ListenAddress())
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String())

	if s.config.UseTLS() {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}
		srv.TLSConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			CipherSuites: preferredCipherSuites,
		}
		err := srv.ServeTLS(lis, s.config.CertPath, s.config.KeyPath)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	err = srv.Serve(lis)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

type OwnerServerState struct {
	DB           *db.State
	ownerKey     crypto.Signer
	ownerKeyType protocol.KeyType
	chain        []*x509.Certificate
}

func getOwnerServerState(config *OwnerServerConfig) (*OwnerServerState, error) {
	dbState, err := config.DB.getState()
	if err != nil {
		return nil, err
	}
	ownerKey, err := parsePrivateKey(config.Owner.OwnerPrivateKey)
	if err != nil {
		return nil, err
	}
	ownerKeyType, err := getPrivateKeyType(ownerKey)
	if err != nil {
		return nil, err
	}
	deviceCA, err := os.ReadFile(config.DeviceCA.CertPath)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(deviceCA)
	if blk == nil {
		return nil, fmt.Errorf("unable to decode device CA")
	}
	parsedDeviceCACert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}

	return &OwnerServerState{
		DB:           dbState,
		chain:        []*x509.Certificate{parsedDeviceCACert},
		ownerKey:     ownerKey,
		ownerKeyType: ownerKeyType,
	}, nil
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

func serveOwner(config *OwnerServerConfig) error {
	state, err := getOwnerServerState(config)
	if err != nil {
		return err
	}

	to2Server := &fdo.TO2Server{
		Session:              state.DB,
		Vouchers:             state.DB,
		VouchersForExtension: state.DB,
		OwnerKeys:            state,
		RvInfo: func(_ context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		Modules:         moduleStateMachines{DB: state.DB, states: make(map[string]*moduleStateMachineState)},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return config.Owner.ReuseCred, nil },
		VerifyVoucher: func(_ context.Context, voucher fdo.Voucher) error {
			return handlers.VerifyVoucher(&voucher, []crypto.PublicKey{state.ownerKey.Public()})
		},
	}

	handler := &transport.Handler{
		Tokens:       state.DB,
		TO2Responder: to2Server,
	}

	// Create OpenAPI server instance
	apiServer := handlers.NewServer([]crypto.PublicKey{state.ownerKey.Public()}, to2Server, state.DB)

	// Create the main HTTP handler with chi router for API endpoints
	mainHandler := http.NewServeMux()

	// Register FDO protocol endpoints
	mainHandler.Handle("POST /fdo/101/msg/{msg}", handler)

	// Register ownerinfo endpoint manually (not part of OpenAPI spec but needed for TO0)
	mainHandler.HandleFunc("/api/v1/ownerinfo", handlers.OwnerInfoHandler)

	// Register OpenAPI routes with prefix stripping for all other /api/v1/ requests
	apiHandler := oapi_owner.Handler(apiServer)
	mainHandler.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))

	// Register health endpoint (not part of the OpenAPI spec but needed for backward compatibility)
	mainHandler.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiServer.GetHealth(w, r)
	}))

	// Listen and serve
	server := NewOwnerServer(config.HTTP, mainHandler)

	// Background TO0 scheduler: after restarts, continue attempting TO0 for any
	// devices without completed TO2 as recorded in the database.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		// nextTry holds per-GUID backoff based on TO0 refresh or fallback
		nextTry := make(map[string]time.Time)
		for {
			// Fetch vouchers that still need TO2
			vouchers, err := db.ListPendingTO0Vouchers(true)
			if err != nil {
				slog.Warn("to0 scheduler: list pending vouchers failed", "err", err)
				<-ticker.C
				continue
			}
			now := time.Now()
			for _, v := range vouchers {
				// Parse voucher to get GUID and RVInfo
				var ov fdo.Voucher
				if err := cbor.Unmarshal(v.CBOR, &ov); err != nil {
					slog.Warn("to0 scheduler: unmarshal voucher failed", "err", err)
					continue
				}
				guidHex := hex.EncodeToString(ov.Header.Val.GUID[:])
				// Skip if already completed
				completed, err := db.IsTO2Completed(ov.Header.Val.GUID[:])
				if err != nil {
					slog.Warn("to0 scheduler: to2 completion check failed", "guid", guidHex, "err", err)
					continue
				}
				if completed {
					delete(nextTry, guidHex)
					continue
				} // Respect backoff schedule
				if t, ok := nextTry[guidHex]; ok && now.Before(t) {
					continue
				}
				// Attempt TO0 once for this GUID
				refresh, err := to0.RegisterRvBlob(ov.Header.Val.RvInfo, guidHex, state.DB, state, config.Owner.TO0InsecureTLS, defaultTo0TTL)
				if err != nil {
					// On failure, retry after 60s
					nextTry[guidHex] = now.Add(10 * time.Second)
					slog.Warn("to0 scheduler: register 'RV2TO0Addr' failed", "guid", guidHex, "err", err)
					continue
				}
				if refresh == 0 {
					refresh = defaultTo0TTL
				}
				slog.Debug("to0 scheduler: register 'RV2TO0Addr' completed", "guid", guidHex, "refresh", refresh)
				nextTry[guidHex] = now.Add(time.Duration(refresh) * time.Second)
			}
			<-ticker.C
		}
	}()

	slog.Debug("Starting server on:", "addr", config.HTTP.ListenAddress())
	return server.Start()
}

func (state *OwnerServerState) OwnerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	return state.ownerKey, state.chain, nil
}

type moduleStateMachines struct {
	DB *db.State
	// current module state machine state for all sessions (indexed by token)
	states map[string]*moduleStateMachineState
}

type moduleStateMachineState struct {
	Name string
	Impl serviceinfo.OwnerModule
	Next func() (string, serviceinfo.OwnerModule, bool)
	Stop func()
}

func (s moduleStateMachines) Module(ctx context.Context) (string, serviceinfo.OwnerModule, error) {
	token, ok := s.DB.TokenFromContext(ctx)
	if !ok {
		return "", nil, fmt.Errorf("invalid context: no token")
	}
	module, ok := s.states[token]
	if !ok {
		return "", nil, fmt.Errorf("NextModule not called")
	}
	return module.Name, module.Impl, nil
}

func (s moduleStateMachines) NextModule(ctx context.Context) (bool, error) {
	token, ok := s.DB.TokenFromContext(ctx)
	if !ok {
		return false, fmt.Errorf("invalid context: no token")
	}
	module, ok := s.states[token]
	if !ok {
		// Create a new module state machine
		_, modules, _, err := s.DB.Devmod(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting devmod: %w", err)
		}
		next, stop := iter.Pull2(ownerModules(modules))
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

func (s moduleStateMachines) CleanupModules(ctx context.Context) {
	token, ok := s.DB.TokenFromContext(ctx)
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

func ownerModules(modules []string) iter.Seq2[string, serviceinfo.OwnerModule] { //nolint:gocyclo
	return func(yield func(string, serviceinfo.OwnerModule) bool) {
		if slices.Contains(modules, "fdo.download") {
			for i, cleanPath := range downloadPaths {
				f, err := os.Open(cleanPath)
				if err != nil {
					log.Fatalf("error opening %q for download FSIM: %v", cleanPath, err)
				}
				defer func() { _ = f.Close() }()

				if !yield("fdo.download", &fsim.DownloadContents[*os.File]{
					Name:         downloads[i], // Use original name for display
					Contents:     f,
					MustDownload: true,
				}) {
					return
				}
			}
		}

		if slices.Contains(modules, "fdo.upload") {
			for _, name := range uploads {
				if !yield("fdo.upload", &fsim.UploadRequest{
					Dir:  uploadDir,
					Name: name,
					CreateTemp: func() (*os.File, error) {
						return os.CreateTemp(uploadDir, ".fdo-upload_*")
					},
				}) {
					return
				}
			}
		}

		if slices.Contains(modules, "fdo.wget") {
			for _, url := range wgetURLs {
				if !yield("fdo.wget", &fsim.WgetCommand{
					Name: path.Base(url.Path),
					URL:  url,
				}) {
					return
				}
			}
		}

		if date && slices.Contains(modules, "fdo.command") {
			if !yield("fdo.command", &fsim.RunCommand{
				Command: "date",
				Args:    []string{"--utc"},
				Stdout:  os.Stdout,
				Stderr:  os.Stderr,
			}) {
				return
			}
		}
	}
}

// Set up the owner command line. Used by the unit tests to reset state between tests.
func ownerCmdInit() {
	rootCmd.AddCommand(ownerCmd)

	// TODO: add FSIM to configuration file TBD
	ownerCmd.Flags().BoolVar(&date, "command-date", false, "Use fdo.command FSIM to have device run \"date --utc\"")
	ownerCmd.Flags().StringArrayVar(&wgets, "command-wget", nil, "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")
	ownerCmd.Flags().StringArrayVar(&uploads, "command-upload", nil, "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	ownerCmd.Flags().StringVar(&uploadDir, "upload-directory", "", "The directory `path` to put file uploads")
	ownerCmd.Flags().StringArrayVar(&downloads, "command-download", nil, "Use fdo.download FSIM for each `file` (flag may be used multiple times)")

	// Declare any CLI flags for overriding configuration file settings.
	// These flags are bound to Viper in the ownerCmd PreRun handler.
	ownerCmd.Flags().Bool("reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
	ownerCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	ownerCmd.Flags().String("owner-key", "", "Owner private key path")
	ownerCmd.Flags().Bool("to0-insecure-tls", false, "Use insecure TLS (skip rendezvous certificate verification) for TO0")
}

func init() {
	ownerCmdInit()
}
