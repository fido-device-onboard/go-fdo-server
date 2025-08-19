/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
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
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/fsim"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
)

var (
	date                bool
	wgets               []string
	uploads             []string
	uploadDir           string
	downloads           []string
	ownerDeviceCertPath string
	ownerPrivateKeyPath string
	reuseCred           bool
)

// serveCmd represents the serve command
var ownerCmd = &cobra.Command{
	Use:   "owner http_address",
	Short: "Serve an instance of the HTTP server for the role",
	Long: `Serve runs the HTTP server for the FDO protocol. It can act as all the three
	main servers in the FDO spec.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		address = args[0]
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := getState()
		if err != nil {
			return err
		}

		if externalAddress == "" {
			externalAddress = address
		}

		// host, portStr, err := net.SplitHostPort(externalAddress)
		// if err != nil {
		// 	return fmt.Errorf("invalid external addr: %w", err)
		// }

		// portNum, err := strconv.ParseUint(portStr, 10, 16)
		// if err != nil {
		// 	return fmt.Errorf("invalid external port: %w", err)
		// }
		// port := uint16(portNum)

		err = db.InitDb(state)
		if err != nil {
			return err
		}

		return serveOwner(state, insecureTLS)
	},
}

// Server represents the HTTP server
type OwnerServer struct {
	addr    string
	extAddr string
	handler http.Handler
	useTLS  bool
}

// NewServer creates a new Server
func NewOwnerServer(addr string, extAddr string, handler http.Handler, useTLS bool) *OwnerServer {
	return &OwnerServer{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS}
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

type OwnerServerState struct {
	DB           *sqlite.DB
	ownerKey     crypto.Signer
	ownerKeyType protocol.KeyType
	chain        []*x509.Certificate
}

func serveOwner(db *sqlite.DB, useTLS bool) error {
	state := &OwnerServerState{
		DB: db,
	}
	okey, keyType, err := parsePrivateKey(ownerPrivateKeyPath)
	if err != nil {
		return err
	}
	state.ownerKey = okey
	state.ownerKeyType = keyType
	c, err := os.ReadFile(ownerDeviceCertPath)
	if err != nil {
		return err
	}
	blk, _ := pem.Decode(c)
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return err
	}
	state.chain = []*x509.Certificate{cert}

	to2Server := &fdo.TO2Server{
		Session:         state.DB,
		Vouchers:        state.DB,
		OwnerKeys:       state,
		RvInfo:          func(context.Context, fdo.Voucher) ([][]protocol.RvInstruction, error) { return rvinfo.FetchRvInfo() },
		Modules:         moduleStateMachines{DB: state.DB, states: make(map[string]*moduleStateMachineState)},
		ReuseCredential: func(context.Context, fdo.Voucher) (bool, error) { return reuseCred, nil },
	}

	handler := &transport.Handler{
		Tokens:       state.DB,
		TO2Responder: to2Server,
	}

	// Handle messages
	apiRouter := http.NewServeMux()
	apiRouter.Handle("GET /to0/{guid}", handlers.To0Handler(db, state, useTLS))
	apiRouter.Handle("POST /owner/vouchers", handlers.InsertVoucherHandler([]crypto.PublicKey{okey.Public()}))
	apiRouter.HandleFunc("/owner/redirect", handlers.OwnerInfoHandler)
	apiRouter.Handle("POST /owner/resell/{guid}", resellHandler(to2Server))
	httpHandler := api.NewHTTPHandler(handler, state.DB).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewOwnerServer(address, externalAddress, httpHandler, useTLS)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

// TODO: move this to handlers
func resellHandler(to2Server *fdo.TO2Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidHex := r.PathValue("guid")

		if !utils.IsValidGUID(guidHex) {
			http.Error(w, "GUID is not a valid GUID", http.StatusBadRequest)
			return
		}

		guidBytes, err := hex.DecodeString(guidHex)
		if err != nil {
			http.Error(w, "Invalid GUID format", http.StatusBadRequest)
			slog.Debug(err.Error())
			return
		}

		var guid protocol.GUID
		copy(guid[:], guidBytes)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failure to read the request body", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}
		blk, _ := pem.Decode(body)
		if blk == nil {
			http.Error(w, "Invalid PEM content", http.StatusInternalServerError)
			return
		}
		nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
		if err != nil {
			http.Error(w, "Error parsing x.509 public key", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		extended, err := to2Server.Resell(context.TODO(), guid, nextOwner, nil)
		if err != nil {
			http.Error(w, "Error reselling voucher", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}
		ovBytes, err := cbor.Marshal(extended)
		if err != nil {
			http.Error(w, "Error marshaling voucher", http.StatusInternalServerError)
			slog.Debug(err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/x-pem-file")
		if err := pem.Encode(w, &pem.Block{
			Type:  "OWNERSHIP VOUCHER",
			Bytes: ovBytes,
		}); err != nil {
			slog.Debug("Error encoding voucher", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (state *OwnerServerState) OwnerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	return state.ownerKey, state.chain, nil
}

type moduleStateMachines struct {
	DB *sqlite.DB
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

func init() {
	serveCmd.AddCommand(ownerCmd)

	//serveCmd.Flags().StringVar(&externalAddress, "external-address", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	ownerCmd.Flags().BoolVar(&date, "command-date", false, "Use fdo.command FSIM to have device run \"date --utc\"")
	ownerCmd.Flags().StringArrayVar(&wgets, "command-wget", nil, "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")
	ownerCmd.Flags().StringArrayVar(&uploads, "command-upload", nil, "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	ownerCmd.Flags().StringVar(&uploadDir, "upload-directory", "", "The directory `path` to put file uploads")
	ownerCmd.Flags().StringArrayVar(&downloads, "command-download", nil, "Use fdo.download FSIM for each `file` (flag may be used multiple times)")
	serveCmd.Flags().BoolVar(&reuseCred, "reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
	ownerCmd.Flags().StringVar(&ownerDeviceCertPath, "device-cert", "", "t3")
	ownerCmd.Flags().StringVar(&ownerPrivateKeyPath, "owner-key", "", "t4")
}
