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
	// TODO(runcom): bind to viper
	serveCmd.PersistentFlags().BoolVar(&insecureTLS, "insecure-tls", false, "Listen with a self-signed TLS certificate")
	serveCmd.PersistentFlags().StringVar(&serverCertPath, "server-cert-path", "", "Path to server certificate")
	serveCmd.PersistentFlags().StringVar(&serverKeyPath, "server-key-path", "", "Path to server private key")
	// serveCmd.Flags().StringVar(&externalAddress, "external-address", "", "External `addr`ess devices should connect to (default \"127.0.0.1:${LISTEN_PORT}\")")
	// serveCmd.Flags().BoolVar(&date, "command-date", false, "Use fdo.command FSIM to have device run \"date --utc\"")
	// serveCmd.Flags().StringArrayVar(&wgets, "command-wget", nil, "Use fdo.wget FSIM for each `url` (flag may be used multiple times)")
	// serveCmd.Flags().StringArrayVar(&uploads, "command-upload", nil, "Use fdo.upload FSIM for each `file` (flag may be used multiple times)")
	// serveCmd.Flags().StringVar(&uploadDir, "upload-directory", "", "The directory `path` to put file uploads")
	// serveCmd.Flags().StringArrayVar(&downloads, "command-download", nil, "Use fdo.download FSIM for each `file` (flag may be used multiple times)")
	// serveCmd.Flags().BoolVar(&reuseCred, "reuse-credentials", false, "Perform the Credential Reuse Protocol in TO2")
}
