package buildassets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconCatalogJSONReadsArchiveCatalog(t *testing.T) {
	body, err := IconCatalogJSON()
	if err != nil {
		t.Fatalf("IconCatalogJSON: %v", err)
	}

	var items []struct {
		DefaultSlug string `json:"defaultSlug"`
		IconURL     string `json:"iconUrl"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("catalog is empty")
	}
	if items[0].DefaultSlug == "" || items[0].IconURL == "" {
		t.Fatalf("catalog item missing slug/icon: %+v", items[0])
	}
}

func TestReadArchiveFileReadsKnownIcon(t *testing.T) {
	body, err := ReadArchiveFile("icons/go.svg")
	if err != nil {
		t.Fatalf("ReadArchiveFile: %v", err)
	}
	if !strings.Contains(string(body), "<svg") {
		t.Fatalf("go icon is not svg content: %.40q", string(body))
	}
}

func TestReadArchiveFileRejectsMissingOrUnsafePaths(t *testing.T) {
	if _, err := ReadArchiveFile("icons/does-not-exist.svg"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file error = %v, want fs.ErrNotExist", err)
	}
	if _, err := ReadArchiveFile("../icons.json"); err == nil {
		t.Fatal("unsafe archive path returned nil error")
	}
}

func TestUnpackIconsWritesCatalogAndIcons(t *testing.T) {
	dst := t.TempDir()
	if err := UnpackIcons(dst); err != nil {
		t.Fatalf("UnpackIcons: %v", err)
	}

	for _, rel := range []string{"icons.json", filepath.Join("icons", "go.svg")} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("expected %s to be unpacked: %v", rel, err)
		}
	}
}
