package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWatchCommandQuotesPaths(t *testing.T) {
	bridge := NewDesktopBridge()

	plain := bridge.WatchCommand("/Users/me/repo", nil)
	if !strings.Contains(plain, "tld watch") || !strings.Contains(plain, "/Users/me/repo") {
		t.Fatalf("watch command missing parts: %q", plain)
	}

	spaced := bridge.WatchCommand("/Users/me/my repo", nil)
	if !strings.Contains(spaced, "'/Users/me/my repo'") {
		t.Fatalf("watch command should quote paths with spaces: %q", spaced)
	}

	withArgs := bridge.WatchCommand("/Users/me/repo", []string{"--rescan"})
	if !strings.HasSuffix(withArgs, "--rescan") {
		t.Fatalf("watch command should append extra args: %q", withArgs)
	}
}

func TestWatchCommandDefaultsToCurrentDirectory(t *testing.T) {
	bridge := NewDesktopBridge()
	command := bridge.WatchCommand("   ", nil)
	if !strings.Contains(command, "tld watch .") {
		t.Fatalf("blank path should default to '.', got %q", command)
	}
}

func TestCLIRuntimeStatusPlatform(t *testing.T) {
	bridge := NewDesktopBridge()
	status := bridge.CLIRuntimeStatus()
	if status.Platform != runtime.GOOS {
		t.Fatalf("platform = %q, want %q", status.Platform, runtime.GOOS)
	}
	wantSupported := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	if status.InstallSupported != wantSupported {
		t.Fatalf("installSupported = %v, want %v for %s", status.InstallSupported, wantSupported, runtime.GOOS)
	}
	if status.InstallHint == "" {
		t.Fatal("expected an install hint")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("simple"); got != "simple" {
		t.Fatalf("shellQuote(simple) = %q", got)
	}
	quoted := shellQuote("has space")
	if runtime.GOOS == "windows" {
		if quoted != `"has space"` {
			t.Fatalf("windows quote = %q", quoted)
		}
	} else if quoted != "'has space'" {
		t.Fatalf("unix quote = %q", quoted)
	}
}

func TestResolveCLIPathFindsBinaryInInstallerDir(t *testing.T) {
	dir := t.TempDir()
	name := cliBinaryName()
	path := filepath.Join(dir, name)
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}

	found, err := lookPathIn(name, []string{dir})
	if err != nil {
		t.Fatalf("expected to find freshly installed binary in installer dir: %v", err)
	}
	if found != path {
		t.Fatalf("resolved path = %q, want %q", found, path)
	}
}

func TestLookPathInIgnoresNonExecutableAndMissing(t *testing.T) {
	dir := t.TempDir()
	name := cliBinaryName()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		// A non-executable file must not be considered installed on unix.
		if _, err := lookPathIn(name, []string{dir}); err == nil {
			t.Fatal("non-executable file should not resolve on unix")
		}
	}
	if _, err := lookPathIn(name, []string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("missing directory should not resolve")
	}
}

func TestResolveCLIPathFallsBackToInstallerDirs(t *testing.T) {
	// An empty PATH must still locate the binary via the documented installer
	// target directories when it exists there.
	name := cliBinaryName()
	var dir string
	for _, candidate := range cliSearchDirs() {
		if info, err := os.Stat(filepath.Join(candidate, name)); err == nil && !info.IsDir() {
			dir = candidate
			break
		}
	}
	if dir == "" {
		t.Skipf("no installed %s found in installer dirs", name)
	}
	if path, ok := resolveCLIPath(""); !ok || path == "" {
		t.Fatalf("resolveCLIPath with empty PATH should find %s in installer dirs (%s)", name, dir)
	}
}
