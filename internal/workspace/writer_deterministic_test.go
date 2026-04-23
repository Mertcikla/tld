package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/diag/tld/internal/workspace"
)

func TestSave_Deterministic(t *testing.T) {
	dir := t.TempDir()
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{Version: "v1"}); err != nil {
		t.Fatal(err)
	}

	ws := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"z-ref": {Name: "Z Element", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
			"a-ref": {Name: "A Element", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
		},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{
				"a-ref": {ID: 1, UpdatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				"z-ref": {ID: 2, UpdatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
			Views: map[string]*workspace.ResourceMetadata{
				"a-ref": {ID: 3, UpdatedAt: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
				"z-ref": {ID: 4, UpdatedAt: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
		},
	}

	if err := workspace.Save(ws); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	aIdx := strings.Index(s, "a-ref:")
	zIdx := strings.Index(s, "z-ref:")

	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("missing keys in YAML:\n%s", s)
	}
	if aIdx > zIdx {
		t.Fatalf("a-ref should come before z-ref in sorted YAML:\n%s", s)
	}
	if strings.Contains(s, "_meta_views:") {
		t.Fatalf("elements.yaml should not contain _meta_views when lockfile exists:\n%s", s)
	}
	if strings.Contains(s, "\n\n") {
		t.Fatalf("elements.yaml should not contain blank spacer lines:\n%s", s)
	}

	lockFile, err := workspace.LoadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lockFile.CurrentViews["a-ref"] == nil || lockFile.CurrentViews["z-ref"] == nil {
		t.Fatalf("lockfile current views missing: %+v", lockFile.CurrentViews)
	}

	connectorsData, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(connectorsData), "\n\n") {
		t.Fatalf("connectors.yaml should not contain blank spacer lines:\n%s", connectorsData)
	}
}
