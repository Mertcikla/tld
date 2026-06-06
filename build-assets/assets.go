// Package buildassets exposes built-in build artifacts used by the tld binary.
package buildassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed icons.tar.gz
var iconsArchive []byte

// IconCatalogJSON returns the built-in icon catalog from icons.tar.gz.
func IconCatalogJSON() ([]byte, error) {
	return ReadArchiveFile("icons.json")
}

// ReadArchiveFile returns one regular file from the built-in icon archive.
func ReadArchiveFile(name string) ([]byte, error) {
	cleanName, ok := cleanArchiveName(name)
	if !ok {
		return nil, fmt.Errorf("invalid archive path %q", name)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(iconsArchive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s: %w", cleanName, fs.ErrNotExist)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		entryName, ok := cleanArchiveName(hdr.Name)
		if !ok || entryName != cleanName {
			continue
		}
		return io.ReadAll(tr)
	}
}

// UnpackIcons writes the built-in icon archive into dst.
func UnpackIcons(dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(iconsArchive))
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

		cleanName, ok := cleanArchiveName(hdr.Name)
		if !ok {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(cleanName))

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
		default:
			continue
		}
	}
}

func cleanArchiveName(name string) (string, bool) {
	cleanName := path.Clean(strings.TrimSpace(name))
	if cleanName == "." || cleanName == ".." || path.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") {
		return "", false
	}
	return cleanName, true
}

func writeFile(filePath string, r io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}
