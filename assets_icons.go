package assets

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	buildassets "github.com/mertcikla/tld/v2/build-assets"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

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
			primary:   os.DirFS(root),
			secondary: FS,
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
	return buildassets.UnpackIcons(dstBase)
}

func materializeIconsTree() (string, error) {
	root, err := os.MkdirTemp("", "tld-icons-*")
	if err != nil {
		return "", err
	}

	dist := filepath.Join(root, "frontend", "dist")
	if err := buildassets.UnpackIcons(dist); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	if err := copyCustomIcons(dist); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	catalogJSON, err := tech.CatalogJSON()
	if err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dist, "icons.json"), catalogJSON, 0o644); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}

	return root, nil
}

func copyCustomIcons(dstBase string) error {
	configDir, err := workspace.ConfigDir()
	if err != nil {
		return nil
	}
	customIconsDir := filepath.Join(configDir, "icons", "icons")
	entries, err := os.ReadDir(customIconsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dstIconsDir := filepath.Join(dstBase, "icons")
	if err := os.MkdirAll(dstIconsDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".svg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		src := filepath.Join(customIconsDir, entry.Name())
		dst := filepath.Join(dstIconsDir, entry.Name())
		if err := copyFile(dst, src, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}

type overlayFS struct {
	primary   fs.FS
	secondary fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	f, err := o.primary.Open(name)
	if err == nil {
		return f, nil
	}
	if isIconOverlayPath(name) {
		return nil, err
	}
	return o.secondary.Open(name)
}

func isIconOverlayPath(name string) bool {
	cleanName := path.Clean(name)
	return cleanName == "frontend/dist/icons.json" ||
		cleanName == "frontend/dist/icons.json.br" ||
		cleanName == "frontend/dist/icons.json.gz" ||
		strings.HasPrefix(cleanName, "frontend/dist/icons/")
}
