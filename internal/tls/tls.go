// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package tls

import (
	"crypto/tls"
	"net"
	net_http "net/http"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-server/internal/version"
	"github.com/fido-device-onboard/go-fdo/http"
)

func TlsTransport(baseURL string, conf *tls.Config, insecureTLS bool) fdo.Transport {

	preferredCipherSuites := []uint16{
		tls.TLS_AES_256_GCM_SHA384,                  // TLS v1.3
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS v1.2
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS v1.2
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS v1.2
	}

	if conf == nil {
		conf = &tls.Config{
			CipherSuites:       preferredCipherSuites,
			InsecureSkipVerify: insecureTLS, //nolint:gosec
		}
	}

	return &http.Transport{
		BaseURL: baseURL,
		Client: &net_http.Client{Transport: &userAgentTransport{&net_http.Transport{
			Proxy: net_http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSClientConfig:       conf,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}, "go-fdo-server/" + version.VERSION}},
	}
}

type userAgentTransport struct {
	next      net_http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *net_http.Request) (*net_http.Response, error) {
	r := req.Clone(req.Context())
	if r.Header == nil {
		r.Header = make(net_http.Header)
	}
	r.Header.Set("User-Agent", t.userAgent)
	return t.next.RoundTrip(r)
}
