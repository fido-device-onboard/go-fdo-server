/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/api/handlers"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo/custom"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/sqlite"
	"github.com/spf13/cobra"
)

var (
	address              string
	externalAddress      string
	manufacturingKey     string
	manufacturingKeyType string
	deviceCert           string
	ownerKey             string
	rsaPss               bool
)

// serveCmd represents the serve command
var manufacturingCmd = &cobra.Command{
	Use:   "manufacturing http_address",
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

		err = db.InitDb(state)
		if err != nil {
			return err
		}

		// Retrieve RV info from DB
		rvInfo, err := rvinfo.FetchRvInfo()
		if err != nil {
			return err
		}

		return serveManufacturing(rvInfo, state, insecureTLS)
	},
}

// Server represents the HTTP server
type ManufacturingServer struct {
	addr    string
	extAddr string
	handler http.Handler
	useTLS  bool
	state   *sqlite.DB
}

// NewServer creates a new Server
func NewManufacturingServer(addr string, extAddr string, handler http.Handler, useTLS bool, state *sqlite.DB) *ManufacturingServer {
	return &ManufacturingServer{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS, state: state}
}

// Start starts the HTTP server
func (s *ManufacturingServer) Start() error {
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
			// TODO(runcom): drop this shit...., no certs from db, just config/cli is sane to me
			cert, err := tlsCert(s.state.DB())
			if err != nil {
				return err
			}
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{*cert},
				CipherSuites: preferredCipherSuites,
			}
			return srv.ServeTLS(lis, "", "")

		}
	}
	return srv.Serve(lis)
}

type ManufacturingServerState struct {
	RvInfo [][]protocol.RvInstruction
	DB     *sqlite.DB
}

func serveManufacturing(rvInfo [][]protocol.RvInstruction, db *sqlite.DB, useTLS bool) error {
	state := &ManufacturingServerState{
		RvInfo: rvInfo,
		DB:     db,
	}
	// Create FDO responder
	handler, err := newManufacturingHandler(state)
	if err != nil {
		return err
	}

	// Handle messages
	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc("GET /vouchers", handlers.GetVoucherHandler)
	apiRouter.Handle("/rvinfo", handlers.RvInfoHandler(&rvInfo))
	httpHandler := api.NewHTTPHandler(handler, &state.RvInfo, state.DB).RegisterRoutes(apiRouter)

	// Listen and serve
	server := NewManufacturingServer(address, externalAddress, httpHandler, useTLS, state.DB)

	slog.Debug("Starting server on:", "addr", address)
	return server.Start()
}

type SingleOwnerManufacturer struct {
	state     *sqlite.DB
	nextOwner crypto.PublicKey
	chain     []*x509.Certificate
	mfgKey    crypto.Signer
	keyType   protocol.KeyType
}

// ManufacturerKey returns the signer of a given key type and its certificate
// chain (required). If key type is not RSAPKCS or RSAPSS then rsaBits is
// ignored. Otherwise it must be either 2048 or 3072.
func (som *SingleOwnerManufacturer) ManufacturerKey(ctx context.Context, keyType protocol.KeyType, rsaBits int) (crypto.Signer, []*x509.Certificate, error) {
	// TODO: check key types are the same as the one asked
	return som.mfgKey, som.chain, nil
}

func (som *SingleOwnerManufacturer) Extend(ctx context.Context, ov *fdo.Voucher) error {
	mfgKey := ov.Header.Val.ManufacturerKey
	keyType, rsaBits := mfgKey.Type, mfgKey.RsaBits()
	owner, _, err := som.ManufacturerKey(ctx, keyType, rsaBits)
	if err != nil {
		return fmt.Errorf("auto extend: error getting %s manufacturer key: %w", keyType, err)
	}
	switch som.nextOwner.(type) {
	case *ecdsa.PublicKey:
		nextOwner, ok := som.nextOwner.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("auto extend: owner key must be %s", keyType)
		}
		extended, err := fdo.ExtendVoucher(ov, owner, nextOwner, nil)
		if err != nil {
			return err
		}
		*ov = *extended
		return nil

	case *rsa.PublicKey:
		nextOwner, ok := som.nextOwner.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("auto extend: owner key must be %s", keyType)
		}
		extended, err := fdo.ExtendVoucher(ov, owner, nextOwner, nil)
		if err != nil {
			return err
		}
		*ov = *extended
		return nil

	default:
		return fmt.Errorf("auto extend: invalid key type %T", owner)
	}
}

func getPrivateKeyType(key any) (protocol.KeyType, error) {
	switch ktype := key.(type) {
	case *rsa.PrivateKey:
		switch ktype.N.BitLen() {
		case 2048:
			return protocol.Rsa2048RestrKeyType, nil
		case 3072:
			// TODO: rsaPss should be an additional key path (right?)
			if rsaPss {
				return protocol.RsaPssKeyType, nil
			} else {
				return protocol.RsaPkcsKeyType, nil
			}
		}
	case *ecdsa.PrivateKey:
		switch ktype.Curve.Params().BitSize {
		case 256:
			return protocol.Secp256r1KeyType, nil
		case 384:
			return protocol.Secp384r1KeyType, nil
		}
	}
	return 0, fmt.Errorf("unsupported key provided")
}

// https://github.com/golang/go/issues/43780
// TODO: this is shared between roles, move to a better place
func parsePrivateKey(keyPath string) (crypto.Signer, protocol.KeyType, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, 0, err
	}
	key, err := x509.ParsePKCS8PrivateKey(b)
	if err == nil {
		keyType, err := getPrivateKeyType(key)
		if err != nil {
			return nil, 0, err
		}
		return key.(crypto.Signer), keyType, nil
	}
	if strings.Contains(err.Error(), "ParseECPrivateKey") {
		key, err = x509.ParseECPrivateKey(b)
		if err != nil {
			return nil, 0, err
		}
		keyType, err := getPrivateKeyType(key)
		if err != nil {
			return nil, 0, err
		}
		return key.(crypto.Signer), keyType, nil
	}
	if strings.Contains(err.Error(), "ParsePKCS1PrivateKey") {
		key, err = x509.ParsePKCS1PrivateKey(b)
		if err != nil {
			return nil, 0, err
		}
		keyType, err := getPrivateKeyType(key)
		if err != nil {
			return nil, 0, err
		}
		return key.(crypto.Signer), keyType, nil
	}
	return nil, 0, fmt.Errorf("unable to parse private key %s", keyPath)
}

//nolint:gocyclo
func newManufacturingHandler(state *ManufacturingServerState) (*transport.Handler, error) {
	key, keyType, err := parsePrivateKey(manufacturingKey)
	if err != nil {
		return nil, err
	}
	c, err := os.ReadFile(deviceCert)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(c)
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}
	o, err := os.ReadFile(ownerKey)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(o))
	if block == nil {
		return nil, fmt.Errorf("unable to decode owner public key")
	}
	var ownerCert *x509.Certificate
	ownerCert, _ = x509.ParseCertificate(block.Bytes)
	som := &SingleOwnerManufacturer{
		state:     state.DB,
		nextOwner: ownerCert.PublicKey.(crypto.PublicKey),
		chain:     []*x509.Certificate{cert},
		mfgKey:    key,
		keyType:   keyType,
	}

	return &transport.Handler{
		Tokens: state.DB,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               state.DB,
			Vouchers:              state.DB,
			SignDeviceCertificate: custom.SignDeviceCertificate(som),
			DeviceInfo: func(_ context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.KeyType, protocol.KeyEncoding, error) {
				return info.DeviceInfo, info.KeyType, info.KeyEncoding, nil
			},
			BeforeVoucherPersist: som.Extend,
			RvInfo:               func(context.Context, *fdo.Voucher) ([][]protocol.RvInstruction, error) { return rvinfo.FetchRvInfo() },
		},
	}, nil
}

// TODO(runcom): move to a more agnostic place
func tlsCert(db *sql.DB) (*tls.Certificate, error) {
	// Ensure that the https table exists
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS https
		( cert BLOB NOT NULL
		, key BLOB NOT NULL
		)`); err != nil {
		return nil, err
	}

	// Load a TLS cert and key from the database
	row := db.QueryRow("SELECT cert, key FROM https LIMIT 1")
	var certDer, keyDer []byte
	if err := row.Scan(&certDer, &keyDer); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if len(keyDer) > 0 {
		key, err := x509.ParsePKCS8PrivateKey(keyDer)
		if err != nil {
			return nil, fmt.Errorf("bad HTTPS key stored: %w", err)
		}
		return &tls.Certificate{
			Certificate: [][]byte{certDer},
			PrivateKey:  key,
		}, nil
	}

	// TODO(runcom)
	// remove this stuff, certs have to be passed from CLI/config
	// Generate a new self-signed TLS CA
	tlsKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(30 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, tlsKey.Public(), tlsKey)
	if err != nil {
		return nil, err
	}
	tlsCA, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}

	// Store TLS cert and key to the database
	keyDER, err := x509.MarshalPKCS8PrivateKey(tlsKey)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("INSERT INTO https (cert, key) VALUES (?, ?)", caDER, keyDER); err != nil {
		return nil, err
	}

	// Use CA to serve TLS
	return &tls.Certificate{
		Certificate: [][]byte{tlsCA.Raw},
		PrivateKey:  tlsKey,
	}, nil
}

func init() {
	serveCmd.AddCommand(manufacturingCmd)

	manufacturingCmd.Flags().StringVar(&externalAddress, "external-address", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	manufacturingCmd.Flags().StringVar(&manufacturingKey, "manufacturing-key", "", "test 1")
	manufacturingCmd.Flags().StringVar(&manufacturingKeyType, "manufacturing-key-type", "", "test 2") // TODO: ???
	manufacturingCmd.Flags().StringVar(&deviceCert, "device-cert", "", "t3")
	manufacturingCmd.Flags().StringVar(&ownerKey, "owner-key", "", "t4")
	manufacturingCmd.Flags().BoolVar(&rsaPss, "rsa-pss", false, "")
}
