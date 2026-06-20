package assets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetStaticFSForTest() {
	iconsFSOnce = sync.Once{}
	iconsFS = nil
	iconsFSErr = nil
	materializeIconsTreeForStaticFS = materializeIconsTree
}

func TestStaticFSMergesCustomIconCatalogAndFiles(t *testing.T) {
	resetStaticFSForTest()
	t.Cleanup(resetStaticFSForTest)

	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	customRoot := filepath.Join(configDir, "icons")
	customIcons := filepath.Join(customRoot, "icons")
	if err := os.MkdirAll(customIcons, 0o755); err != nil {
		t.Fatalf("mkdir custom icons: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customRoot, "icons.json"), []byte(`[
  {
    "iconUrl": "/icons/go.svg",
    "name": "Custom Go",
    "nameShort": "Custom Go",
    "defaultSlug": "go",
    "aliases": ["custom-golang"]
  },
  {
    "iconUrl": "/icons/my-custom-icon.svg",
    "name": "My Custom Icon",
    "nameShort": "Custom",
    "defaultSlug": "my-custom-icon",
    "aliases": ["custom-service"]
  },
  {
    "iconUrl": "/icons/my-custom-png.png",
    "name": "My Custom PNG",
    "nameShort": "Custom PNG",
    "defaultSlug": "my-custom-png"
  }
]`), 0o644); err != nil {
		t.Fatalf("write custom catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customIcons, "go.svg"), []byte(`<svg id="custom-go"></svg>`), 0o644); err != nil {
		t.Fatalf("write custom go icon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customIcons, "my-custom-icon.svg"), []byte(`<svg id="my-custom-icon"></svg>`), 0o644); err != nil {
		t.Fatalf("write custom icon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customIcons, "my-custom-png.png"), []byte("png-body"), 0o644); err != nil {
		t.Fatalf("write custom png icon: %v", err)
	}

	staticFS, err := StaticFS()
	if err != nil {
		t.Fatalf("StaticFS: %v", err)
	}

	catalogBody, err := fs.ReadFile(staticFS, "frontend/dist/icons.json")
	if err != nil {
		t.Fatalf("read merged icons.json: %v", err)
	}
	var catalog []struct {
		Name        string   `json:"name"`
		DefaultSlug string   `json:"defaultSlug"`
		Aliases     []string `json:"aliases"`
	}
	if err := json.Unmarshal(catalogBody, &catalog); err != nil {
		t.Fatalf("unmarshal merged catalog: %v", err)
	}
	if !catalogContains(catalog, "go", "Custom Go", "custom-golang") {
		t.Fatalf("merged catalog missing custom go override")
	}
	if !catalogContains(catalog, "my-custom-icon", "My Custom Icon", "custom-service") {
		t.Fatalf("merged catalog missing custom icon")
	}
	if !catalogContains(catalog, "my-custom-png", "My Custom PNG", "") {
		t.Fatalf("merged catalog missing custom png")
	}
	if _, err := fs.ReadFile(staticFS, "frontend/dist/icons.json.br"); err == nil {
		t.Fatal("compressed embedded icons.json should not bypass merged catalog overlay")
	}

	iconBody, err := fs.ReadFile(staticFS, "frontend/dist/icons/go.svg")
	if err != nil {
		t.Fatalf("read custom go icon: %v", err)
	}
	if !strings.Contains(string(iconBody), `id="custom-go"`) {
		t.Fatalf("go icon was not overlaid: %s", string(iconBody))
	}
	pngBody, err := fs.ReadFile(staticFS, "frontend/dist/icons/my-custom-png.png")
	if err != nil {
		t.Fatalf("read custom png icon: %v", err)
	}
	if string(pngBody) != "png-body" {
		t.Fatalf("custom png body = %q, want png-body", string(pngBody))
	}
}

func TestStaticFSFallsBackToDataDirWhenTempDirIsNotWritable(t *testing.T) {
	resetStaticFSForTest()
	t.Cleanup(resetStaticFSForTest)

	dataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", dataDir)
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())

	unwritable := filepath.Join(t.TempDir(), "unwritable")
	if err := os.Mkdir(unwritable, 0o500); err != nil {
		t.Fatalf("mkdir unwritable temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(unwritable, 0o700)
	})
	t.Setenv("TMPDIR", unwritable)

	staticFS, err := StaticFS()
	if err != nil {
		t.Fatalf("StaticFS: %v", err)
	}
	if _, err := fs.ReadFile(staticFS, "frontend/dist/icons/go.svg"); err != nil {
		t.Fatalf("read fallback icon: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, "cache"))
	if err != nil {
		t.Fatalf("read fallback cache dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected icon overlay to be materialized under data dir cache")
	}
}

func TestStaticFSFallsBackToEmbeddedFSWhenIconMaterializationFails(t *testing.T) {
	resetStaticFSForTest()
	t.Cleanup(resetStaticFSForTest)

	materializeIconsTreeForStaticFS = func() (string, error) {
		return "", errors.New("icon cache denied")
	}

	staticFS, err := StaticFS()
	if err != nil {
		t.Fatalf("StaticFS returned error for recoverable icon materialization failure: %v", err)
	}
	if _, err := fs.ReadFile(staticFS, "frontend/dist/index.html"); err != nil {
		t.Fatalf("read embedded index.html after icon materialization failure: %v", err)
	}
}

func catalogContains(items []struct {
	Name        string   `json:"name"`
	DefaultSlug string   `json:"defaultSlug"`
	Aliases     []string `json:"aliases"`
}, slug, name, alias string) bool {
	for _, item := range items {
		if item.DefaultSlug != slug || item.Name != name {
			continue
		}
		if alias == "" {
			return true
		}
		for _, candidate := range item.Aliases {
			if candidate == alias {
				return true
			}
		}
	}
	return false
}
