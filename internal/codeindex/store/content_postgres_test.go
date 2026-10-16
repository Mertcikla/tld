//go:build integration

package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func TestPostgresContentOrgIDUpgrade(t *testing.T) {
	for _, orgID := range []uuid.UUID{uuid.Nil, uuid.New()} {
		t.Run(orgID.String(), func(t *testing.T) {
			st := openPostgresTestStore(t)
			ctx := app.WithTenantOrgID(context.Background(), orgID)
			// Recreate the schema installed before the tenant key repair.
			for _, table := range []string{"codeindex_blobs", "codeindex_artifacts"} {
				if _, err := st.bun.ExecContext(ctx, `ALTER TABLE `+table+`
					ALTER COLUMN org_id DROP DEFAULT,
					ALTER COLUMN org_id TYPE TEXT USING org_id::text,
					ALTER COLUMN org_id SET DEFAULT '00000000-0000-0000-0000-000000000000'`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := st.bun.ExecContext(ctx, `DELETE FROM bun_migrations WHERE name = '20261013000100'`); err != nil {
				t.Fatal(err)
			}

			text := []byte("package upgrade\nfunc A() {}\n")
			artifact := []byte("scip-index-bytes")
			src := &graph.Source{Path: "a.go", Text: text, Hash: graph.Hash(text), Language: "go"}
			g := graph.NewGraph("1", "2")
			g.Sources[src.Path] = src
			g.ProjectArtifacts["go|."] = graph.ProjectArtifact{Fingerprint: "fp", Data: artifact}
			snap := &pb.Snapshot{Id: g.SnapshotID, RepositoryId: g.RepositoryID, Sources: []*pb.SourceFile{{Path: src.Path, Hash: src.Hash}}}
			if err := st.Publish(ctx, "/repo", snap, g); err != nil {
				t.Fatal(err)
			}
			if err := dbrepo.ApplyEmbeddedMigrations(ctx, st.bun, assets.FS, "migrations/postgres"); err != nil {
				t.Fatal(err)
			}
			stored, err := st.LoadGraph(ctx, snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			if source := stored.Sources[src.Path]; source == nil || string(source.Text) != string(text) {
				t.Fatalf("source after upgrade = %+v", source)
			}
			if got := stored.ProjectArtifacts["go|."]; got.Fingerprint != "fp" || string(got.Data) != string(artifact) {
				t.Fatalf("artifact after upgrade = %+v", got)
			}
			if err := st.DeleteSnapshot(ctx, snap.Id); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"codeindex_blobs", "codeindex_artifacts"} {
				var count int
				if err := st.bun.NewRaw(`SELECT COUNT(*) FROM `+table).Scan(ctx, &count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("%s retained %d orphan rows", table, count)
				}
			}
		})
	}
}
