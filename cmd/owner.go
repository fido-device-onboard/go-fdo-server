// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/fsim"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The owner server configuration
type OwnerConfig struct {
	ExternalAddress   string         `mapstructure:"external-address"`
	OwnerDeviceCACert string         `mapstructure:"device-ca-cert"`
	OwnerPrivateKey   string         `mapstructure:"owner-key"`
	ReuseCred         bool           `mapstructure:"reuse-credentials"`
	HTTP              HTTPConfig     `mapstructure:"http"`
	DB                DatabaseConfig `mapstructure:"database"`
}

var (
	// FSIM configuration TBD
	date      bool
	wgets     []string
	uploads   []string
	uploadDir string
	downloads []string
)

// ownerCmd represents the owner command
var ownerCmd = &cobra.Command{
	Use:   "owner http_address",
	Short: "Serve an instance of the owner server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			viper.Set("owner.http.listen", args[0])
		}

		var fdoConfig FIDOServerConfig
		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return fmt.Errorf("failed to unmarshal owner config: %w", err)
		}

		if fdoConfig.Owner == nil {
			return fmt.Errorf("failed to find Owner config")
		}
		return serveOwner(fdoConfig.Owner)
	},
}

// Server represents the HTTP server
type OwnerServer struct {
	handler http.Handler
	config  HTTPConfig
	extAddr string
}

// NewServer creates a new Server
func NewOwnerServer(config HTTPConfig, extAddr string, handler http.Handler) *OwnerServer {
	return &OwnerServer{handler: handler, config: config, extAddr: extAddr}
}

// Start starts the HTTP server
func (s *OwnerServer) Start() error {
	err := s.config.validate()
	if err != nil {
		return err
	}
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
	lis, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String(), "external", s.extAddr)

	if s.config.UseTLS {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}

		if s.config.CertPath != "" && s.config.KeyPath != "" {
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, s.config.CertPath, s.config.KeyPath)
		} else {
			return fmt.Errorf("no TLS cert or key provided")
		}
	}
	return srv.Serve(lis)
}

type OwnerServerState struct {
	DB           *db.State
	ownerKey     crypto.Signer
	ownerKeyType protocol.KeyType
	chain        []*x509.Certificate
}

func getOwnerServerState(config *OwnerConfig) (*OwnerServerState, error) {
	dbState, err := config.DB.getState()
	if err != nil {
		return nil, err
	}
	ownerKey, err := parsePrivateKey(config.OwnerPrivateKey)
	if err != nil {
		return nil, err
	}
	ownerKeyType, err := getPrivateKeyType(ownerKey)
	if err != nil {
		return nil, err
	}
	deviceCA, err := os.ReadFile(config.OwnerDeviceCACert)
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

func serveOwner(config *OwnerConfig) error {
	state, err := getOwnerServerState(config)
	if err != nil {
		return err
	}

	to2Server := &fdo.TO2Server{
		Session:   state.DB,
		Vouchers:  state.DB,
		OwnerKeys: state,
		RvInfo: func(_ context.Context, voucher fdo.Voucher) ([][]protocol.RvInstruction, error) {
			return voucher.Header.Val.RvInfo, nil
		},
		Modules:         moduleStateMachines{DB: state.DB, states: make(map[string]*moduleStateMachineState)},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return config.ReuseCred, nil },
		VerifyVoucher: func(_ context.Context, voucher fdo.Voucher) error {
			return handlers.VerifyVoucher(&voucher, []crypto.PublicKey{state.ownerKey.Public()})
		},
	}

	handler := &transport.Handler{
		Tokens:       state.DB,
		TO2Responder: to2Server,
	}

	// Handle messages
	apiRouter := http.NewServeMux()
	apiRouter.Handle("GET /to0/{guid}", handlers.To0Handler(&handlers.To0HandlerState{
		VoucherState: state.DB,
		KeyState:     state,
		UseTLS:       config.HTTP.UseTLS,
	}))
	apiRouter.Handle("POST /owner/vouchers", handlers.InsertVoucherHandler([]crypto.PublicKey{state.ownerKey.Public()}))
	apiRouter.HandleFunc("/owner/redirect", handlers.OwnerInfoHandler)
	apiRouter.Handle("POST /owner/resell/{guid}", handlers.ResellHandler(to2Server))
	httpHandler := api.NewHTTPHandler(handler, state.DB.DB).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewOwnerServer(config.HTTP, config.ExternalAddress, httpHandler)

	slog.Debug("Starting server on:", "addr", config.HTTP.Listen)
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
			for _, name := range downloads {
				f, err := os.Open(filepath.Clean(name))
				if err != nil {
					log.Fatalf("error opening %q for download FSIM: %v", name, err)
				}
				defer func() { _ = f.Close() }()

				if !yield("fdo.download", &fsim.DownloadContents[*os.File]{
					Name:         name,
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
			for _, urlString := range wgets {
				url, err := url.Parse(urlString)
				if err != nil || url.Path == "" {
					continue
				}
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

	// Declare any CLI flags for overriding configuration file settings and bind
	// them into the proper fields of the configuration structure

	ownerCmd.Flags().Bool("reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
	ownerCmd.Flags().String("device-ca-cert", "", "Device CA certificate path")
	ownerCmd.Flags().String("owner-key", "", "Owner private key path")
	ownerCmd.Flags().String("external-address", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	if err := viper.BindPFlag("owner.reuse-credentials", ownerCmd.Flags().Lookup("reuse-credentials")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("owner.device-ca-cert", ownerCmd.Flags().Lookup("device-ca-cert")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("owner.owner-key", ownerCmd.Flags().Lookup("owner-key")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("owner.external-address", ownerCmd.Flags().Lookup("external-address")); err != nil {
		panic(err)
	}

	if err := addDatabaseConfig(ownerCmd, "owner.database"); err != nil {
		panic(err)
	}
	if err := addHTTPConfig(ownerCmd, "owner.http"); err != nil {
		panic(err)
	}
}

func init() {
	ownerCmdInit()
}
