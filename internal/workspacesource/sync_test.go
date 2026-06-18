package workspacesource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

type exportTestStore struct {
	api.Store

	views      []*diagv1.View
	elements   []*diagv1.Element
	placements []*diagv1.PlacedElement
	connectors []*diagv1.Connector
}

func newExportTestStore(viewName string) *exportTestStore {
	return &exportTestStore{
		views: []*diagv1.View{{Id: 1, Name: viewName}},
	}
}

func (s *exportTestStore) ListViews(context.Context, uuid.UUID) ([]*diagv1.View, error) {
	return s.views, nil
}

func (s *exportTestStore) ListElements(context.Context, uuid.UUID, int32, int32, string) ([]*diagv1.Element, int, error) {
	return s.elements, len(s.elements), nil
}

func (s *exportTestStore) ListAllPlacements(context.Context, uuid.UUID) ([]*diagv1.PlacedElement, error) {
	return s.placements, nil
}

func (s *exportTestStore) ListAllConnectors(context.Context, uuid.UUID) ([]*diagv1.Connector, error) {
	return s.connectors, nil
}

func exportWorkspaceSource(t *testing.T, dir string, store Store) *Result {
	t.Helper()
	result, err := Export(context.Background(), store, Options{WorkspaceDir: dir})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	return result
}

func platformViewFile(dir string) string {
	return filepath.Join(dir, "views", "platform", ViewFileName)
}

func requireExportError(t *testing.T, dir string, store Store, want string) {
	t.Helper()
	_, err := Export(context.Background(), store, Options{WorkspaceDir: dir})
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Export() error = %v, want containing %q", err, want)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestExportPreservesNonViewMarkdownNeighbors(t *testing.T) {
	dir := t.TempDir()
	store := newExportTestStore("Platform")
	exportWorkspaceSource(t, dir, store)

	neighbor := filepath.Join(dir, "views", "notes.txt")
	if err := os.WriteFile(neighbor, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write neighbor: %v", err)
	}
	markdownNeighbor := filepath.Join(dir, "views", "notes.md")
	if err := os.WriteFile(markdownNeighbor, []byte("# keep me"), 0o644); err != nil {
		t.Fatalf("write markdown neighbor: %v", err)
	}

	exportWorkspaceSource(t, dir, store)

	if got := readFileString(t, neighbor); got != "keep me" {
		t.Fatalf("neighbor content = %q, want preserved", got)
	}
	if got := readFileString(t, markdownNeighbor); got != "# keep me" {
		t.Fatalf("markdown neighbor content = %q, want preserved", got)
	}
	if _, err := os.Stat(platformViewFile(dir)); err != nil {
		t.Fatalf("expected exported view file: %v", err)
	}
	if got := readFileString(t, platformViewFile(dir)); !strings.HasPrefix(got, "```mermaid\n") || !strings.HasSuffix(got, "\n```\n") {
		t.Fatalf("exported view file is not a Mermaid Markdown block:\n%s", got)
	}
}

func TestExportAbortsWhenTreeIsDirty(t *testing.T) {
	dir := t.TempDir()
	store := newExportTestStore("Platform")
	exportWorkspaceSource(t, dir, store)

	path := platformViewFile(dir)
	const dirtyContent = "flowchart LR\n%% locally edited\n"
	if err := os.WriteFile(path, []byte(dirtyContent), 0o644); err != nil {
		t.Fatalf("dirty write: %v", err)
	}

	requireExportError(t, dir, store, "changed since last import/export")

	if got := readFileString(t, path); got != dirtyContent {
		t.Fatalf("dirty file content = %q, want unchanged", got)
	}
}

func TestExportAbortsWhenPreviouslyManagedRootIsMissing(t *testing.T) {
	dir := t.TempDir()
	store := newExportTestStore("Platform")
	exportWorkspaceSource(t, dir, store)

	root := filepath.Join(dir, "views")
	if err := os.Rename(root, filepath.Join(dir, "views-moved")); err != nil {
		t.Fatalf("move source root: %v", err)
	}

	requireExportError(t, dir, store, "is missing")

	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("source root was recreated, stat err=%v", err)
	}
}

func TestExportAbortsWhenExistingViewMarkdownWouldBecomeStale(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "views")
	writeTestViewMarkdown(t, root, "old/view.md", `flowchart LR
%% tld/v1 view ref=old parent=root name=Old
`)
	hash, err := HashTree(root)
	if err != nil {
		t.Fatalf("HashTree: %v", err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version: "v1",
		WorkspaceSource: &workspace.WorkspaceSourceLock{
			ViewsDir: "views",
			LastHash: hash,
		},
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	store := newExportTestStore("Platform")
	requireExportError(t, dir, store, "would leave stale view.md files")

	if _, err := os.Stat(filepath.Join(root, "old", ViewFileName)); err != nil {
		t.Fatalf("stale file was removed: %v", err)
	}
	if _, err := os.Stat(platformViewFile(dir)); !os.IsNotExist(err) {
		t.Fatalf("new export file was written despite stale source, stat err=%v", err)
	}
}

func TestExportAbortsWhenLegacyMMDExists(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "views")
	writeLegacyMMD(t, root, "old/view.mmd", `flowchart LR
%% tld/v1 view ref=old parent=root name=Old
`)

	store := newExportTestStore("Platform")
	requireExportError(t, dir, store, "legacy .mmd files")

	if _, err := os.Stat(filepath.Join(root, "old", "view.mmd")); err != nil {
		t.Fatalf("legacy file was removed: %v", err)
	}
	if _, err := os.Stat(platformViewFile(dir)); !os.IsNotExist(err) {
		t.Fatalf("new export file was written despite legacy source, stat err=%v", err)
	}
}

func TestExportAbortsWhenExistingTargetIsUnwritable(t *testing.T) {
	dir := t.TempDir()
	store := newExportTestStore("Platform")
	exportWorkspaceSource(t, dir, store)

	path := platformViewFile(dir)
	original := readFileString(t, path)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	requireExportError(t, dir, store, "is not writable")

	if got := readFileString(t, path); got != original {
		t.Fatalf("unwritable file content changed")
	}
}

func TestPlanImportUsesNumericConnectorRefAsExistingID(t *testing.T) {
	desired := &desiredWorkspace{
		Connectors: map[string]*desiredConnector{
			"9": {
				Ref:       "9",
				ViewRef:   "platform",
				SourceRef: "api",
				TargetRef: "db",
				Label:     "reads",
			},
		},
		ConnectorOrder: []string{"9"},
	}
	state := &sqliteState{
		connectorsByID: map[int32]*diagv1.Connector{
			9: {Id: 9},
		},
	}
	lock := &workspace.WorkspaceSourceLock{
		ManagedConnectors: map[string]*workspace.ResourceMetadata{},
	}

	plan := planImport(desired, state, lock)

	if plan.connectors.Created != 0 {
		t.Fatalf("created connectors = %d, want 0", plan.connectors.Created)
	}
	if plan.connectors.Updated != 1 {
		t.Fatalf("updated connectors = %d, want 1", plan.connectors.Updated)
	}
}
