// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/fido-device-onboard/go-fdo-server/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	format := flag.String("format", "man", "Output format: man or markdown")
	out := flag.String("out", "", "Output directory (default: docs/man for man, docs/cli for markdown)")
	flag.Parse()

	root := cmd.Root()
	disableAutoGenTag(root)

	var outDir string
	switch *format {
	case "man":
		outDir = "docs/man"
		if *out != "" {
			outDir = *out
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatalf("failed to create output directory: %v", err)
		}
		header := &doc.GenManHeader{
			Title:   "GO-FDO-SERVER",
			Section: "1",
		}
		if err := doc.GenManTree(root, header, outDir); err != nil {
			log.Fatalf("failed to generate man pages: %v", err)
		}
	case "markdown":
		outDir = "docs/cli"
		if *out != "" {
			outDir = *out
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Fatalf("failed to create output directory: %v", err)
		}
		if err := doc.GenMarkdownTree(root, outDir); err != nil {
			log.Fatalf("failed to generate markdown docs: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown format %q: must be \"man\" or \"markdown\"\n", *format)
		os.Exit(1)
	}

	fmt.Printf("Generated %s documentation in %s\n", *format, outDir)
}

func disableAutoGenTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}
