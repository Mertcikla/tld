package assets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed build-assets/icons.tar.gz
var iconsArchive []byte

var (
	iconsFSOnce sync.Once
	iconsFS     fs.FS
	iconsFSErr  error
)

// StaticFS returns the embedded application files plus the icon archive
// unpacked into a temporary overlay filesystem.
func StaticFS() (fs.FS, error) {
	iconsFSOnce.Do(func() {
		root, err := materializeIconsTree()
		if err != nil {
			iconsFSErr = err
			return
		}
		iconsFS = overlayFS{
			primary:   FS,
			secondary: os.DirFS(root),
		}
	})

	return iconsFS, iconsFSErr
}

// ExtractIcons writes the embedded icon archive into dstBase.
// The archive entries are unpacked as children of dstBase.
func ExtractIcons(dstBase string) error {
	if err := os.RemoveAll(filepath.Join(dstBase, "icons")); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dstBase, "icons.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(dstBase, "icons.index.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(dstBase, "icons.meta.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return unpackIcons(bytes.NewReader(iconsArchive), dstBase)
}

func materializeIconsTree() (string, error) {
	root, err := os.MkdirTemp("", "tld-icons-*")
	if err != nil {
		return "", err
	}

	if err := unpackIcons(bytes.NewReader(iconsArchive), filepath.Join(root, "frontend", "dist")); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}

	return root, nil
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
		default:
			// Skip non-regular files.
			continue
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

type overlayFS struct {
	primary   fs.FS
	secondary fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if f, err := o.primary.Open(name); err == nil {
		return f, nil
	}
	return o.secondary.Open(name)
}
