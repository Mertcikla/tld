package materialize

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/store"
)

func groupMapFiles() []community.File {
	return []community.File{
		{ID: "id-a", Path: "src/alpha/a.go", DisplayName: "a.go", Language: "go"},
		{ID: "id-b", Path: "src/alpha/b.go", DisplayName: "b.go", Language: "go"},
		{ID: "id-c", Path: "src/beta/c.go", DisplayName: "c.go", Language: "go"},
		{ID: "id-d", Path: "src/beta/d.go", DisplayName: "d.go", Language: "go"},
	}
}

func openGroupMapStore(t *testing.T) (*store.SQLiteStore, *cstore.Store) {
	t.Helper()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })
	return sqliteStore, cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
}

func viewIDByName(t *testing.T, sqliteStore *store.SQLiteStore, name string) int64 {
	t.Helper()
	var id int64
	if err := sqliteStore.DB().QueryRow(
		`SELECT v.id FROM views v JOIN elements e ON e.id = v.owner_element_id WHERE e.name = ? ORDER BY v.id LIMIT 1`,
		name).Scan(&id); err != nil {
		t.Fatalf("view for %q: %v", name, err)
	}
	return id
}

func TestApplyGroupMapHierarchyAndRollup(t *testing.T) {
	ctx := context.Background()
	sqliteStore, idx := openGroupMapStore(t)
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
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.ViewID == 0 || result.Views < 4 || result.Elements < 6 || result.Connectors != 2 {
		t.Fatalf("result = %+v", result)
	}

	rootViewID := result.ViewID
	var rootNames []string
	rows, err := sqliteStore.DB().QueryContext(ctx,
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

	alphaViewID := viewIDByName(t, sqliteStore, "alpha")
	if alphaViewID == rootViewID {
		t.Fatal("alpha view is not nested")
	}
	childViewID := viewIDByName(t, sqliteStore, "alpha-child")

	var alphaFiles int
	if err := sqliteStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = 'b.go'`,
		alphaViewID).Scan(&alphaFiles); err != nil {
		t.Fatal(err)
	}
	if alphaFiles != 1 {
		t.Fatalf("alpha view files = %d, want 1", alphaFiles)
	}
	var childFiles int
	if err := sqliteStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = 'a.go'`,
		childViewID).Scan(&childFiles); err != nil {
		t.Fatal(err)
	}
	if childFiles != 1 {
		t.Fatalf("alpha-child view files = %d, want 1", childFiles)
	}

	var rollupView int64
	if err := sqliteStore.DB().QueryRowContext(ctx,
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
	if err := sqliteStore.DB().QueryRowContext(ctx,
		`SELECT c.view_id FROM connectors c
			JOIN elements s ON s.id = c.source_element_id
			JOIN elements t ON t.id = c.target_element_id
			WHERE (s.name = 'alpha-child' AND t.name = 'b.go') OR (s.name = 'b.go' AND t.name = 'alpha-child')`).Scan(&internalView); err != nil {
		t.Fatalf("internal connector: %v", err)
	}
	if internalView != alphaViewID {
		t.Fatalf("internal connector view = %d, want alpha view %d", internalView, alphaViewID)
	}

	second, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if second.ViewID != result.ViewID || second.Pruned != 0 || second.Connectors != result.Connectors {
		t.Fatalf("rerun changed map: %+v vs %+v", second, result)
	}
}

func TestApplyGroupMapLeafConnectorBudget(t *testing.T) {
	ctx := context.Background()
	sqliteStore, idx := openGroupMapStore(t)
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
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{MaxLeafConnectorsPerView: 1})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.Connectors != 1 {
		t.Fatalf("leaf connectors = %d, want 1", result.Connectors)
	}
	var rows int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM connectors`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("stored connectors = %d, want 1", rows)
	}
}

func TestApplyGroupMapPrunesStaleGroups(t *testing.T) {
	ctx := context.Background()
	sqliteStore, idx := openGroupMapStore(t)
	files := groupMapFiles()
	groups := []*community.Group{
		{Key: "one", Name: "one", Files: 1, Members: []int{0}},
	}
	input := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files: files, Groups: groups,
	}
	if _, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	updated := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files:  files,
		Groups: []*community.Group{{Key: "two", Name: "two", Files: 2, Members: []int{1, 2}}},
	}
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, updated, MapOptions{})
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
	sqliteStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, GroupMapInput{
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
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT id FROM views WHERE name = 'Workspace' ORDER BY id LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatalf("find workspace root: %v", err)
	}
	var topPlacements int
	if err := sqliteStore.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM placements p JOIN elements e ON e.id = p.element_id WHERE p.view_id = ? AND e.name = ?`,
		workspaceID, "demo").Scan(&topPlacements); err != nil {
		t.Fatal(err)
	}
	if topPlacements != 1 {
		t.Fatalf("top element placements in workspace root = %d, want 1", topPlacements)
	}
	var ownerName string
	if err := sqliteStore.DB().QueryRowContext(ctx,
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
	sqliteStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, GroupMapInput{
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
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	// Two top-level components import external packages, so imports roll up to
	// one connector each instead of one per (file, import) pair.
	if result.Connectors != 2 {
		t.Fatalf("import connectors = %d, want 2 (one per importing component)", result.Connectors)
	}
	var external, flask, celery int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'External'`).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'flask'`).Scan(&flask); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE name = 'celery'`).Scan(&celery); err != nil {
		t.Fatal(err)
	}
	if external != 1 || flask != 1 || celery != 1 {
		t.Fatalf("External=%d flask=%d celery=%d, want 1/1/1", external, flask, celery)
	}
	var placedImports int
	if err := sqliteStore.DB().QueryRowContext(ctx, `
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
	sqliteStore, idx := openGroupMapStore(t)
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
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Files: files, Groups: groups, Imports: imports,
	}, MapOptions{MaxConnectorsPerView: 5, MaxLeafConnectorsPerView: 5})
	if err != nil {
		t.Fatalf("apply group map: %v", err)
	}
	if result.Connectors != 5 {
		t.Fatalf("import connectors = %d, want 5 (capped by the view budget)", result.Connectors)
	}
}

func TestApplyGroupMapPreservesUserEdits(t *testing.T) {
	ctx := context.Background()
	sqliteStore, idx := openGroupMapStore(t)
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
	if _, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	var fileID int64
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT id FROM elements WHERE name = 'b.go'`).Scan(&fileID); err != nil {
		t.Fatalf("find file element: %v", err)
	}
	if _, err := sqliteStore.UpdateElement(ctx, fileID, core.LibraryElement{Name: "renamed.go", Tags: []string{"keep"}}); err != nil {
		t.Fatalf("user edit: %v", err)
	}
	userView, err := sqliteStore.CreateView(ctx, "My Diagram", nil, nil)
	if err != nil {
		t.Fatalf("create user view: %v", err)
	}
	if _, err := sqliteStore.AddPlacement(ctx, userView.ID, fileID, 5, 6); err != nil {
		t.Fatalf("user placement: %v", err)
	}
	groupViewID := viewIDByName(t, sqliteStore, "alpha")
	renamedView := "Renamed Alpha"
	if _, err := sqliteStore.UpdateView(ctx, groupViewID, &renamedView, nil, nil, nil); err != nil {
		t.Fatalf("rename view: %v", err)
	}

	// Re-materialize the same repository at a new snapshot.
	input.SnapshotID = "snap-2"
	if _, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	got, err := sqliteStore.ElementByID(ctx, fileID)
	if err != nil {
		t.Fatalf("element: %v", err)
	}
	if got.Name != "renamed.go" {
		t.Fatalf("name = %q, want preserved renamed.go", got.Name)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Fatalf("tags = %v, want preserved [keep]", got.Tags)
	}
	placements, err := sqliteStore.ListElementPlacements(ctx, fileID)
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
	view, err := sqliteStore.ViewByID(ctx, groupViewID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Name != renamedView {
		t.Fatalf("view name = %q, want preserved %q", view.Name, renamedView)
	}
}

func TestApplyGroupMapAdjustsConnectorHandles(t *testing.T) {
	ctx := context.Background()
	sqliteStore, idx := openGroupMapStore(t)
	result, err := ApplyGroupMap(ctx, sqliteStore, idx, GroupMapInput{
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
	rows, err := sqliteStore.DB().QueryContext(ctx, `SELECT source_handle, target_handle FROM connectors`)
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
