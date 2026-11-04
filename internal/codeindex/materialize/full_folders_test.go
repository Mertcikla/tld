package materialize

import (
	"context"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/mertcikla/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

func TestFullMapRollsDependenciesUpFolderHierarchy(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		edges []MapEdge
		want  []string
	}{
		{name: "same folder", files: []string{"app/a.go", "app/b.go"}, want: []string{}},
		{name: "siblings and duplicate dependencies", files: []string{"app/left/a.go", "app/right/b.go", "app/left/c.go"}, edges: []MapEdge{{FromFactID: "a", ToFactID: "b", Kind: "calls"}, {FromFactID: "a", ToFactID: "b", Kind: "references"}, {FromFactID: "c", ToFactID: "b", Kind: "implements"}}, want: []string{"app/left->app/right"}},
		{name: "separate deep branches", files: []string{"client/src/pages/a.go", "server/app/api/b.go"}, want: []string{"client/src/pages->server/app/api", "client/src->server/app", "client->server"}},
		{name: "unequal depths", files: []string{"client/src/pages/a.go", "server/b.go"}, want: []string{"client/src/pages->server", "client/src->server", "client->server"}},
		{name: "stop at shared ancestor", files: []string{"app/left/deep/a.go", "app/right/b.go"}, want: []string{"app/left/deep->app/right", "app/left->app/right"}},
		{name: "ancestor and descendant", files: []string{"app/a.go", "app/nested/b.go"}, want: []string{}},
		{name: "root file", files: []string{"a.go", "lib/b.go"}, want: []string{}},
		{name: "opposite directions", files: []string{"left/a.go", "right/b.go"}, edges: []MapEdge{{FromFactID: "a", ToFactID: "b", Kind: "calls"}, {FromFactID: "b", ToFactID: "a", Kind: "references"}}, want: []string{"left->right", "right->left"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ws, idx := openGroupMapStore(t)
			files := make([]community.File, 0, len(tc.files))
			for _, p := range tc.files {
				files = append(files, community.File{ID: strings.TrimSuffix(path.Base(p), ".go"), Path: p})
			}
			edges := tc.edges
			if edges == nil {
				edges = []MapEdge{{FromFactID: "a", ToFactID: "b", Kind: "calls", SourceLines: []uint32{3}, TargetLines: []uint32{7}}}
			}
			input := GroupMapInput{Full: true, RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "one", Files: files, Edges: edges}
			result, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{})
			if err != nil {
				t.Fatal(err)
			}
			mappings, err := idx.MappingsByRepository(ctx, "repo")
			if err != nil {
				t.Fatal(err)
			}
			directoryByID := map[int64]string{}
			viewByDirectory := map[string]int64{".": result.ViewID}
			before := map[string]int64{}
			for _, m := range mappings {
				before[m.LogicalKey] = m.ResourceID
				if !strings.HasPrefix(m.LogicalKey, "map|directory|repo|") {
					continue
				}
				p := strings.TrimPrefix(m.LogicalKey, "map|directory|repo|")
				if m.Kind == cstore.MappingElement {
					directoryByID[m.ResourceID] = p
				}
				if m.Kind == cstore.MappingView {
					viewByDirectory[strings.TrimSuffix(p, "|view")] = m.ResourceID
				}
			}
			got := []string{}
			for _, m := range mappings {
				if m.Kind != cstore.MappingConnector || !strings.HasPrefix(m.LogicalKey, "map|folder-edge|") {
					continue
				}
				c, err := ws.ConnectorByID(ctx, m.ResourceID)
				if err != nil {
					t.Fatal(err)
				}
				source, target := directoryByID[c.SourceElementID], directoryByID[c.TargetElementID]
				if source == "" || target == "" || source == target {
					t.Fatalf("invalid folder endpoints: %+v", c)
				}
				if c.ViewID != viewByDirectory[path.Dir(source)] {
					t.Fatalf("folder connector owned by wrong view: %+v", c)
				}
				if c.Direction != "forward" || (c.Label != nil && *c.Label != "") || (c.Relationship != nil && *c.Relationship != "") {
					t.Fatalf("folder summary not an unlabeled directed dependency: %+v", c)
				}
				if (c.SourceHandle != nil && strings.Contains(*c.SourceHandle, "-line-")) || (c.TargetHandle != nil && strings.Contains(*c.TargetHandle, "-line-")) {
					t.Fatal("folder summary inherited file symbol anchors")
				}
				got = append(got, source+"->"+target)
			}
			want := append([]string{}, tc.want...)
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("folder connectors=%v want %v", got, want)
			}
			input.SnapshotID = "two"
			if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
				t.Fatal(err)
			}
			after, err := idx.MappingsByRepository(ctx, "repo")
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(mappings) {
				t.Fatalf("rerun added duplicate resources: %d -> %d", len(mappings), len(after))
			}
			for _, m := range after {
				if before[m.LogicalKey] != m.ResourceID {
					t.Fatalf("rerun replaced %s", m.LogicalKey)
				}
			}
		})
	}
}
