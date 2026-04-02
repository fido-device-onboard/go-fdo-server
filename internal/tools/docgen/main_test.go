// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunManFormat(t *testing.T) {
	outDir := t.TempDir()
	if err := run("man", outDir); err != nil {
		t.Fatalf("run(\"man\") failed: %v", err)
	}

	expected := []string{
		"go-fdo-server.1",
		"go-fdo-server-manufacturing.1",
		"go-fdo-server-owner.1",
		"go-fdo-server-rendezvous.1",
	}
	for _, name := range expected {
		path := filepath.Join(outDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected man page %s not found: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("man page %s is empty", name)
		}
	}
}

func TestRunMarkdownFormat(t *testing.T) {
	outDir := t.TempDir()
	if err := run("markdown", outDir); err != nil {
		t.Fatalf("run(\"markdown\") failed: %v", err)
	}

	expected := []string{
		"go-fdo-server.md",
		"go-fdo-server_manufacturing.md",
		"go-fdo-server_owner.md",
		"go-fdo-server_rendezvous.md",
	}
	for _, name := range expected {
		path := filepath.Join(outDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected markdown file %s not found: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("markdown file %s is empty", name)
		}
	}
}

func TestRunCreatesOutputDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "nested", "output")
	if err := run("man", outDir); err != nil {
		t.Fatalf("run() failed to create nested output dir: %v", err)
	}
	if _, err := os.Stat(outDir); err != nil {
		t.Fatalf("output directory was not created: %v", err)
	}
}

func TestRunUnknownFormat(t *testing.T) {
	outDir := t.TempDir()
	err := run("bogus", outDir)
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

func TestRunInvalidOutputDir(t *testing.T) {
	err := run("man", "/dev/null/invalid")
	if err == nil {
		t.Fatal("expected error for invalid output dir, got nil")
	}
}
