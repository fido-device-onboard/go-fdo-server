// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

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
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/api"
	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/ownerinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/rvinfo"
	"github.com/fido-device-onboard/go-fdo-server/internal/to0"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/custom"
	"github.com/fido-device-onboard/go-fdo/fsim"
	transport "github.com/fido-device-onboard/go-fdo/http"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"github.com/fido-device-onboard/go-fdo/sqlite"
)

var serverFlags = flag.NewFlagSet("server", flag.ContinueOnError)

var (
	useTLS           bool
	addr             string
	dbPath           string
	dbPass           string
	extAddr          string
	resaleGUID       string
	resaleKey        string
	reuseCred        bool
	rvBypass         bool
	downloads        stringList
	uploadDir        string
	uploadReqs       stringList
	insecureTLS      bool
	serverCertPath   string
	serverKeyPath    string
	printOwnerPubKey string
	importVoucher    string
	wgets            stringList
)

type stringList []string

func (list *stringList) Set(v string) error {
	*list = append(*list, v)
	return nil
}

func (list *stringList) String() string {
	return fmt.Sprintf("[%s]", strings.Join(*list, ","))
}

func init() {
	serverFlags.StringVar(&dbPath, "db", "", "SQLite database file path")
	serverFlags.StringVar(&dbPass, "db-pass", "", "SQLite database encryption-at-rest passphrase")
	serverFlags.BoolVar(&debug, "debug", debug, "Print HTTP contents")
	serverFlags.StringVar(&extAddr, "ext-http", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	serverFlags.StringVar(&addr, "http", "localhost:8080", "The `addr`ess to listen on")
	serverFlags.StringVar(&resaleGUID, "resale-guid", "", "Voucher `guid` to extend for resale")
	serverFlags.StringVar(&resaleKey, "resale-key", "", "The `path` to a PEM-encoded x.509 public key for the next owner")
	serverFlags.BoolVar(&reuseCred, "reuse-cred", false, "Perform the Credential Reuse Protocol in TO2")
	serverFlags.BoolVar(&insecureTLS, "insecure-tls", false, "Listen with a self-signed TLS certificate")
	serverFlags.StringVar(&serverCertPath, "server-cert", "", "Path to server certificate")
	serverFlags.StringVar(&serverKeyPath, "server-key", "", "Path to server private key")
	serverFlags.StringVar(&printOwnerPubKey, "print-owner-public", "", "Print owner public key of `type` and exit")
	serverFlags.StringVar(&importVoucher, "import-voucher", "", "Import a PEM encoded voucher file at `path`")
	serverFlags.Var(&downloads, "download", "Use fdo.download FSIM for each `file` (flag may be used multiple times)")
	serverFlags.StringVar(&uploadDir, "upload-dir", "uploads", "The directory `path` to put file uploads")
	serverFlags.Var(&uploadReqs, "upload", "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	serverFlags.Var(&wgets, "wget", "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")

}

// Server represents the HTTP server
type Server struct {
	addr    string
	extAddr string
	handler http.Handler
	useTLS  bool
	state   *sqlite.DB
}

// NewServer creates a new Server
func NewServer(addr string, extAddr string, handler http.Handler, useTLS bool, state *sqlite.DB) *Server {
	return &Server{addr: addr, extAddr: extAddr, handler: handler, useTLS: useTLS, state: state}
}

// Start starts the HTTP server
func (s *Server) Start() error {
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
		if serverCertPath != "" && serverKeyPath != "" {
			srv.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
			return srv.ServeTLS(lis, serverCertPath, serverKeyPath)
		} else {
			cert, err := tlsCert(s.state.DB())
			if err != nil {
				return err
			}
			srv.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{*cert},
			}
			return srv.ServeTLS(lis, "", "")

		}
	}
	return srv.Serve(lis)
}

func server() error { //nolint:gocyclo
	if debug {
		level.Set(slog.LevelDebug)
	}

	if dbPath == "" {
		return errors.New("db flag is required")
	}
	state, err := sqlite.New(dbPath, dbPass)
	if err != nil {
		return err
	}
	// If printing owner public key, do so and exit
	if printOwnerPubKey != "" {
		return doPrintOwnerPubKey(state)
	}

	// If importing a voucher, do so and exit
	if importVoucher != "" {
		return doImportVoucher(state)
	}
	useTLS = insecureTLS

	if extAddr == "" {
		extAddr = addr
	}

	host, portStr, err := net.SplitHostPort(extAddr)
	if err != nil {
		return fmt.Errorf("invalid external addr: %w", err)
	}

	portNum, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid external port: %w", err)
	}
	port := uint16(portNum)

	err = db.InitDb(state)
	if err != nil {
		return err
	}

	// set tls for TO0
	to0.SetTo0Tls(useTLS)

	// Retrieve RV info from DB
	rvInfo, err := rvinfo.FetchRvInfo()
	if err != nil {
		return err
	}

	if rvInfo != nil {
		rvBypass = rvinfo.HasRVBypass(rvInfo)
	} else {
		rvBypass = false
	}

	// CreateRvInfo initializes new RV info if not found in DB
	if rvInfo == nil {
		rvInfo, err = rvinfo.CreateRvInfo(useTLS, host, port)
		if err != nil {
			return err
		}
	}

	// CreateRvTO2Addr initializes new owner info and stores it with default values if not found in DB
	err = ownerinfo.CreateRvTO2Addr(host, port, useTLS)
	if err != nil {
		return fmt.Errorf("failed to create and store rvTO2Addrs: %v", err)
	}

	// Invoke resale protocol if a GUID is specified
	if resaleGUID != "" {
		return resell(state)
	}

	return serveHTTP(rvInfo, state)
}

type ServerState struct {
	RvInfo [][]protocol.RvInstruction
	DB     *sqlite.DB
}

func serveHTTP(rvInfo [][]protocol.RvInstruction, db *sqlite.DB) error {
	state := &ServerState{
		RvInfo: rvInfo,
		DB:     db,
	}
	// Create FDO responder
	handler, err := newHandler(state)
	if err != nil {
		return err
	}

	// Handle messages
	httpHandler := api.NewHTTPHandler(handler, &state.RvInfo, state.DB).RegisterRoutes()
	// Listen and serve
	server := NewServer(addr, extAddr, httpHandler, useTLS, state.DB)

	slog.Debug("Starting server on:", "addr", addr)
	return server.Start()

}

func doPrintOwnerPubKey(state *sqlite.DB) error {
	keyType, err := protocol.ParseKeyType(printOwnerPubKey)
	if err != nil {
		return fmt.Errorf("%w: see usage", err)
	}
	key, _, err := state.OwnerKey(keyType)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return err
	}
	return pem.Encode(os.Stdout, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
}

func doImportVoucher(state *sqlite.DB) error {
	// Parse voucher
	pemVoucher, err := os.ReadFile(filepath.Clean(importVoucher))
	if err != nil {
		return err
	}
	blk, _ := pem.Decode(pemVoucher)
	if blk == nil {
		return fmt.Errorf("invalid PEM encoded file: %s", importVoucher)
	}
	if blk.Type != "OWNERSHIP VOUCHER" {
		return fmt.Errorf("expected PEM block of ownership voucher type, found %s", blk.Type)
	}
	var ov fdo.Voucher
	if err := cbor.Unmarshal(blk.Bytes, &ov); err != nil {
		return fmt.Errorf("error parsing voucher: %w", err)
	}

	// Check that voucher owner key matches
	expectedPubKey, err := ov.OwnerPublicKey()
	if err != nil {
		return fmt.Errorf("error parsing owner public key from voucher: %w", err)
	}
	ownerKey, _, err := state.OwnerKey(ov.Header.Val.ManufacturerKey.Type)
	if err != nil {
		return fmt.Errorf("error getting owner key: %w", err)
	}
	if !ownerKey.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(expectedPubKey) {
		return fmt.Errorf("owner key in database does not match the owner of the voucher")
	}

	// Store voucher
	return state.AddVoucher(context.Background(), &ov)
}

func resell(state *sqlite.DB) error {
	// Parse resale-guid flag
	guidBytes, err := hex.DecodeString(strings.ReplaceAll(resaleGUID, "-", ""))
	if err != nil {
		return fmt.Errorf("error parsing GUID of voucher to resell: %w", err)
	}
	if len(guidBytes) != 16 {
		return fmt.Errorf("error parsing GUID of voucher to resell: must be 16 bytes")
	}
	var guid protocol.GUID
	copy(guid[:], guidBytes)

	// Parse next owner key
	if resaleKey == "" {
		return fmt.Errorf("resale-guid depends on resale-key flag being set")
	}
	keyBytes, err := os.ReadFile(filepath.Clean(resaleKey))
	if err != nil {
		return fmt.Errorf("error reading next owner key file: %w", err)
	}
	blk, _ := pem.Decode(keyBytes)
	if blk == nil {
		return fmt.Errorf("invalid PEM file: %s", resaleKey)
	}
	nextOwner, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return fmt.Errorf("error parsing x.509 public key: %w", err)
	}

	// Perform resale protocol
	extended, err := (&fdo.TO2Server{
		Vouchers:  state,
		OwnerKeys: state,
	}).Resell(context.TODO(), guid, nextOwner, nil)
	if err != nil {
		return fmt.Errorf("resale protocol: %w", err)
	}
	ovBytes, err := cbor.Marshal(extended)
	if err != nil {
		return fmt.Errorf("resale protocol: error marshaling voucher: %w", err)
	}
	return pem.Encode(os.Stdout, &pem.Block{
		Type:  "OWNERSHIP VOUCHER",
		Bytes: ovBytes,
	})
}

//nolint:gocyclo
func newHandler(state *ServerState) (*transport.Handler, error) {
	// Generate manufacturing component keys
	rsa2048MfgKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	rsa3072MfgKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	ec256MfgKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ec384MfgKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	generateCA := func(key crypto.Signer) ([]*x509.Certificate, error) {
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(30 * 365 * 24 * time.Hour),
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		return []*x509.Certificate{cert}, nil
	}
	rsa2048Chain, err := generateCA(rsa2048MfgKey)
	if err != nil {
		return nil, err
	}
	rsa3072Chain, err := generateCA(rsa3072MfgKey)
	if err != nil {
		return nil, err
	}
	ec256Chain, err := generateCA(ec256MfgKey)
	if err != nil {
		return nil, err
	}
	ec384Chain, err := generateCA(ec384MfgKey)
	if err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Rsa2048RestrKeyType, rsa2048MfgKey, rsa2048Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.RsaPkcsKeyType, rsa3072MfgKey, rsa3072Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.RsaPssKeyType, rsa3072MfgKey, rsa3072Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Secp256r1KeyType, ec256MfgKey, ec256Chain); err != nil {
		return nil, err
	}
	if err := state.DB.AddManufacturerKey(protocol.Secp384r1KeyType, ec384MfgKey, ec384Chain); err != nil {
		return nil, err
	}

	// Generate owner keys
	rsa2048OwnerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	rsa3072OwnerKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	ec256OwnerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ec384OwnerKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Rsa2048RestrKeyType, rsa2048OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.RsaPkcsKeyType, rsa3072OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.RsaPssKeyType, rsa3072OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Secp256r1KeyType, ec256OwnerKey, nil); err != nil {
		return nil, err
	}
	if err := state.DB.AddOwnerKey(protocol.Secp384r1KeyType, ec384OwnerKey, nil); err != nil {
		return nil, err
	}

	// Auto-register RV blob so that TO1 can be tested unless a TO0 address is
	// given or RV bypass is set
	var autoTO0 fdo.AutoTO0
	var autoTO0Addrs []protocol.RvTO2Addr
	if !rvBypass {
		autoTO0 = state.DB

		for _, directive := range protocol.ParseDeviceRvInfo(state.RvInfo) {
			if directive.Bypass {
				continue
			}

			for _, url := range directive.URLs {
				to1Host := url.Hostname()
				to1Port, err := strconv.ParseUint(url.Port(), 10, 16)
				if err != nil {
					return nil, fmt.Errorf("error parsing TO1 port to use for TO2: %w", err)
				}
				proto := protocol.HTTPTransport
				if useTLS {
					proto = protocol.HTTPSTransport
				}
				autoTO0Addrs = append(autoTO0Addrs, protocol.RvTO2Addr{
					DNSAddress:        &to1Host,
					Port:              uint16(to1Port),
					TransportProtocol: proto,
				})
			}
		}
	}

	return &transport.Handler{
		Tokens: state.DB,
		DIResponder: &fdo.DIServer[custom.DeviceMfgInfo]{
			Session:               state.DB,
			Vouchers:              state.DB,
			SignDeviceCertificate: custom.SignDeviceCertificate(state.DB),
			DeviceInfo: func(_ context.Context, info *custom.DeviceMfgInfo, _ []*x509.Certificate) (string, protocol.KeyType, protocol.KeyEncoding, error) {
				return info.DeviceInfo, info.KeyType, info.KeyEncoding, nil
			},
			AutoExtend:   state.DB,
			AutoTO0:      autoTO0,
			AutoTO0Addrs: autoTO0Addrs,
			RvInfo:       func(context.Context, *fdo.Voucher) ([][]protocol.RvInstruction, error) { return state.RvInfo, nil },
		},
		TO0Responder: &fdo.TO0Server{
			Session: state.DB,
			RVBlobs: state.DB,
		},
		TO1Responder: &fdo.TO1Server{
			Session: state.DB,
			RVBlobs: state.DB,
		},
		TO2Responder: &fdo.TO2Server{
			Session:         state.DB,
			Vouchers:        state.DB,
			OwnerKeys:       state.DB,
			RvInfo:          func(context.Context, fdo.Voucher) ([][]protocol.RvInstruction, error) { return state.RvInfo, nil },
			OwnerModules:    ownerModules,
			ReuseCredential: func(context.Context, fdo.Voucher) bool { return reuseCred },
		},
	}, nil
}

func ownerModules(ctx context.Context, guid protocol.GUID, info string, chain []*x509.Certificate, devmod serviceinfo.Devmod, modules []string) iter.Seq2[string, serviceinfo.OwnerModule] {
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
			for _, name := range uploadReqs {
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
	}
}

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
