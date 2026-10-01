package parser

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// scanDir discovers the parser-relevant sources in dir and scans them, mirroring
// how the indexer feeds the discovered source set into Scan.
func scanDir(t *testing.T, dir string) []Fact {
	t.Helper()
	sources := map[string]*graph.Source{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != dir && SkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !IsInfraSource(d.Name(), rel) && !isCodeExt(filepath.Ext(rel)) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[rel] = &graph.Source{Path: rel, Text: b, Hash: graph.Hash(b)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := Scan(context.Background(), dir, sources)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestScanConfigFacts(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("compose.yml", "services:\n  api:\n    build: ./api\n    depends_on:\n      - db\n  db:\n    image: postgres:16\n")
	write("Dockerfile", "FROM golang:1.26\nEXPOSE 8080\nENV PORT=8080\nCMD [\"/app\"]\n")
	write("api/main.go", "package main\nimport (\n\t\"database/sql\"\n\t\"net/http\"\n)\nfunc main() {\n\thttp.HandleFunc(\"/login\", login)\n\t_, _ = sql.Open(\"postgres\", \"dsn\")\n}\n")
	write("serverless/serverless.yml", "service: example\nfunctions:\n  cleanup:\n    handler: handler.cleanup\n")
	write("k8s/deployment.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  template:\n    spec:\n      containers:\n        - image: redis:7\n")
	write("proto/auth.proto", "service Auth {\n  rpc Login(LoginRequest) returns (LoginResponse);\n}\n")

	facts := scanDir(t, dir)
	byKind := map[pb.FactKind]int{}
	bySubjectObject := map[string]bool{}
	for _, f := range facts {
		byKind[f.Kind]++
		bySubjectObject[f.Kind.String()+"|"+f.Subject+"|"+f.Object] = true
	}
	expect := []struct {
		kind    pb.FactKind
		subject string
		object  string
	}{
		{pb.FactKind_FACT_KIND_DEPLOYABLE, "api", "compose service"},
		{pb.FactKind_FACT_KIND_DEPLOYABLE, "db", "compose service"},
		{pb.FactKind_FACT_KIND_DEPLOYABLE, "web", "Deployment"},
		{pb.FactKind_FACT_KIND_DEPLOYABLE, "cleanup", "serverless function"},
		{pb.FactKind_FACT_KIND_DEPENDENCY, "api", "db"},
		{pb.FactKind_FACT_KIND_DATASTORE, "db", "postgres"},
		{pb.FactKind_FACT_KIND_DATASTORE, "web", "redis"},
		{pb.FactKind_FACT_KIND_RPC, "Auth", "Login"},
		{pb.FactKind_FACT_KIND_ENV, "root", "PORT"},
	}
	for _, e := range expect {
		if !bySubjectObject[e.kind.String()+"|"+e.subject+"|"+e.object] {
			t.Errorf("missing %s %s->%s; got %v", e.kind, e.subject, e.object, byKind)
		}
	}
	// Source call names alone cannot establish runtime roles. Explicit
	// contracts in this fixture still contribute their own route/RPC facts.
	if byKind[pb.FactKind_FACT_KIND_ROUTE] != 0 {
		t.Fatalf("code call was promoted to a route: %v", byKind)
	}
}

func TestCodePatternDoesNotTurnDictionaryGetIntoRoute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.py")
	if err := os.WriteFile(path, []byte("value = kwargs.get('file', None)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, fact := range codePatternFacts(path, "library.py", "root") {
		if fact.Kind == pb.FactKind_FACT_KIND_ROUTE {
			t.Fatalf("dictionary lookup became route: %+v", fact)
		}
	}
}

func TestScanRecoversAfterMalformedYAMLDocument(t *testing.T) {
	dir := t.TempDir()
	body := "apiVersion: apps/v1\n" +
		"kind: Deployment\n" +
		"metadata:\n" +
		"  name: web\n" +
		"---\n" +
		"foo: [unclosed\n" +
		"---\n" +
		"apiVersion: v1\n" +
		"kind: Deployment\n" +
		"metadata:\n" +
		"  name: api\n"
	if err := os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	facts := scanDir(t, dir)
	var first, second *Fact
	for i := range facts {
		if facts[i].Kind != pb.FactKind_FACT_KIND_DEPLOYABLE {
			continue
		}
		switch facts[i].Subject {
		case "web":
			first = &facts[i]
		case "api":
			second = &facts[i]
		}
	}
	if first == nil {
		t.Fatal("deployment before the malformed document was lost")
	}
	if second == nil {
		t.Fatal("document after the malformed one was dropped")
	}
	if first.Line != 2 {
		t.Errorf("deployment line = %d, want 2", first.Line)
	}
	if second.Line != 9 {
		t.Errorf("second deployment line = %d, want 9 (line offset lost in document split)", second.Line)
	}
}

func TestScanWorkspaceFacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte("go 1.22\n\nuse ./svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	facts := scanDir(t, dir)
	has := func(kind pb.FactKind, subject, object string) bool {
		for _, f := range facts {
			if f.Kind == kind && f.Subject == subject && f.Object == object {
				return true
			}
		}
		return false
	}
	if !has(pb.FactKind_FACT_KIND_WORKSPACE_MEMBER, "root", "./svc") {
		t.Error("go.work workspace member missing")
	}
}

func TestScanDoesNotDuplicateGoImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range scanDir(t, dir) {
		if f.Kind == pb.FactKind_FACT_KIND_IMPORT && f.Object == "fmt" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("fmt import facts = %d, want 1", count)
	}
}
