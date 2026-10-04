package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestResolveNameAliasOnPath(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "scip_dart")
	writeExecutable(t, want)
	t.Setenv("PATH", dir)

	got, err := ResolveName("scip-dart")
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveName = %q, want %q", got, want)
	}
}

func TestResolveNameWellKnownDir(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".pub-cache", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(bin, "scip_dart")
	writeExecutable(t, want)
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	got, err := ResolveName("scip-dart")
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveName = %q, want %q", got, want)
	}
}

func TestResolveNameDirectPath(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "scip-go")
	writeExecutable(t, exe)
	got, err := ResolveName(exe)
	if err != nil {
		t.Fatalf("ResolveName: %v", err)
	}
	if got != exe {
		t.Fatalf("ResolveName = %q, want %q", got, exe)
	}
}

func TestResolveNameMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if _, err := ResolveName("scip-nonexistent-tool"); err == nil {
		t.Fatal("ResolveName accepted a missing tool")
	}
}

func TestResolveUsesConfiguredPath(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "scip-go")
	writeExecutable(t, exe)
	cfg := config.Default()
	cfg.Tools.SCIPGo = exe
	tool := Tool{Name: "scip-go", path: func(config.Config) string { return cfg.Tools.SCIPGo }}

	got, err := Resolve(tool, cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != exe {
		t.Fatalf("Resolve = %q, want %q", got, exe)
	}
}

func TestCheckFlagsBelowMinimum(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "scip-go")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho 'scip-go 0.1.1'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Tools.SCIPGo = exe
	tool := Tool{Name: "scip-go", VersionArgs: []string{"--version"}, MinVersion: "0.1.26", path: func(config.Config) string { return cfg.Tools.SCIPGo }}

	status := checkOne(context.Background(), tool, cfg)
	if !status.Found || !status.BelowMinimum || status.Minimum != "0.1.26" {
		t.Fatalf("status = %+v, want below-minimum", status)
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		version, minimum    string
		atLeast, comparable bool
	}{
		{"scip-go 0.1.26", "0.1.26", true, true},
		{"scip-go 0.1.25", "0.1.26", false, true},
		{"rust-analyzer 1.95.0 (59807616 2026-04-14)", "1.80.0", true, true},
		{"scip-ruby 0.5.0 git abc", "0.5.0", true, true},
		{"usage: scip-php [options]", "0.1.0", false, false},
		{"scip-java version 0.0.0-SNAPSHOT", "0.1.0", false, true},
	}
	for _, c := range cases {
		atLeast, comparable := versionAtLeast(c.version, c.minimum)
		if atLeast != c.atLeast || comparable != c.comparable {
			t.Fatalf("versionAtLeast(%q,%q) = (%v,%v), want (%v,%v)", c.version, c.minimum, atLeast, comparable, c.atLeast, c.comparable)
		}
	}
}
