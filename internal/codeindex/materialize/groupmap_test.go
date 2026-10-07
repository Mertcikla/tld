package materialize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

type cancelMapStore struct {
	core.Store
	cancel    context.CancelFunc
	remaining int
}

func (s *cancelMapStore) created(err error) {
	if err == nil {
		s.remaining--
		if s.remaining == 0 {
			s.cancel()
		}
	}
}

func (s *cancelMapStore) CreateElement(ctx context.Context, input core.LibraryElement) (core.LibraryElement, error) {
	el, err := s.Store.CreateElement(ctx, input)
	s.created(err)
	return el, err
}

func (s *cancelMapStore) CreateView(ctx context.Context, name string, label *string, ownerID *int64) (core.ViewSummary, error) {
	view, err := s.Store.CreateView(ctx, name, label, ownerID)
	s.created(err)
	return view, err
}

func (s *cancelMapStore) CreateConnector(ctx context.Context, input core.Connector) (core.Connector, error) {
	connector, err := s.Store.CreateConnector(ctx, input)
	s.created(err)
	return connector, err
}

func TestApplyGroupMapCancellationRetainsOwnership(t *testing.T) {
	for _, cancelAfter := range []int{1, 2, 5, 9} {
		t.Run(fmt.Sprint(cancelAfter), func(t *testing.T) {
			ws, idx := openGroupMapStore(t)
			input := GroupMapInput{
				RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "snap",
				Files:  groupMapFiles(),
				Groups: []*community.Group{{Key: "all", Name: "all", Files: 4, Members: []int{0, 1, 2, 3}}},
				Edges:  []MapEdge{{FromFactID: "id-a", ToFactID: "id-b", Weight: 1, Kind: "calls"}},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cancelStore := &cancelMapStore{Store: ws, cancel: cancel, remaining: cancelAfter}
			_, err := ApplyGroupMap(ctx, cancelStore, idx, input, MapOptions{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation after %d resources, got %v", cancelAfter, err)
			}
			before, err := idx.MappingsByRepository(context.Background(), "repo")
			if err != nil || len(before) == 0 {
				t.Fatalf("lost ownership on cancellation: %v mappings=%d", err, len(before))
			}
			if _, err := ApplyGroupMap(context.Background(), ws, idx, input, MapOptions{}); err != nil {
				t.Fatal(err)
			}
			after, err := idx.MappingsByRepository(context.Background(), "repo")
			if err != nil {
				t.Fatal(err)
			}
			byKey := map[string]int64{}
			for _, mapping := range after {
				byKey[mapping.LogicalKey] = mapping.ResourceID
			}
			for _, mapping := range before {
				if byKey[mapping.LogicalKey] != mapping.ResourceID {
					t.Fatalf("retry replaced %s: %d -> %d", mapping.LogicalKey, mapping.ResourceID, byKey[mapping.LogicalKey])
				}
			}
			var count int
			if err := ws.DB().QueryRow(`SELECT count(*) FROM elements WHERE name = 'demo'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("retry created duplicate repository elements: %d", count)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func filterPrefix(values []string, prefix string) []string {
	var out []string
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			out = append(out, value)
		}
	}
	return out
}

func hasGroupLayer(layers []core.ViewLayer, name string) bool {
	for _, layer := range layers {
		if layer.Name != name {
			continue
		}
		for _, tag := range layer.Tags {
			if strings.HasPrefix(tag, "group:") {
				return true
			}
		}
	}
	return false
}

func groupMapFiles() []community.File {
	return []community.File{
		{ID: "id-a", Path: "src/alpha/a.go", DisplayName: "a.go", Language: "go"},
		{ID: "id-b", Path: "src/alpha/b.go", DisplayName: "b.go", Language: "go"},
		{ID: "id-c", Path: "src/beta/c.go", DisplayName: "c.go", Language: "go"},
		{ID: "id-d", Path: "src/beta/d.go", DisplayName: "d.go", Language: "go"},
	}
}

func openGroupMapStore(t *testing.T) (*app.Store, *cstore.Store) {
	t.Helper()
	appStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = appStore.Close() })
	return appStore, cstore.NewStore(appStore.DB(), appStore.BunDB(), appStore.Dialect())
}

func viewIDByName(t *testing.T, appStore *app.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := appStore.DB().QueryRow(
		`SELECT v.id FROM views v JOIN elements e ON e.id = v.owner_element_id WHERE e.name = ? ORDER BY v.id LIMIT 1`,
		name).Scan(&id); err != nil {
		t.Fatalf("view for %q: %v", name, err)
	}
	return id
}

func TestApplyGroupMapHierarchyAndRollup(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	files := groupMapFiles()
	groups := []*community.Group{
		{
			Key: "alpha", Name: "alpha", Files: 2, Members: []int{1},
			Children: []*community.Group{{Key: "alpha-child", Name: "alpha-child", Files: 1, Members: []int{0}}},
		},
		{Key: "beta", Name: "beta", Files: 2, Members: []int{2, 3}},
	}
	SortGroups(groups)
	input := GroupMapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		Files:          files,
		Groups:         groups,
		Edges: []MapEdge{
			{FromFactID: "id-a", ToFactID: "id-b", Weight: 2},
			{FromFactID: "id-b", ToFactID: "id-c", Weight: 1},
		},
	}
	result, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.ViewID == 0 || result.Views < 4 || result.Elements < 6 || result.Connectors != 2 {
		t.Fatalf("result = %+v", result)
	}

	rootViewID := result.ViewID
	var rootNames []string
	rows, err := appStore.DB().QueryContext(ctx,
		`SELECT e.name FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? ORDER BY e.name`, rootViewID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		rootNames = append(rootNames, name)
	}
	_ = rows.Close()
	if len(rootNames) != 2 || rootNames[0] != "alpha" || rootNames[1] != "beta" {
		t.Fatalf("root placements = %v, want alpha and beta", rootNames)
	}

	alphaViewID := viewIDByName(t, appStore, "alpha")
	if alphaViewID == rootViewID {
		t.Fatal("alpha view is not nested")
	}
	childViewID := viewIDByName(t, appStore, "alpha-child")

	var alphaFiles int
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = 'b.go'`,
		alphaViewID).Scan(&alphaFiles); err != nil {
		t.Fatal(err)
	}
	if alphaFiles != 1 {
		t.Fatalf("alpha view files = %d, want 1", alphaFiles)
	}
	var childFiles int
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = 'a.go'`,
		childViewID).Scan(&childFiles); err != nil {
		t.Fatal(err)
	}
	if childFiles != 1 {
		t.Fatalf("alpha-child view files = %d, want 1", childFiles)
	}

	var rollupView int64
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT c.view_id FROM connectors c
			JOIN elements s ON s.id = c.source_element_id
			JOIN elements t ON t.id = c.target_element_id
			WHERE (s.name = 'alpha' AND t.name = 'beta') OR (s.name = 'beta' AND t.name = 'alpha')`).Scan(&rollupView); err != nil {
		t.Fatalf("cross-group connector: %v", err)
	}
	if rollupView != rootViewID {
		t.Fatalf("cross-group connector view = %d, want root %d", rollupView, rootViewID)
	}
	var internalView int64
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT c.view_id FROM connectors c
			JOIN elements s ON s.id = c.source_element_id
			JOIN elements t ON t.id = c.target_element_id
			WHERE (s.name = 'alpha-child' AND t.name = 'b.go') OR (s.name = 'b.go' AND t.name = 'alpha-child')`).Scan(&internalView); err != nil {
		t.Fatalf("internal connector: %v", err)
	}
	if internalView != alphaViewID {
		t.Fatalf("internal connector view = %d, want alpha view %d", internalView, alphaViewID)
	}

	second, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if second.ViewID != result.ViewID || second.Pruned != 0 || second.Connectors != result.Connectors {
		t.Fatalf("rerun changed map: %+v vs %+v", second, result)
	}
}

func TestApplyGroupMapLeafConnectorBudget(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	files := groupMapFiles()
	groups := []*community.Group{
		{Key: "leaf", Name: "leaf", Files: 3, Members: []int{0, 1, 2}},
	}
	input := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files: files, Groups: groups,
		Edges: []MapEdge{
			{FromFactID: "id-a", ToFactID: "id-b", Weight: 3},
			{FromFactID: "id-b", ToFactID: "id-c", Weight: 2},
			{FromFactID: "id-a", ToFactID: "id-c", Weight: 1},
		},
	}
	result, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{MaxLeafConnectorsPerView: 1})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.Connectors != 1 {
		t.Fatalf("leaf connectors = %d, want 1", result.Connectors)
	}
	var rows int
	if err := appStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM connectors`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("stored connectors = %d, want 1", rows)
	}
}

func TestApplyGroupMapPrunesStaleGroups(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	files := groupMapFiles()
	groups := []*community.Group{
		{Key: "one", Name: "one", Files: 1, Members: []int{0}},
	}
	input := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files: files, Groups: groups,
	}
	if _, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	updated := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files:  files,
		Groups: []*community.Group{{Key: "two", Name: "two", Files: 2, Members: []int{1, 2}}},
	}
	result, err := ApplyGroupMap(ctx, appStore, idx, updated, MapOptions{})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if result.Pruned == 0 {
		t.Fatal("expected the removed group to be pruned")
	}
	if _, ok, err := idx.MappingByLogicalKey(ctx, groupElementKey("repo-1", "one")); err != nil || ok {
		t.Fatalf("stale group mapping not pruned (ok=%v err=%v)", ok, err)
	}
}

func TestApplyGroupMapNestsUnderWorkspaceRoot(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		Files:          groupMapFiles(),
		Groups: []*community.Group{
			{Key: "one", Name: "one", Files: 2, Members: []int{0, 1}},
		},
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}

	var workspaceID int64
	if err := appStore.DB().QueryRowContext(ctx, `SELECT id FROM views WHERE name = 'Workspace' ORDER BY id LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatalf("find workspace root: %v", err)
	}
	var topPlacements int
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = ?`,
		workspaceID, "demo").Scan(&topPlacements); err != nil {
		t.Fatal(err)
	}
	if topPlacements != 1 {
		t.Fatalf("top element placements in workspace root = %d, want 1", topPlacements)
	}
	var ownerName string
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT e.name FROM views v JOIN elements e ON e.id = v.owner_element_id WHERE v.id = ?`,
		result.ViewID).Scan(&ownerName); err != nil {
		t.Fatalf("map view owner: %v", err)
	}
	if ownerName != "demo" {
		t.Fatalf("map view owner = %q, want demo", ownerName)
	}
}

func TestApplyGroupMapMaterializesImports(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		Files:          groupMapFiles(),
		Groups: []*community.Group{
			{Key: "one", Name: "one", Files: 2, Members: []int{0, 1}},
			{Key: "two", Name: "two", Files: 2, Members: []int{2, 3}},
		},
		Imports: []MapImport{
			{FileFactID: "id-a", Import: "flask"},
			{FileFactID: "id-a", Import: "celery"},
			{FileFactID: "id-a", Import: "flask"}, // duplicate, must be deduped
			{FileFactID: "id-c", Import: "flask"},
		},
	}, MapOptions{IncludeExternalImports: true})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	// Two top-level components import external packages, so imports roll up to
	// one connector each instead of one per (file, import) pair.
	if result.Connectors != 2 {
		t.Fatalf("import connectors = %d, want 2 (one per importing component)", result.Connectors)
	}
	var external, flask, celery int
	if err := appStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'External'`).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if err := appStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'flask'`).Scan(&flask); err != nil {
		t.Fatal(err)
	}
	if err := appStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'celery'`).Scan(&celery); err != nil {
		t.Fatal(err)
	}
	if external != 1 || flask != 1 || celery != 1 {
		t.Fatalf("External=%d flask=%d celery=%d, want 1/1/1", external, flask, celery)
	}
	var placedImports int
	if err := appStore.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM placements p
		JOIN elements e ON e.id = p.element_id
		JOIN views v ON v.id = p.view_id
		WHERE v.name = 'External' AND e.name IN ('flask', 'celery')`).Scan(&placedImports); err != nil {
		t.Fatal(err)
	}
	if placedImports != 2 {
		t.Fatalf("imports placed in External view = %d, want 2", placedImports)
	}
}

func TestApplyGroupMapBoundsImportConnectors(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	const components = 50
	files := make([]community.File, 0, components)
	groups := make([]*community.Group, 0, components)
	imports := make([]MapImport, 0, components)
	for i := 0; i < components; i++ {
		id := fmt.Sprintf("id-%d", i)
		files = append(files, community.File{ID: id, Path: fmt.Sprintf("pkg%d/a.go", i), DisplayName: "a.go", Language: "go"})
		groups = append(groups, &community.Group{Key: fmt.Sprintf("g%d", i), Name: fmt.Sprintf("g%d", i), Files: 1, Members: []int{i}})
		imports = append(imports, MapImport{FileFactID: id, Import: fmt.Sprintf("pkg-%d", i)})
	}
	result, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files: files, Groups: groups, Imports: imports,
	}, MapOptions{MaxConnectorsPerView: 5, MaxLeafConnectorsPerView: 5, IncludeExternalImports: true})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.Connectors != 5 {
		t.Fatalf("import connectors = %d, want 5 (capped by the view budget)", result.Connectors)
	}
}

func TestApplyGroupMapPreservesUserEdits(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	files := groupMapFiles()
	groups := []*community.Group{
		{Key: "alpha", Name: "alpha", Files: 4, Members: []int{0, 1, 2, 3}},
	}
	input := GroupMapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		Files:          files,
		Groups:         groups,
	}
	if _, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	var fileID int64
	if err := appStore.DB().QueryRowContext(ctx, `SELECT id FROM elements WHERE name = 'b.go'`).Scan(&fileID); err != nil {
		t.Fatalf("find file element: %v", err)
	}
	if _, err := appStore.UpdateElement(ctx, fileID, core.LibraryElement{Name: "renamed.go", Tags: []string{"keep"}}); err != nil {
		t.Fatalf("user edit: %v", err)
	}
	userView, err := appStore.CreateView(ctx, "My Diagram", nil, nil)
	if err != nil {
		t.Fatalf("create user view: %v", err)
	}
	if _, err := appStore.AddPlacement(ctx, userView.ID, fileID, 5, 6); err != nil {
		t.Fatalf("user placement: %v", err)
	}
	groupViewID := viewIDByName(t, appStore, "alpha")
	renamedView := "Renamed Alpha"
	if _, err := appStore.UpdateView(ctx, groupViewID, &renamedView, nil, nil, nil); err != nil {
		t.Fatalf("rename view: %v", err)
	}

	// Re-materialize the same repository at a new snapshot.
	input.SnapshotID = "snap-2"
	if _, err := ApplyGroupMap(ctx, appStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	got, err := appStore.ElementByID(ctx, fileID)
	if err != nil {
		t.Fatalf("element: %v", err)
	}
	if got.Name != "renamed.go" {
		t.Fatalf("name = %q, want preserved renamed.go", got.Name)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Fatalf("tags = %v, want preserved [keep]", got.Tags)
	}
	placements, err := appStore.ListElementPlacements(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, placement := range placements {
		if placement.ViewID == userView.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("user placement was lost after re-materialization")
	}
	view, err := appStore.ViewByID(ctx, groupViewID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Name != renamedView {
		t.Fatalf("view name = %q, want preserved %q", view.Name, renamedView)
	}
}

func TestApplyGroupMapEnrichesGeneratedResources(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	files := []community.File{
		{ID: "id-a", Path: "src/alpha/a.go", DisplayName: "a.go", Language: "go"},
		{ID: "id-b", Path: "src/alpha/b.ts", DisplayName: "b.ts", Language: "tsx"},
		{ID: "id-c", Path: "src/alpha/nested/c.go", DisplayName: "c.go", Language: "go"},
		{ID: "id-d", Path: "src/beta/d_test.go", DisplayName: "d_test.go", Language: "go"},
	}
	// alpha mixes two loose files with a nested subgroup, so its group layer
	// structures the view. beta and the nested group are pure, so they get none.
	groups := []*community.Group{
		{Key: "alpha", Name: "alpha", Files: 3, Members: []int{0, 1}, Children: []*community.Group{
			{Key: "alpha-child", Name: "alpha-child", Files: 1, Members: []int{2}},
		}},
		{Key: "beta", Name: "beta", Files: 1, Members: []int{3}},
	}
	result, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", RepositoryRoot: "/repo/demo", SnapshotID: "snap-1",
		Files: files, Groups: groups,
		Edges: []MapEdge{{FromFactID: "id-a", ToFactID: "id-d", Weight: 3, Kind: "calls"}},
	}, MapOptions{AnnotateConnectors: true, AnnotateTags: true, AnnotateTechnology: true, GroupLayers: true})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}

	elementByName := func(name string) core.LibraryElement {
		t.Helper()
		var id int64
		if err := appStore.DB().QueryRowContext(ctx, `SELECT id FROM elements WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatalf("element %q: %v", name, err)
		}
		element, err := appStore.ElementByID(ctx, id)
		if err != nil {
			t.Fatalf("element %q: %v", name, err)
		}
		return element
	}

	if file := elementByName("b.ts"); !containsString(file.Tags, "typescript") || file.Technology == nil || *file.Technology == "" {
		t.Fatalf("b.ts = tags:%v technology:%v, want typescript + technology", file.Tags, file.Technology)
	}
	if file := elementByName("d_test.go"); !containsString(file.Tags, "test") {
		t.Fatalf("d_test.go tags = %v, want test", file.Tags)
	}

	alpha := elementByName("alpha")
	if !containsString(alpha.Tags, "go") || !containsString(alpha.Tags, "typescript") {
		t.Fatalf("alpha tags = %v, want go and typescript", alpha.Tags)
	}
	if groupTags := filterPrefix(alpha.Tags, "group:"); len(groupTags) != 0 {
		t.Fatalf("alpha group tags = %v, want none on the group node", alpha.Tags)
	}
	if alpha.Technology == nil || !strings.Contains(*alpha.Technology, "Go") {
		t.Fatalf("alpha technology = %v, want Go", alpha.Technology)
	}

	// Only alpha's loose files share the group tag; the nested subgroup node and
	// files in pure views stay outside the background.
	alphaGroupTags := filterPrefix(elementByName("a.go").Tags, "group:")
	if len(alphaGroupTags) != 1 {
		t.Fatalf("a.go group tags = %v, want exactly one", alphaGroupTags)
	}
	if !containsString(elementByName("b.ts").Tags, alphaGroupTags[0]) {
		t.Fatalf("b.ts tags = %v, want shared group tag %q", elementByName("b.ts").Tags, alphaGroupTags[0])
	}
	for _, name := range []string{"c.go", "d_test.go", "alpha-child"} {
		if tags := filterPrefix(elementByName(name).Tags, "group:"); len(tags) != 0 {
			t.Fatalf("%s group tags = %v, want none", name, tags)
		}
	}

	var label, relationship sql.NullString
	var tags string
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT label, relationship, tags FROM connectors LIMIT 1`).Scan(&label, &relationship, &tags); err != nil {
		t.Fatalf("connector: %v", err)
	}
	if !label.Valid || label.String != "calls" || !relationship.Valid || relationship.String != "calls" {
		t.Fatalf("connector label/relationship = %v/%v, want calls", label, relationship)
	}
	if !strings.Contains(tags, "dependency") {
		t.Fatalf("connector tags = %s, want dependency", tags)
	}

	rootLayers, err := appStore.Layers(ctx, result.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if hasGroupLayer(rootLayers, "alpha") {
		t.Fatalf("root layers = %+v, want no single-node group layer", rootLayers)
	}
	alphaLayers, err := appStore.Layers(ctx, viewIDByName(t, appStore, "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasGroupLayer(alphaLayers, "alpha") {
		t.Fatalf("alpha layers = %+v, want an alpha group layer over its loose files", alphaLayers)
	}
	if len(alphaLayers) != 1 {
		t.Fatalf("alpha layers = %+v, want exactly one group layer", alphaLayers)
	}
	if childLayers, err := appStore.Layers(ctx, viewIDByName(t, appStore, "alpha-child")); err != nil {
		t.Fatal(err)
	} else if hasGroupLayer(childLayers, "alpha-child") {
		t.Fatalf("alpha-child layers = %+v, want no group layer for a pure view", childLayers)
	}
}

func TestApplyGroupMapAnnotatesImportMetadata(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	if _, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID: "repo-1", SnapshotID: "snap-1", Files: groupMapFiles(),
		Groups:  []*community.Group{{Key: "one", Name: "one", Files: 4, Members: []int{0, 1, 2, 3}}},
		Imports: []MapImport{{FileFactID: "id-a", Import: "flask"}},
	}, MapOptions{IncludeExternalImports: true, AnnotateConnectors: true, AnnotateTags: true}); err != nil {
		t.Fatalf("apply group map: %v", err)
	}

	var externalTags, importTags string
	if err := appStore.DB().QueryRowContext(ctx, `SELECT tags FROM elements WHERE name = 'External'`).Scan(&externalTags); err != nil {
		t.Fatal(err)
	}
	if err := appStore.DB().QueryRowContext(ctx, `SELECT tags FROM elements WHERE name = 'flask'`).Scan(&importTags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(externalTags, "external") || !strings.Contains(importTags, "external") {
		t.Fatalf("external/import tags = %s/%s, want external", externalTags, importTags)
	}

	var label, relationship, tags string
	if err := appStore.DB().QueryRowContext(ctx,
		`SELECT label, relationship, tags FROM connectors LIMIT 1`).Scan(&label, &relationship, &tags); err != nil {
		t.Fatal(err)
	}
	if label != "imports" || relationship != "imports" || !strings.Contains(tags, "external") {
		t.Fatalf("import connector = %q/%q/%s, want imports/imports/external", label, relationship, tags)
	}
}

func TestApplyGroupMapPreservesUserConnectorLabel(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	input := GroupMapInput{
		RepositoryID: "repo-1", SnapshotID: "snap-1", Files: groupMapFiles(),
		Groups: []*community.Group{
			{Key: "alpha", Name: "alpha", Files: 2, Members: []int{0, 1}},
			{Key: "beta", Name: "beta", Files: 2, Members: []int{2, 3}},
		},
		Edges: []MapEdge{{FromFactID: "id-a", ToFactID: "id-c", Weight: 1, Kind: "calls"}},
	}
	opts := MapOptions{AnnotateConnectors: true}
	if _, err := ApplyGroupMap(ctx, appStore, idx, input, opts); err != nil {
		t.Fatal(err)
	}
	var connectorID int64
	if err := appStore.DB().QueryRowContext(ctx, `SELECT id FROM connectors LIMIT 1`).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	custom := "validates JWT"
	if _, err := appStore.UpdateConnector(ctx, connectorID, core.Connector{Label: &custom}); err != nil {
		t.Fatal(err)
	}

	input.SnapshotID = "snap-2"
	if _, err := ApplyGroupMap(ctx, appStore, idx, input, opts); err != nil {
		t.Fatal(err)
	}
	var label, relationship sql.NullString
	if err := appStore.DB().QueryRowContext(ctx, `SELECT label, relationship FROM connectors WHERE id = ?`, connectorID).Scan(&label, &relationship); err != nil {
		t.Fatal(err)
	}
	if !label.Valid || label.String != custom {
		t.Fatalf("label = %v, want preserved %q", label, custom)
	}
	if !relationship.Valid || relationship.String != "calls" {
		t.Fatalf("relationship = %v, want generated calls", relationship)
	}
}

func TestApplyGroupMapAdjustsConnectorHandles(t *testing.T) {
	ctx := context.Background()
	appStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, appStore, idx, GroupMapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		SnapshotID:     "snap-1",
		Files:          groupMapFiles(),
		Groups: []*community.Group{
			{Key: "one", Name: "one", Files: 2, Members: []int{0, 1}},
			{Key: "two", Name: "two", Files: 2, Members: []int{2, 3}},
		},
		Edges: []MapEdge{{FromFactID: "id-a", ToFactID: "id-c", Weight: 1}},
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.Connectors == 0 {
		t.Fatal("expected a cross-group connector")
	}
	rows, err := appStore.DB().QueryContext(ctx, `SELECT source_handle, target_handle FROM connectors`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	valid := map[string]bool{"top": true, "bottom": true, "left": true, "right": true}
	checked := 0
	for rows.Next() {
		var sourceHandle, targetHandle sql.NullString
		if err := rows.Scan(&sourceHandle, &targetHandle); err != nil {
			t.Fatal(err)
		}
		if !sourceHandle.Valid || !targetHandle.Valid || !valid[sourceHandle.String] || !valid[targetHandle.String] {
			t.Fatalf("connector handles = %v/%v, want top/bottom/left/right", sourceHandle, targetHandle)
		}
		checked++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no connectors to inspect")
	}
}

func TestGroupSplitRemovesObsoletePlacements(t *testing.T) {
	ctx := context.Background()
	ws, idx := openGroupMapStore(t)
	input := GroupMapInput{RepositoryID: "repo-split", SnapshotID: "a", Files: groupMapFiles(), Groups: []*community.Group{{Key: "parent", Name: "parent", Files: 4, Members: []int{0, 1, 2, 3}}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	parentID := viewIDByName(t, ws, "parent")
	user, err := ws.CreateElement(ctx, core.LibraryElement{Name: "user-note"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, parentID, user.ID, 100, 200); err != nil {
		t.Fatal(err)
	}
	input.SnapshotID = "b"
	input.Groups = []*community.Group{{Key: "parent", Name: "parent", Files: 4, Children: []*community.Group{
		{Key: "alpha", Name: "alpha", Files: 2, Members: []int{0, 1}},
		{Key: "beta", Name: "beta", Files: 2, Members: []int{2, 3}},
	}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	if viewIDByName(t, ws, "parent") != parentID {
		t.Fatal("parent was recreated")
	}
	placements, err := ws.ElementPlacements(ctx, parentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 3 {
		t.Fatalf("parent retains obsolete file placements: %+v", placements)
	}
	userRetained := false
	for _, placement := range placements {
		if placement.ElementID == user.ID {
			userRetained = true
		}
	}
	if !userRetained {
		t.Fatal("user placement was removed")
	}
	for _, name := range []string{"alpha", "beta"} {
		placements, err := ws.ElementPlacements(ctx, viewIDByName(t, ws, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(placements) != 2 {
			t.Fatalf("child %s placements = %d", name, len(placements))
		}
	}
	// Merging the same parent back removes obsolete child placements as well.
	input.Groups = []*community.Group{{Key: "parent", Name: "parent", Files: 4, Members: []int{0, 1, 2, 3}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	placements, err = ws.ElementPlacements(ctx, parentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 5 {
		t.Fatalf("merged placements = %d", len(placements))
	}
}

// Explicitly implements the optional batch API while retaining the regular
// Store wrapper, so cancellation occurs after the first batch has committed.
type cancelBatchMapStore struct {
	core.Store
	batch  core.BatchElementCreator
	cancel context.CancelFunc
}

func (s *cancelBatchMapStore) CreateElements(ctx context.Context, inputs []core.LibraryElement) ([]core.LibraryElement, error) {
	rows, err := s.batch.CreateElements(ctx, inputs)
	if err == nil {
		s.cancel()
	}
	return rows, err
}

func TestGroupMapBatchCancellationRetainsAllCommittedFiles(t *testing.T) {
	ws, idx := openGroupMapStore(t)
	files := make([]community.File, 205)
	members := make([]int, len(files))
	for i := range files {
		files[i] = community.File{ID: fmt.Sprint(i), Path: fmt.Sprintf("src/f%d.go", i), DisplayName: fmt.Sprintf("f%d.go", i), Language: "go"}
		members[i] = i
	}
	input := GroupMapInput{RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "snap", Files: files, Groups: []*community.Group{{Key: "all", Name: "all", Files: len(files), Members: members}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrapped := &cancelBatchMapStore{Store: ws, batch: ws, cancel: cancel}
	_, err := ApplyGroupMap(ctx, wrapped, idx, input, MapOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	mappings, err := idx.MappingsByRepository(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	var committed int
	before := map[string]int64{}
	for _, mapping := range mappings {
		before[mapping.LogicalKey] = mapping.ResourceID
		if strings.Contains(mapping.LogicalKey, "fact|") {
			committed++
		}
	}
	if committed != 100 {
		t.Fatalf("lost committed ownership: %d files", committed)
	}
	if _, err := ApplyGroupMap(context.Background(), ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	after, err := idx.MappingsByRepository(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range after {
		if id, ok := before[mapping.LogicalKey]; ok && id != mapping.ResourceID {
			t.Fatalf("retry replaced %s", mapping.LogicalKey)
		}
	}
	var count int
	if err := ws.DB().QueryRow(`SELECT count(*) FROM elements WHERE file_path IS NOT NULL`).Scan(&count); err != nil || count != len(files) {
		t.Fatalf("duplicated/lost files: %d %v", count, err)
	}
}

func TestPlacementBatchRollbackAndConnectorCopy(t *testing.T) {
	ws, _ := openGroupMapStore(t)
	ctx := context.Background()
	a, err := ws.CreateElement(ctx, core.LibraryElement{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ws.CreateElement(ctx, core.LibraryElement{Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := ws.CreateView(ctx, "source", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := ws.CreateView(ctx, "target", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		if _, err := ws.AddPlacement(ctx, source.ID, id, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ws.CreateConnector(ctx, core.Connector{ViewID: source.ID, SourceElementID: a.ID, TargetElementID: b.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = ws.DB().Exec(fmt.Sprintf(`CREATE TRIGGER fail_placement BEFORE INSERT ON placements WHEN NEW.view_id = %d AND NEW.element_id = %d BEGIN SELECT RAISE(ABORT, 'test failure'); END`, target.ID, b.ID))
	if err != nil {
		t.Fatal(err)
	}
	pending := []core.ElementPlacement{{ElementID: a.ID, PositionX: 10}, {ElementID: b.ID, PositionX: 20}}
	if err := ws.AddPlacements(ctx, target.ID, pending); err == nil {
		t.Fatal("expected failure")
	}
	placements, err := ws.ElementPlacements(ctx, target.ID)
	if err != nil || len(placements) != 0 {
		t.Fatalf("partial batch: %+v %v", placements, err)
	}
	if _, err := ws.DB().Exec(`DROP TRIGGER fail_placement`); err != nil {
		t.Fatal(err)
	}
	if err := ws.AddPlacements(ctx, target.ID, pending); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := ws.DB().QueryRow(`SELECT count(*) FROM connectors WHERE view_id = ?`, target.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("connector copy: %d %v", count, err)
	}
	if err := ws.AddPlacements(ctx, target.ID, pending); err != nil {
		t.Fatal(err)
	}
	if err := ws.DB().QueryRow(`SELECT count(*) FROM connectors WHERE view_id = ?`, target.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate connector copy: %d %v", count, err)
	}
}
