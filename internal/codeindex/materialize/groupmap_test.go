package materialize

import (
	"context"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mapper"
	"github.com/mertcikla/tld/v2/internal/store"
)

func groupMapDataset() *mapper.Dataset {
	facts := []mapper.Fact{
		{ID: "id-a", Path: "src/alpha/a.go", DisplayName: "a.go", Language: "go"},
		{ID: "id-b", Path: "src/alpha/b.go", DisplayName: "b.go", Language: "go"},
		{ID: "id-c", Path: "src/beta/c.go", DisplayName: "c.go", Language: "go"},
		{ID: "id-d", Path: "src/beta/d.go", DisplayName: "d.go", Language: "go"},
	}
	return &mapper.Dataset{Snapshot: "snap-1", Profile: "community", Facts: facts}
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
	dataset := groupMapDataset()
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
		Dataset:        dataset,
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
	dataset := groupMapDataset()
	groups := []*community.Group{
		{Key: "leaf", Name: "leaf", Files: 3, Members: []int{0, 1, 2}},
	}
	input := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Dataset: dataset, Groups: groups,
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
	dataset := groupMapDataset()
	groups := []*community.Group{
		{Key: "one", Name: "one", Files: 1, Members: []int{0}},
	}
	input := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Dataset: dataset, Groups: groups,
	}
	if _, err := ApplyGroupMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	updated := GroupMapInput{
		RepositoryID: "repo-1", RepositoryName: "demo", SnapshotID: "snap-1",
		Dataset: dataset,
		Groups:  []*community.Group{{Key: "two", Name: "two", Files: 2, Members: []int{1, 2}}},
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
