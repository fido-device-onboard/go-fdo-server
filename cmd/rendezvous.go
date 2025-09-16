// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rendezvousCmd represents the rendezvous command
var rendezvousCmd = &cobra.Command{
	Use:   "rendezvous http_address",
	Short: "Serve an instance of the rendezvous server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration first
		if err := rendezvousCmdLoadConfig(cmd, args); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := getState()
		if err != nil {
			return err
		}

		err = db.InitDb(state)
		if err != nil {
			return err
		}

		return serveRendezvous(state, insecureTLS)
	},
}

// Server represents the HTTP server
type RendezvousServer struct {
	addr    string
	extAddr string
	handler http.Handler
	useTLS  bool
}

// NewServer creates a new Server
func NewRendezvousServer(addr string, extAddr string, handler http.Handler, useTLS bool) *RendezvousServer {
	return &RendezvousServer{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS}
}

// Start starts the HTTP server
func (s *RendezvousServer) Start() error {
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
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer func() { _ = lis.Close() }()
	slog.Info("Listening", "local", lis.Addr().String(), "external", s.extAddr)

	if s.useTLS {
		preferredCipherSuites := []uint16{
			tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
		}

		if serverCertPath != "" && serverKeyPath != "" {
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, serverCertPath, serverKeyPath)
		} else {
			return fmt.Errorf("no TLS cert or key provided")
		}
	}
	return srv.Serve(lis)
}

type RendezvousServerState struct {
	DB *sqlite.DB
}

func serveRendezvous(db *sqlite.DB, useTLS bool) error {
	state := &RendezvousServerState{
		DB: db,
	}
	// Create FDO responder
	handler := &transport.Handler{
		Tokens: state.DB,
		TO0Responder: &fdo.TO0Server{
			Session: state.DB,
			RVBlobs: state.DB,
		},
		TO1Responder: &fdo.TO1Server{
			Session: state.DB,
			RVBlobs: state.DB,
		}}

	httpHandler := api.NewHTTPHandler(handler, state.DB).RegisterRoutes(nil)

	// Listen and serve
	server := NewRendezvousServer(address, externalAddress, httpHandler, useTLS)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

func init() {
	rootCmd.AddCommand(rendezvousCmd)

	rendezvousCmd.Flags().String("config", "", "Pathname of the configuration file")
}

// Load configuration from viper
func rendezvousCmdLoadConfig(cmd *cobra.Command, args []string) error {
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
	}

	// If the http_address has been provided on the command line it
	// will take precedence over the content of the configuration
	// file. Yet viper has no visibility of the command line so we
	// force it here:
	if len(args) > 0 {
		viper.Set("address", args[0])
	}

	// Get the config flag directly from the command
	configFilePath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("failed to get config flag: %w", err)
	}

	if configFilePath != "" {
		slog.Debug("Loading rendezvous server configuration file", "path", configFilePath)
		viper.SetConfigFile(configFilePath)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("configuration file read failed: %w", err)
		}
	}

	// Load root configuration after reading config file
	if err := rootCmdLoadConfig(); err != nil {
		return err
	}

	address = viper.GetString("address")

	if address == "" {
		return fmt.Errorf("the rendezvous command requires the 'http_address' argument")
	}

	return nil
}
