package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	target := "./public"
	if len(os.Args) > 1 && os.Args[1] != "" {
		target = os.Args[1]
	}

	archivePath, err := resolveArchivePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve archive path: %v\n", err)
		os.Exit(1)
	}

	if err := extractIcons(archivePath, target); err != nil {
		fmt.Fprintf(os.Stderr, "extract icons: %v\n", err)
		os.Exit(1)
	}
}

func resolveArchivePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Dir(exePath)
	archivePath := filepath.Join(scriptDir, "..", "build-assets", "icons.tar.gz")
	if _, err := os.Stat(archivePath); err == nil {
		return archivePath, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	archivePath = filepath.Join(wd, "..", "build-assets", "icons.tar.gz")
	if _, err := os.Stat(archivePath); err == nil {
		return archivePath, nil
	}

	return "", fmt.Errorf("build-assets/icons.tar.gz not found")
}

func extractIcons(archivePath, dstBase string) error {
	if err := os.RemoveAll(filepath.Join(dstBase, "icons")); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dstBase, "icons.json")); err != nil && !os.IsNotExist(err) {
		return err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return unpackIcons(f, dstBase)
}

func unpackIcons(r io.Reader, dstBase string) error {
	if err := os.MkdirAll(dstBase, 0o755); err != nil {
		return err
	}

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(hdr.Name)
		if cleanName == "." || cleanName == ".." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			continue
		}

		target := filepath.Join(dstBase, cleanName)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		}
	}
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}
