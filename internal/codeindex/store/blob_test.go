package store

import (
	"bytes"
	"context"
	"math/rand/v2"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func TestBlobCodec(t *testing.T) {
	noise := make([]byte, 4096)
	_, _ = rand.NewChaCha8([32]byte{1}).Read(noise)
	for _, tc := range []struct {
		name       string
		raw        []byte
		compressed bool
	}{
		{"nil", nil, false},
		{"empty", []byte{}, false},
		{"short", []byte("package sample"), false},
		{"gzip prefix", []byte{0x1f, 0x8b, 8, 0}, false},
		{"incompressible", noise, false},
		{"large", bytes.Repeat([]byte(testSource), 100), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, flags, err := compressBlob(tc.raw, blobContentGzip)
			if err != nil {
				t.Fatal(err)
			}
			if (flags != 0) != tc.compressed || len(encoded) > len(tc.raw) {
				t.Fatalf("compression = %d, size %d -> %d", flags, len(tc.raw), len(encoded))
			}
			decoded, err := decompressBlob(encoded, flags, blobContentGzip)
			if err != nil || !bytes.Equal(decoded, tc.raw) || (tc.raw == nil) != (decoded == nil) {
				t.Fatalf("round trip mismatch: %v", err)
			}
			if tc.compressed {
				corrupt := bytes.Clone(encoded)
				corrupt[len(corrupt)-1] ^= 0xff
				if _, err := decompressBlob(corrupt, flags, blobContentGzip); err == nil {
					t.Fatal("accepted corrupt gzip checksum")
				}
				if _, err := decompressBlob(encoded[:len(encoded)-5], flags, blobContentGzip); err == nil {
					t.Fatal("accepted truncated gzip data")
				}
			}
		})
	}
	if _, err := decompressBlob(nil, 8, blobContentGzip); err == nil {
		t.Fatal("accepted unsupported encoding")
	}
}

func TestBlobCompression(t *testing.T) {
	st, handle := openTestStore(t)
	t.Cleanup(func() { _ = handle.Close() })
	testBlobCompression(t, st)
}

func testBlobCompression(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	text := bytes.Repeat([]byte(testSource), 100)
	cache := `{"padding":"` + strings.Repeat("cache", 1000) + `"}`
	src := &graph.Source{Path: "a.go", Text: text, Hash: graph.Hash(text), Language: "go", SyntaxCache: cache, FileCache: cache}
	g := graph.NewGraph("1", "2")
	g.Sources[src.Path] = src
	small := &graph.Source{Path: "small.go", Text: []byte("package sample"), Language: "go", SyntaxCache: "tiny cache"}
	small.Hash = graph.Hash(small.Text)
	g.Sources[small.Path] = small
	snap := &pb.Snapshot{Id: g.SnapshotID, RepositoryId: g.RepositoryID,
		Sources: []*pb.SourceFile{{Path: src.Path, Hash: src.Hash}, {Path: small.Path, Hash: small.Hash}}}
	for _, id := range []string{"2", "3"} {
		snap.Id = id
		if err := st.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
		loaded, err := st.LoadGraph(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range g.Sources {
			got := loaded.Sources[path]
			if got == nil || !bytes.Equal(got.Text, want.Text) || got.SyntaxCache != want.SyntaxCache || got.FileCache != want.FileCache {
				t.Fatalf("source %s did not round trip", path)
			}
			if got, err := st.Source(ctx, want.Hash); err != nil || !bytes.Equal(got, want.Text) {
				t.Fatalf("source lookup %s: %v", path, err)
			}
		}
	}
	var content, syntax, file []byte
	var compression, count int
	if err := st.bun.NewRaw(`SELECT content, syntax_cache, file_cache, compression FROM codeindex_blobs WHERE hash = ?`, src.Hash).Scan(ctx, &content, &syntax, &file, &compression); err != nil {
		t.Fatal(err)
	}
	if compression != blobGzipMask || len(content)+len(syntax)+len(file) >= (len(text)+2*len(cache))/2 {
		t.Fatalf("expected all payloads compressed: flags=%d, size=%d", compression, len(content)+len(syntax)+len(file))
	}
	if err := st.bun.NewRaw(`SELECT compression FROM codeindex_blobs WHERE hash = ?`, small.Hash).Scan(ctx, &compression); err != nil || compression != 0 {
		t.Fatalf("small blob compression = %d: %v", compression, err)
	}
	if err := st.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_blobs`).Scan(ctx, &count); err != nil || count != 2 {
		t.Fatalf("deduplication: count=%d, err=%v", count, err)
	}
	// Source lookup need only decompress content, even when cache bytes are bad.
	if _, err := st.bun.NewRaw(`UPDATE codeindex_blobs SET syntax_cache = ? WHERE hash = ?`, []byte("broken"), src.Hash).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Source(ctx, src.Hash); err != nil || !bytes.Equal(got, text) {
		t.Fatalf("source lookup with corrupt cache: %v", err)
	}
	if _, err := st.LoadGraph(ctx, snap.Id); err == nil {
		t.Fatal("graph load accepted corrupt compressed cache")
	}
	if _, err := st.bun.NewRaw(`UPDATE codeindex_blobs SET content = ? WHERE hash = ?`, []byte("broken"), src.Hash).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Source(ctx, src.Hash); err == nil {
		t.Fatal("source lookup accepted corrupt compressed content")
	}
}

func TestBlobCompressionUpgrade(t *testing.T) {
	st, handle := openTestStore(t)
	t.Cleanup(func() { _ = handle.Close() })
	testBlobCompressionUpgrade(t, st)
}

func testBlobCompressionUpgrade(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	// Recreate the previous cache column types and seed a legacy blob.
	if st.Dialect() == dbrepo.DialectPostgres {
		if _, err := st.bun.ExecContext(ctx, `ALTER TABLE codeindex_blobs
			ALTER COLUMN syntax_cache DROP DEFAULT,
			ALTER COLUMN syntax_cache TYPE TEXT USING convert_from(syntax_cache, 'UTF8'),
			ALTER COLUMN syntax_cache SET DEFAULT '',
			ALTER COLUMN file_cache DROP DEFAULT,
			ALTER COLUMN file_cache TYPE TEXT USING convert_from(file_cache, 'UTF8'),
			ALTER COLUMN file_cache SET DEFAULT ''`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.bun.ExecContext(ctx, `ALTER TABLE codeindex_blobs DROP COLUMN compression`); err != nil {
		t.Fatal(err)
	}
	text := []byte{0x1f, 0x8b, 8, 0, 0xff}
	cache := `{"unicode":"ç日本語"}`
	if _, err := st.bun.NewRaw(`INSERT INTO codeindex_blobs (org_id, hash, language, content, syntax_cache, file_cache) VALUES (?, ?, ?, ?, ?, ?)`, scope(ctx).value(), graph.Hash(text), "go", text, cache, "file cache").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.bun.ExecContext(ctx, `DELETE FROM bun_migrations WHERE name = '20261014000300'`); err != nil {
		t.Fatal(err)
	}
	dir := "migrations"
	if st.Dialect() == dbrepo.DialectPostgres {
		dir += "/postgres"
	}
	if err := dbrepo.ApplyEmbeddedMigrations(ctx, st.bun, assets.FS, dir); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Source(ctx, graph.Hash(text)); err != nil || !bytes.Equal(got, text) {
		t.Fatalf("legacy source after upgrade: %v", err)
	}
	// Publishing a reference to the existing row must preserve its legacy data.
	g := graph.NewGraph("1", "2")
	g.Sources["old.go"] = &graph.Source{Path: "old.go", Text: text, Hash: graph.Hash(text), Language: "go"}
	snap := &pb.Snapshot{Id: "2", RepositoryId: "1", Sources: []*pb.SourceFile{{Path: "old.go", Hash: graph.Hash(text)}}}
	if err := st.Publish(ctx, "/repo", snap, g); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadGraph(ctx, snap.Id)
	if err != nil {
		t.Fatal(err)
	}
	src := loaded.Sources["old.go"]
	if src == nil || !bytes.Equal(src.Text, text) || src.SyntaxCache != cache || src.FileCache != "file cache" {
		t.Fatalf("legacy source/cache changed: %+v", src)
	}
	if err := dbrepo.ApplyEmbeddedMigrations(ctx, st.bun, assets.FS, dir); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
}
