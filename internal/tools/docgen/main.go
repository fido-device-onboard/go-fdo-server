// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fido-device-onboard/go-fdo-server/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	format := flag.String("format", "man", "Output format: man or markdown")
	out := flag.String("out", "", "Output directory (default: docs/man for man, docs/cli for markdown)")
	flag.Parse()

	if err := run(*format, *out); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(format, outDir string) error {
	root := cmd.Root()
	disableAutoGenTag(root)

	defaults := map[string]string{"man": "docs/man", "markdown": "docs/cli"}
	defaultDir, ok := defaults[format]
	if !ok {
		return fmt.Errorf("unknown format %q: must be \"man\" or \"markdown\"", format)
	}

	if outDir == "" {
		outDir = defaultDir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	switch format {
	case "man":
		header := &doc.GenManHeader{
			Section: "1",
			Source:  "go-fdo-server",
			Manual:  "Go FDO Server",
		}
		if err := doc.GenManTree(root, header, outDir); err != nil {
			return fmt.Errorf("failed to generate man pages: %w", err)
		}
	case "markdown":
		if err := doc.GenMarkdownTree(root, outDir); err != nil {
			return fmt.Errorf("failed to generate markdown docs: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format %q\n", format)
	}

	fmt.Printf("Generated %s documentation in %s\n", format, outDir)
	return nil
}

func disableAutoGenTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}
