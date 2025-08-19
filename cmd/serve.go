// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"github.com/spf13/cobra"
)

var (
	insecureTLS    bool
	serverCertPath string
	serverKeyPath  string
)

var serveCmd = &cobra.Command{
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	// TODO(runcom)
	Use:   "serve <role>",
	Short: "Server implementation of FIDO Device Onboard specification in Go",
	Long: `Server implementation of the three main FDO servers. It can act
	as a Manufacturer, Owner and Rendezvous.

	The server also provides APIs to interact with the various servers implementations.
`,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.PersistentFlags().BoolVar(&insecureTLS, "insecure-tls", false, "Listen with a self-signed TLS certificate")
	serveCmd.PersistentFlags().StringVar(&serverCertPath, "server-cert-path", "", "Path to server certificate")
	serveCmd.PersistentFlags().StringVar(&serverKeyPath, "server-key-path", "", "Path to server private key")
}
