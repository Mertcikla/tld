package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAppStoreRuntimePathEnvSetsDefaults(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", "")
	t.Setenv("TLD_DATA_DIR", "")

	root := t.TempDir()
	if err := setAppStoreRuntimePathEnv(root); err != nil {
		t.Fatalf("setAppStoreRuntimePathEnv: %v", err)
	}

	appRoot := filepath.Join(root, macAppStoreSupportDir)
	if got := os.Getenv("TLD_CONFIG_DIR"); got != appRoot {
		t.Fatalf("TLD_CONFIG_DIR = %q, want %q", got, appRoot)
	}
	if got, want := os.Getenv("TLD_DATA_DIR"), filepath.Join(appRoot, "data"); got != want {
		t.Fatalf("TLD_DATA_DIR = %q, want %q", got, want)
	}
}

func TestSetAppStoreRuntimePathEnvPreservesExplicitEnv(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_DATA_DIR", dataDir)

	if err := setAppStoreRuntimePathEnv(t.TempDir()); err != nil {
		t.Fatalf("setAppStoreRuntimePathEnv: %v", err)
	}

	if got := os.Getenv("TLD_CONFIG_DIR"); got != configDir {
		t.Fatalf("TLD_CONFIG_DIR = %q, want explicit %q", got, configDir)
	}
	if got := os.Getenv("TLD_DATA_DIR"); got != dataDir {
		t.Fatalf("TLD_DATA_DIR = %q, want explicit %q", got, dataDir)
	}
}
