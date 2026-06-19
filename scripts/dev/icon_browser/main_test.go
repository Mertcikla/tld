package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testArchiveEntry struct {
	name     string
	body     string
	typeflag byte
	mode     int64
}

func TestUnpackArchiveSkipsUnsafeEntries(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "icons.tar.gz")
	writeTestArchive(t, archivePath, []testArchiveEntry{
		{name: "icons/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "icons/good.svg", body: `<svg id="good"></svg>`, typeflag: tar.TypeReg, mode: 0o644},
		{name: "icons.json", body: `[{"iconUrl":"/icons/good.svg","name":"Good","nameShort":"Good","defaultSlug":"good"}]`, typeflag: tar.TypeReg, mode: 0o644},
		{name: "../evil.svg", body: `<svg id="evil"></svg>`, typeflag: tar.TypeReg, mode: 0o644},
		{name: "/absolute.svg", body: `<svg id="absolute"></svg>`, typeflag: tar.TypeReg, mode: 0o644},
		{name: "other.svg", body: `<svg id="other"></svg>`, typeflag: tar.TypeReg, mode: 0o644},
	})

	dst := filepath.Join(t.TempDir(), "out")
	if err := unpackArchive(archivePath, dst); err != nil {
		t.Fatalf("unpackArchive: %v", err)
	}
	assertFileContains(t, filepath.Join(dst, "icons", "good.svg"), `id="good"`)
	assertFileContains(t, filepath.Join(dst, "icons.json"), `"defaultSlug":"good"`)
	for _, unsafePath := range []string{
		filepath.Join(dst, "evil.svg"),
		filepath.Join(dst, "absolute.svg"),
		filepath.Join(dst, "other.svg"),
	} {
		if _, err := os.Stat(unsafePath); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("unsafe path %s exists or returned unexpected error: %v", unsafePath, err)
		}
	}
}

func TestPatchIconRenamesFileAndPreservesCatalogOrder(t *testing.T) {
	workDir := seededWorkspace(t, []catalogItem{
		{IconURL: "/icons/old.svg", Name: "Old", NameShort: "Old", DefaultSlug: "old"},
		{IconURL: "/icons/other.svg", Name: "Other", NameShort: "Other", DefaultSlug: "other"},
	})
	writeIcon(t, workDir, "old.svg", `<svg id="old"></svg>`)
	writeIcon(t, workDir, "other.svg", `<svg id="other"></svg>`)

	server := &iconBrowserServer{workDir: workDir}
	updated, err := server.patchIconLocked("old", catalogItem{
		IconURL:     "/icons/new.svg",
		Name:        "New",
		NameShort:   "New",
		DefaultSlug: "new",
		Aliases:     []string{"New Alias", "new alias"},
	})
	if err != nil {
		t.Fatalf("patchIconLocked: %v", err)
	}
	if updated.DefaultSlug != "new" {
		t.Fatalf("updated slug = %q, want new", updated.DefaultSlug)
	}
	if _, err := os.Stat(filepath.Join(workDir, "icons", "old.svg")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old icon still exists or stat failed unexpectedly: %v", err)
	}
	assertFileContains(t, filepath.Join(workDir, "icons", "new.svg"), `id="old"`)
	if !server.dirty {
		t.Fatal("server.dirty = false, want true")
	}

	catalog, err := readCatalogFile(workDir)
	if err != nil {
		t.Fatalf("readCatalogFile: %v", err)
	}
	if got := []string{catalog[0].DefaultSlug, catalog[1].DefaultSlug}; strings.Join(got, ",") != "new,other" {
		t.Fatalf("catalog order = %v, want [new other]", got)
	}
	if got := catalog[0].Aliases; len(got) != 1 || got[0] != "New Alias" {
		t.Fatalf("aliases = %v, want duplicate-normalized alias", got)
	}
}

func TestRepackWorkspaceCreatesBackupAndWritesUpdates(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "icons.tar.gz")
	writeTestArchive(t, archivePath, []testArchiveEntry{
		{name: "icons/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "icons/base.svg", body: `<svg id="base"></svg>`, typeflag: tar.TypeReg, mode: 0o644},
		{name: "icons.json", body: `[
  {
    "iconUrl": "/icons/base.svg",
    "name": "Base",
    "nameShort": "Base",
    "defaultSlug": "base"
  }
]`, typeflag: tar.TypeReg, mode: 0o644},
	})

	workDir := filepath.Join(root, "work")
	if err := unpackArchive(archivePath, workDir); err != nil {
		t.Fatalf("unpackArchive: %v", err)
	}
	writeIcon(t, workDir, "added.svg", `<svg id="added"></svg>`)
	if err := writeCatalogFile(workDir, []catalogItem{
		{IconURL: "/icons/base.svg", Name: "Base", NameShort: "Base", DefaultSlug: "base"},
		{IconURL: "/icons/added.svg", Name: "Added", NameShort: "Added", DefaultSlug: "added"},
	}); err != nil {
		t.Fatalf("writeCatalogFile: %v", err)
	}

	backupPath, err := repackWorkspace(archivePath, workDir)
	if err != nil {
		t.Fatalf("repackWorkspace: %v", err)
	}
	assertFileContains(t, backupPath, "")

	unpacked := filepath.Join(root, "unpacked")
	if err := unpackArchive(archivePath, unpacked); err != nil {
		t.Fatalf("unpack repacked archive: %v", err)
	}
	assertFileContains(t, filepath.Join(unpacked, "icons", "added.svg"), `id="added"`)
	body, err := os.ReadFile(filepath.Join(unpacked, "icons.json"))
	if err != nil {
		t.Fatalf("read repacked catalog: %v", err)
	}
	var catalog []catalogItem
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatalf("unmarshal repacked catalog: %v", err)
	}
	if len(catalog) != 2 || catalog[1].DefaultSlug != "added" {
		t.Fatalf("catalog = %+v, want added item appended", catalog)
	}
}

func TestValidateCatalogRejectsMissingIcon(t *testing.T) {
	workDir := seededWorkspace(t, []catalogItem{
		{IconURL: "/icons/missing.svg", Name: "Missing", NameShort: "Missing", DefaultSlug: "missing"},
	})
	if err := validateCatalog(workDir, []catalogItem{
		{IconURL: "/icons/missing.svg", Name: "Missing", NameShort: "Missing", DefaultSlug: "missing"},
	}); err == nil || !strings.Contains(err.Error(), "missing icon") {
		t.Fatalf("validateCatalog error = %v, want missing icon error", err)
	}
}

func seededWorkspace(t *testing.T, catalog []catalogItem) string {
	t.Helper()
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "icons"), 0o755); err != nil {
		t.Fatalf("mkdir icons: %v", err)
	}
	if err := writeCatalogFile(workDir, catalog); err != nil {
		t.Fatalf("writeCatalogFile: %v", err)
	}
	return workDir
}

func writeIcon(t *testing.T, workDir string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, "icons", name), []byte(body), 0o644); err != nil {
		t.Fatalf("write icon %s: %v", name, err)
	}
}

func assertFileContains(t *testing.T, filePath string, want string) {
	t.Helper()
	body, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}
	if want != "" && !strings.Contains(string(body), want) {
		t.Fatalf("%s does not contain %q: %s", filePath, want, string(body))
	}
}

func writeTestArchive(t *testing.T, archivePath string, entries []testArchiveEntry) {
	t.Helper()
	var buf bytes.Buffer
	gzw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gzip writer: %v", err)
	}
	tw := tar.NewWriter(gzw)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     mode,
			Typeflag: typeflag,
			ModTime:  time.Unix(0, 0),
		}
		if typeflag == tar.TypeReg {
			hdr.Size = int64(len(entry.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", entry.name, err)
		}
		if typeflag == tar.TypeReg {
			if _, err := io.WriteString(tw, entry.body); err != nil {
				t.Fatalf("write body %s: %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}
