package main

import (
	"fmt"
	"os"
	"path/filepath"

	buildassets "github.com/mertcikla/tld/v2/build-assets"
)

func main() {
	target := "./public"
	if len(os.Args) > 1 && os.Args[1] != "" {
		target = os.Args[1]
	}

	if err := extractIcons(target); err != nil {
		fmt.Fprintf(os.Stderr, "extract icons: %v\n", err)
		os.Exit(1)
	}
}

func extractIcons(dstBase string) error {
	if err := os.RemoveAll(filepath.Join(dstBase, "icons")); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dstBase, "icons.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return buildassets.UnpackIcons(dstBase)
}
