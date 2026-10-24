package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTargetStateIsolation(t *testing.T) {
	dir := t.TempDir()
	ws := &Workspace{Dir: dir, Elements: map[string]*Element{"api": {Name: "API"}}, Connectors: map[string]*Connector{}}
	if err := Save(ws); err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"local-a", "remote-a", "remote-b"} {
		lock, err := BindTarget(ws, key)
		if err != nil {
			t.Fatal(err)
		}
		if len(ws.Meta.Elements) != 0 || lock.Metadata != nil || lock.WorkspaceHash != "" {
			t.Fatalf("new destination %s inherited IDs or baseline", key)
		}
		ws.Meta.Elements["api"] = &ResourceMetadata{ID: ResourceID(i + 1), UpdatedAt: time.Now()}
		UpdateLockFile(lock, key, "test", &ResourceCounts{Elements: i + 1}, key+"-hash", nil, ws.Meta)
		if err := WriteLockFile(dir, lock); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(dir, ".tld.lock"))
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"local-a", "remote-a", "remote-b"} {
		lock, err := BindTarget(ws, key)
		if err != nil {
			t.Fatal(err)
		}
		if ws.Meta.Elements["api"].ID != ResourceID(i+1) || lock.Metadata.Elements["api"].ID != ResourceID(i+1) || lock.WorkspaceHash != key+"-hash" || lock.Resources.Elements != i+1 {
			t.Fatalf("lost destination %s history: %+v", key, lock)
		}
		ws.Meta.Elements["api"].ID = 99
		if lock.Metadata.Elements["api"].ID == 99 {
			t.Fatal("current metadata aliases historical baseline")
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, ".tld.lock"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("binding mutated persisted metadata: %v", err)
	}
}

func TestBindTargetRejectsLegacyAndInlineIDs(t *testing.T) {
	dir := t.TempDir()
	ws := &Workspace{
		Dir:        dir,
		Elements:   map[string]*Element{"api": {Name: "API"}},
		Connectors: map[string]*Connector{"root/api~api/calls": {View: "root", Source: "api", Target: "api", Label: "calls", ID: 9}},
		Meta:       &Meta{Elements: map[string]*ResourceMetadata{"api": {ID: 7}}, Views: map[string]*ResourceMetadata{"api": {ID: 8}}},
	}
	if err := Save(ws); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Meta.Elements["api"].ID != 7 {
		t.Fatal("legacy fixture did not load")
	}
	if _, err := BindTarget(loaded, "new-destination"); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Meta.Elements)+len(loaded.Meta.Views)+len(loaded.Meta.Connectors) != 0 || loaded.Connectors["root/api~api/calls"].ID != 0 {
		t.Fatal("unattributed legacy IDs leaked into new destination")
	}
}
