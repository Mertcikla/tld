package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const macAppStoreSupportDir = "tlDiagram"

func configureAppStoreRuntimePaths() error {
	if !appStoreBuild || runtime.GOOS != "darwin" {
		return nil
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve app store config dir: %w", err)
	}
	return setAppStoreRuntimePathEnv(configRoot)
}

func setAppStoreRuntimePathEnv(configRoot string) error {
	appRoot := filepath.Join(configRoot, macAppStoreSupportDir)
	if err := setenvIfUnset("TLD_CONFIG_DIR", appRoot); err != nil {
		return err
	}
	return setenvIfUnset("TLD_DATA_DIR", filepath.Join(appRoot, "data"))
}

func setenvIfUnset(key, value string) error {
	if os.Getenv(key) != "" {
		return nil
	}
	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}
