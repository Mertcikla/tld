package materialize

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mapper"
	"github.com/mertcikla/tld/v2/internal/store"
)

func TestApplyMapHierarchyUpsertAndPrune(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	dataset, bins := testMapData(t)
	input := MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
	}
	options := MapOptions{Progress: func(current, total int, detail string) {}}
	result, err := ApplyMap(ctx, sqliteStore, idx, input, options)
	if err != nil {
		t.Fatalf("apply map: %v", err)
	}
	if result.ViewID == 0 || result.Views < 4 || result.Elements < 6 {
		t.Fatalf("result = %+v", result)
	}
	var rootPlacements int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM placements WHERE view_id = ?`, result.ViewID).Scan(&rootPlacements); err != nil {
		t.Fatal(err)
	}
	if rootPlacements == 0 {
		t.Fatal("root view has no placements")
	}

	viewsBefore := countTable(t, sqliteStore, "views")
	elementsBefore := countTable(t, sqliteStore, "elements")

	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{
		LogicalKey:   "map|stale|entry",
		Kind:         cstore.MappingElement,
		ResourceID:   0,
		RepositoryID: input.RepositoryID,
		SnapshotID:   input.SnapshotID,
	}}); err != nil {
		t.Fatalf("seed stale mapping: %v", err)
	}

	second, err := ApplyMap(ctx, sqliteStore, idx, input, MapOptions{})
	if err != nil {
		t.Fatalf("second apply map: %v", err)
	}
	if second.ViewID != result.ViewID {
		t.Fatalf("view id changed: %d -> %d", result.ViewID, second.ViewID)
	}
	if second.Pruned != 1 {
		t.Fatalf("pruned = %d, want 1", second.Pruned)
	}
	if got := countTable(t, sqliteStore, "views"); got != viewsBefore {
		t.Fatalf("views changed on rerun: %d -> %d", viewsBefore, got)
	}
	if got := countTable(t, sqliteStore, "elements"); got != elementsBefore {
		t.Fatalf("elements changed on rerun: %d -> %d", elementsBefore, got)
	}
	if _, ok, err := idx.MappingByLogicalKey(ctx, "map|stale|entry"); err != nil || ok {
		t.Fatalf("stale mapping not pruned (ok=%v err=%v)", ok, err)
	}
}

func TestApplyMapNestsUnderWorkspaceRoot(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	dataset, bins := testMapData(t)
	result, err := ApplyMap(ctx, sqliteStore, idx, MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply map: %v", err)
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

func TestApplyMapUsesInferredDomainNames(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/demo"
	facts := []mapper.Fact{
		{ID: "a1", Path: "src/auth/a.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "a2", Path: "src/auth/b.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "x1", Path: "src/xml/c.go", DisplayName: "ParseXml", Language: "go"},
		{ID: "x2", Path: "src/xml/d.go", DisplayName: "ParseXml", Language: "go"},
	}
	vectors := [][]float64{{1, 0, 0}, {1, 0, 0}, {0, 1, 0}, {0, 1, 0}}
	dataset := &mapper.Dataset{Snapshot: "snap-1", Profile: "p1", Root: &root, Facts: facts, Vectors: vectors}
	options := mapper.DefaultOptions()
	pipeline, err := mapper.RunPipeline(dataset.Vectors, &options)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	binOptions := mapper.DefaultBinOptions()
	binOptions.FolderPoolingThreshold = 2
	bins, err := mapper.BuildBins(dataset, pipeline, &binOptions)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}
	if _, err := ApplyMap(ctx, sqliteStore, idx, MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: root,
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
	}, MapOptions{}); err != nil {
		t.Fatalf("apply map: %v", err)
	}

	counts := elementNameCounts(t, sqliteStore)
	for name, want := range map[string]int{
		"demo":  1, // top element
		"auth":  4, // cluster + bin + aggregated "src" folder + "src/auth" base name
		"parse": 2, // cluster + its bin
		"xml":   1, // "src/xml" folder shows only its base name
	} {
		if counts[name] != want {
			t.Fatalf("element name %q count = %d, want %d (all: %v)", name, counts[name], want, counts)
		}
	}
	for name := range counts {
		if strings.Contains(name, " · ") {
			t.Fatalf("element name %q still joins multiple names: %v", name, counts)
		}
	}
}

func TestApplyMapCreatesFileConnectors(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	root := "/repo/demo"
	facts := []mapper.Fact{
		{ID: "a1", Path: "src/auth/a.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "a2", Path: "src/auth/b.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "x1", Path: "src/xml/c.go", DisplayName: "ParseXml", Language: "go"},
		{ID: "x2", Path: "src/xml/d.go", DisplayName: "ParseXml", Language: "go"},
	}
	vectors := [][]float64{{1, 0, 0}, {1, 0, 0}, {0, 1, 0}, {0, 1, 0}}
	dataset := &mapper.Dataset{Snapshot: "snap-1", Profile: "p1", Root: &root, Facts: facts, Vectors: vectors}
	options := mapper.DefaultOptions()
	pipeline, err := mapper.RunPipeline(dataset.Vectors, &options)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	binOptions := mapper.DefaultBinOptions()
	binOptions.FolderPoolingThreshold = 2
	bins, err := mapper.BuildBins(dataset, pipeline, &binOptions)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}
	result, err := ApplyMap(ctx, sqliteStore, idx, MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: root,
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
		Edges: []MapEdge{
			{FromFactID: "a1", ToFactID: "x1"}, // duplicated below
			{FromFactID: "a1", ToFactID: "x1"},
			{FromFactID: "x1", ToFactID: "a1"}, // reverse merges into bidirectional
			{FromFactID: "a2", ToFactID: "x2"}, // forward only
			{FromFactID: "a2", ToFactID: "a2"}, // self edge dropped
		},
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply map: %v", err)
	}
	if result.Connectors != 2 {
		t.Fatalf("connectors = %d, want 2 (deduped + merged)", result.Connectors)
	}
	rows, err := sqliteStore.DB().QueryContext(ctx, `SELECT direction, COUNT(*) FROM connectors GROUP BY direction ORDER BY direction`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]int{}
	for rows.Next() {
		var direction string
		var count int
		if err := rows.Scan(&direction, &count); err != nil {
			t.Fatal(err)
		}
		got[direction] = count
	}
	if got["both"] != 1 || got["forward"] != 1 {
		t.Fatalf("directions = %v, want both=1 forward=1", got)
	}
	// Cross-cluster endpoints stay off-view: they are not relocated into the
	// connector's owner view.
	var relocated int
	if err := sqliteStore.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM connectors c
		WHERE (SELECT COUNT(*) FROM placements p
		       WHERE p.view_id = c.view_id AND p.element_id IN (c.source_element_id, c.target_element_id)) <> 0`).Scan(&relocated); err != nil {
		t.Fatal(err)
	}
	if relocated != 0 {
		t.Fatalf("%d connectors relocated endpoints into their view", relocated)
	}
}

func TestApplyMapMaterializesImports(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	dataset, bins := twoClusterData(t)
	result, err := ApplyMap(ctx, sqliteStore, idx, MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
		Imports: []MapImport{
			{FileFactID: "a1", Import: "flask"},
			{FileFactID: "a1", Import: "celery"},
			{FileFactID: "a1", Import: "flask"}, // duplicate, must be deduped
			{FileFactID: "x1", Import: "flask"},
		},
	}, MapOptions{})
	if err != nil {
		t.Fatalf("apply map: %v", err)
	}
	if result.Connectors != 3 {
		t.Fatalf("import connectors = %d, want 3 (deduped)", result.Connectors)
	}
	counts := elementNameCounts(t, sqliteStore)
	if counts["External"] != 1 {
		t.Fatalf("External element count = %d, want 1 (%v)", counts["External"], counts)
	}
	if counts["flask"] != 1 || counts["celery"] != 1 {
		t.Fatalf("import elements not deduped: %v", counts)
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
	// Import connectors stay off-view.
	var relocated int
	if err := sqliteStore.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM connectors c
		WHERE (SELECT COUNT(*) FROM placements p
		       WHERE p.view_id = c.view_id AND p.element_id IN (c.source_element_id, c.target_element_id)) <> 0`).Scan(&relocated); err != nil {
		t.Fatal(err)
	}
	if relocated != 0 {
		t.Fatalf("%d connectors relocated endpoints into their view", relocated)
	}
}

func TestApplyMapClearsLegacyGroupKinds(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	dataset, bins := twoClusterData(t)
	input := MapInput{
		RepositoryID:   "repo-1",
		RepositoryName: "demo",
		RepositoryRoot: "/repo/demo",
		SnapshotID:     "snap-1",
		RunID:          "run-1",
		Dataset:        dataset,
		Bins:           bins,
	}
	if _, err := ApplyMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("apply map: %v", err)
	}
	// Simulate elements materialized before groups stopped carrying a kind.
	if _, err := sqliteStore.DB().ExecContext(ctx, `UPDATE elements SET kind = 'cluster' WHERE kind = '' OR kind IS NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMap(ctx, sqliteStore, idx, input, MapOptions{}); err != nil {
		t.Fatalf("second apply map: %v", err)
	}
	var legacy int
	if err := sqliteStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE kind IN ('bin', 'cluster')`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 0 {
		t.Fatalf("%d elements still carry bin/cluster kinds", legacy)
	}
}

func twoClusterData(t *testing.T) (*mapper.Dataset, *mapper.BinningResult) {
	t.Helper()
	root := "/repo/demo"
	facts := []mapper.Fact{
		{ID: "a1", Path: "src/auth/a.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "a2", Path: "src/auth/b.go", DisplayName: "AuthToken", Language: "go"},
		{ID: "x1", Path: "src/xml/c.go", DisplayName: "ParseXml", Language: "go"},
		{ID: "x2", Path: "src/xml/d.go", DisplayName: "ParseXml", Language: "go"},
	}
	vectors := [][]float64{{1, 0, 0}, {1, 0, 0}, {0, 1, 0}, {0, 1, 0}}
	dataset := &mapper.Dataset{Snapshot: "snap-1", Profile: "p1", Root: &root, Facts: facts, Vectors: vectors}
	options := mapper.DefaultOptions()
	pipeline, err := mapper.RunPipeline(dataset.Vectors, &options)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	binOptions := mapper.DefaultBinOptions()
	binOptions.FolderPoolingThreshold = 2
	bins, err := mapper.BuildBins(dataset, pipeline, &binOptions)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}
	return dataset, bins
}

func elementNameCounts(t *testing.T, sqliteStore *store.SQLiteStore) map[string]int {
	t.Helper()
	rows, err := sqliteStore.DB().Query(`SELECT name, COUNT(*) FROM elements GROUP BY name`)
	if err != nil {
		t.Fatalf("query elements: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			t.Fatal(err)
		}
		out[name] = count
	}
	return out
}

func testMapData(t *testing.T) (*mapper.Dataset, *mapper.BinningResult) {
	t.Helper()
	root := "/repo/demo"
	paths := []string{"src/a.go", "src/b.go", "src/deep/c.go", "src/deep/d.go"}
	facts := make([]mapper.Fact, len(paths))
	vectors := make([][]float64, len(paths))
	for i, path := range paths {
		facts[i] = mapper.Fact{ID: "id-" + string(rune('a'+i)), Path: path, DisplayName: "fact", Language: "go"}
		vectors[i] = []float64{1, 0, 0}
	}
	dataset := &mapper.Dataset{Snapshot: "snap-1", Profile: "p1", Root: &root, Facts: facts, Vectors: vectors}
	options := mapper.DefaultOptions()
	options.BlockSize = 8
	pipeline, err := mapper.RunPipeline(dataset.Vectors, &options)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	binOptions := mapper.DefaultBinOptions()
	binOptions.FolderPoolingThreshold = 2
	bins, err := mapper.BuildBins(dataset, pipeline, &binOptions)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}
	return dataset, bins
}

func countTable(t *testing.T, sqliteStore *store.SQLiteStore, table string) int {
	t.Helper()
	var count int
	if err := sqliteStore.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
