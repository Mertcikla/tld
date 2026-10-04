package community

import (
	"encoding/json"
	"fmt"
	"testing"
)

func testFiles(paths ...string) []File {
	out := make([]File, len(paths))
	for i, path := range paths {
		out[i] = File{ID: fmt.Sprintf("f%02d", i), Path: path}
	}
	return out
}

// planted builds communities of fully connected nodes joined by weak bridges.
func planted(communities, size int, internal, weak float64) ([]File, []Edge) {
	paths := make([]string, 0, communities*size)
	for c := 0; c < communities; c++ {
		for i := 0; i < size; i++ {
			paths = append(paths, fmt.Sprintf("pkg%d/file%02d.go", c, i))
		}
	}
	files := testFiles(paths...)
	edges := make([]Edge, 0, communities*size*size)
	for c := 0; c < communities; c++ {
		base := c * size
		for i := 0; i < size; i++ {
			for j := i + 1; j < size; j++ {
				edges = append(edges, Edge{A: base + i, B: base + j, Weight: internal})
			}
		}
		if c > 0 {
			edges = append(edges, Edge{A: 0, B: c * size, Weight: weak})
		}
	}
	return files, edges
}

func rootMemberCommunities(t *testing.T, result *Result, size int) map[int]map[int]struct{} {
	t.Helper()
	out := map[int]map[int]struct{}{}
	for groupIndex, group := range result.Groups {
		if group.Isolated {
			continue
		}
		set := map[int]struct{}{}
		var visit func(item *Group)
		visit = func(item *Group) {
			for _, member := range item.Members {
				set[member/size] = struct{}{}
			}
			for _, child := range item.Children {
				visit(child)
			}
		}
		visit(group)
		if len(set) > 1 {
			t.Fatalf("root group %q mixes planted communities: %v", group.Name, set)
		}
		out[groupIndex] = set
	}
	return out
}

func TestBuildPlantedCommunities(t *testing.T) {
	files, edges := planted(3, 8, 10, 1)
	result, err := Build(files, edges, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	recovered := rootMemberCommunities(t, result, 8)
	if len(recovered) < 2 {
		t.Fatalf("expected planted communities to split, got %d root groups", len(recovered))
	}
	total := 0
	for _, group := range result.Groups {
		total += group.Files
	}
	if total != len(files) {
		t.Fatalf("grouped files = %d, want %d", total, len(files))
	}
	if result.Metrics.IsolatedFiles != 0 {
		t.Fatalf("isolated files = %d, want 0", result.Metrics.IsolatedFiles)
	}
	if result.Metrics.Modularity <= 0 {
		t.Fatalf("modularity = %f, want positive", result.Metrics.Modularity)
	}
}

func TestBuildDeterministic(t *testing.T) {
	files, edges := planted(4, 6, 5, 0.5)
	first, err := Build(files, edges, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	reversed := make([]Edge, len(edges))
	for i, e := range edges {
		reversed[len(edges)-1-i] = e
	}
	second, err := Build(files, reversed, Options{})
	if err != nil {
		t.Fatalf("build reversed: %v", err)
	}
	firstJSON, _ := json.Marshal(first.Groups)
	secondJSON, _ := json.Marshal(second.Groups)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("grouping depends on edge order:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestBuildSubdividesLargeCommunities(t *testing.T) {
	// Two loosely coupled blobs, each internally a dense ring with chords so
	// Louvain finds subcommunities larger than the leaf budget.
	const size = 60
	var edges []Edge
	for blob := 0; blob < 2; blob++ {
		base := blob * size
		for i := 0; i < size; i++ {
			edges = append(edges, Edge{A: base + i, B: base + (i+1)%size, Weight: 1})
			edges = append(edges, Edge{A: base + i, B: base + (i+7)%size, Weight: 1})
		}
	}
	for i := 0; i < 10; i++ {
		edges = append(edges, Edge{A: i, B: size + i, Weight: 0.1})
	}
	paths := make([]string, 2*size)
	for i := range paths {
		paths[i] = fmt.Sprintf("pkg/file%03d.go", i)
	}
	files := testFiles(paths...)
	result, err := Build(files, edges, Options{MaxLeafFiles: 20, MaxDepth: 3, MinRootGroups: 2})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	hasChildren := false
	var check func(item *Group)
	check = func(item *Group) {
		if len(item.Children) > 0 {
			hasChildren = true
			for _, child := range item.Children {
				check(child)
			}
		}
	}
	for _, group := range result.Groups {
		check(group)
	}
	if !hasChildren {
		for _, group := range result.Groups {
			t.Logf("root %q files=%d members=%d children=%d", group.Name, group.Files, len(group.Members), len(group.Children))
		}
		t.Fatal("expected groups larger than the leaf budget to subdivide")
	}
}

func TestBuildIsolatesBecomeDirectoryGroups(t *testing.T) {
	files := testFiles("docs/guide.md", "docs/api.md", "scripts/build.sh", "README.md")
	result, err := Build(files, nil, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	byName := map[string]*Group{}
	for _, group := range result.Groups {
		if group.Isolated {
			byName[group.Name] = group
		}
	}
	for _, name := range []string{"docs", "scripts", "unmapped"} {
		if byName[name] == nil {
			t.Fatalf("missing isolated group %q (got %v)", name, byName)
		}
	}
	if result.Metrics.IsolatedFiles != 4 {
		t.Fatalf("isolated files = %d, want 4", result.Metrics.IsolatedFiles)
	}
}

func TestBuildEmptyAndTiny(t *testing.T) {
	empty, err := Build(nil, nil, Options{})
	if err != nil {
		t.Fatalf("empty build: %v", err)
	}
	if len(empty.Groups) != 0 {
		t.Fatalf("empty build returned %d groups", len(empty.Groups))
	}

	tiny, err := Build(testFiles("a.go", "b.go"), []Edge{{A: 0, B: 1, Weight: 1}}, Options{})
	if err != nil {
		t.Fatalf("tiny build: %v", err)
	}
	if len(tiny.Groups) != 1 || tiny.Groups[0].Files != 2 {
		t.Fatalf("tiny build groups = %+v, want one group of two files", tiny.Groups)
	}
	if tiny.Metrics.IsolatedFiles != 0 {
		t.Fatalf("tiny isolated = %d, want 0", tiny.Metrics.IsolatedFiles)
	}
}

func TestBuildNamesFromDominantFolder(t *testing.T) {
	files := testFiles("internal/exec/runner.go", "internal/exec/plan.go", "internal/exec/import.go")
	edges := []Edge{{A: 0, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}, {A: 0, B: 2, Weight: 1}}
	result, err := Build(files, edges, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Groups) != 1 {
		for _, group := range result.Groups {
			t.Logf("group %q files=%d members=%v children=%d", group.Name, group.Files, group.Members, len(group.Children))
		}
		t.Fatalf("groups = %d, want 1", len(result.Groups))
	}
	group := result.Groups[0]
	if group.Name != "internal/exec" || group.Source != "folder" {
		t.Fatalf("group name = %q (%s), want internal/exec (folder)", group.Name, group.Source)
	}
	if group.Internal != 3 || group.External != 0 {
		t.Fatalf("internal/external = %v/%v, want 3/0", group.Internal, group.External)
	}
}

func TestBuildNameFallback(t *testing.T) {
	files := testFiles("a.go", "b.go")
	edges := []Edge{{A: 0, B: 1, Weight: 1}}
	result, err := Build(files, edges, Options{
		NameFallback: func(members []int) string { return "lexical name" },
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Name != "lexical name" {
		t.Fatalf("groups = %+v, want one lexically named group", result.Groups)
	}
}

func TestBuildGenericRootPrefersLexicalName(t *testing.T) {
	files := testFiles("internal/alpha/a.go", "internal/beta/b.go", "internal/gamma/c.go")
	edges := []Edge{{A: 0, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}, {A: 0, B: 2, Weight: 1}}
	result, err := Build(files, edges, Options{
		NameFallback: func(members []int) string { return "ledger pipeline" },
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(result.Groups))
	}
	if result.Groups[0].Name != "ledger pipeline" || result.Groups[0].Source != "lexical" {
		t.Fatalf("group name = %q (%s), want lexical ledger pipeline", result.Groups[0].Name, result.Groups[0].Source)
	}
}

func TestBuildContainerPrefixPrefersLexicalName(t *testing.T) {
	files := testFiles("frontend/src/api/a.ts", "frontend/src/pages/b.tsx", "frontend/src/utils/c.ts")
	edges := []Edge{{A: 0, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}, {A: 0, B: 2, Weight: 1}}
	result, err := Build(files, edges, Options{
		NameFallback: func(members []int) string { return "editor canvas" },
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(result.Groups))
	}
	if result.Groups[0].Name != "editor canvas" || result.Groups[0].Source != "lexical" {
		t.Fatalf("group name = %q (%s), want lexical editor canvas", result.Groups[0].Name, result.Groups[0].Source)
	}
}

func TestBuildIsolatedBucketsKeepFolderNames(t *testing.T) {
	// The lexical fallback must never relabel structural directory buckets.
	files := testFiles("docs/guide.md", "docs/api.md")
	result, err := Build(files, nil, Options{
		NameFallback: func(members []int) string { return "invented token" },
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Name != "docs" {
		t.Fatalf("isolated bucket = %+v, want docs", result.Groups)
	}
}

func TestGroupKeyStableAcrossEdgeChanges(t *testing.T) {
	files, edges := planted(2, 5, 5, 1)
	first, err := Build(files, edges, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	weighted := make([]Edge, len(edges))
	copy(weighted, edges)
	for i := range weighted {
		weighted[i].Weight *= 3
	}
	second, err := Build(files, weighted, Options{})
	if err != nil {
		t.Fatalf("build weighted: %v", err)
	}
	if len(first.Groups) != len(second.Groups) {
		t.Fatalf("group counts differ: %d vs %d", len(first.Groups), len(second.Groups))
	}
	for i := range first.Groups {
		if first.Groups[i].Key != second.Groups[i].Key {
			t.Fatalf("group key changed with edge weight: %s vs %s", first.Groups[i].Key, second.Groups[i].Key)
		}
	}
}

func TestFolderOf(t *testing.T) {
	cases := map[string]string{
		"a.go":               ".",
		"src/a.go":           "src",
		"src/deep/a.go":      "src/deep",
		"src/../lib/a.go":    "lib",
		`src\windows\a.go`:   "src/windows",
		"../outside/a.go":    "../outside",
		"/absolute/root/a.go": "absolute/root",
	}
	for input, want := range cases {
		if got := folderOf(input); got != want {
			t.Errorf("folderOf(%q) = %q, want %q", input, got, want)
		}
	}
}

func BenchmarkBuildLarge(b *testing.B) {
	const communities, size = 60, 20
	files, edges := planted(communities, size, 4, 0.2)
	opts := Options{MaxLeafFiles: 30, MaxDepth: 3}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Build(files, edges, opts); err != nil {
			b.Fatal(err)
		}
	}
}
