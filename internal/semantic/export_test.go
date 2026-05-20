package semantic

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/semantic/semanticpb"
	"github.com/mertcikla/tld/v2/internal/watch"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestBuilderSymbolRecordProjectsRawWatchGraph(t *testing.T) {
	end := 8
	repo := watch.Repository{ID: 1, RepoRoot: "/repo/orders", Branch: sql.NullString{String: "main", Valid: true}}
	symbols := []watch.Symbol{
		{ID: 1, RepositoryID: 1, StableKey: "go:cmd/orders/main.go:function:Handle", Name: "Handle", QualifiedName: "Handle", Kind: "function", FilePath: "cmd/orders/main.go", StartLine: 3, EndLine: &end, RawJSON: `{"description":"Handles checkout orders.","raw_signature":"public static func Handle(ctx context.Context) error {"}`},
		{ID: 2, RepositoryID: 1, StableKey: "go:internal/payments/client.go:function:Charge", Name: "Charge", QualifiedName: "Charge", Kind: "function", FilePath: "internal/payments/client.go", StartLine: 2, EndLine: &end, RawJSON: `{}`},
	}
	facts := []watch.Fact{
		{RepositoryID: 1, FilePath: "cmd/orders/main.go", StableKey: "env", Type: "config.env", Enricher: "config.concrete_bindings", SubjectKind: "symbol", SubjectStableKey: symbols[0].StableKey, StartLine: 4, Confidence: 0.92, Name: "DB_HOST", Tags: []string{"owner:ord_proc"}, AttributesJSON: `{"key":"DB_HOST"}`},
		{RepositoryID: 1, FilePath: "cmd/orders/main.go", StableKey: "topic", Type: "messaging.topic", Enricher: "messaging.concrete_bindings", SubjectKind: "symbol", SubjectStableKey: symbols[0].StableKey, StartLine: 5, Confidence: 0.9, Name: "orders.created", AttributesJSON: `{"name":"orders.created"}`},
	}
	refs := []watch.Reference{{RepositoryID: 1, SourceSymbolID: 1, TargetSymbolID: 2, Kind: "call"}}

	b := newBuilder(repo, symbols, facts, refs, map[string]string{symbols[0].StableKey: "identity:Handle"})
	record := b.symbolRecord(symbols[0])
	if !strings.HasPrefix(record.GetUrn(), "urn:tld:symbol:") || !strings.HasSuffix(record.GetUrn(), ":identity.Handle") {
		t.Fatalf("urn = %q", record.GetUrn())
	}
	if record.GetDomainBoundary() != "order_processing" {
		t.Fatalf("domain = %q", record.GetDomainBoundary())
	}
	if record.GetEntityType() != semanticpb.EntityType_ENTITY_TYPE_FUNCTION {
		t.Fatalf("entity type = %v", record.GetEntityType())
	}
	if record.GetRawSignature() != "func Handle(ctx context.Context) error" {
		t.Fatalf("signature = %q", record.GetRawSignature())
	}
	if len(record.GetRelationships()) != 1 || record.GetRelationships()[0].GetRelationshipType() != "depends_on" {
		t.Fatalf("relationships = %+v", record.GetRelationships())
	}
	if got := strings.Join(record.GetSystemContext().GetExternalBindings(), ","); !strings.Contains(got, "EnvKey: DB_HOST") || !strings.Contains(got, "Kafka: topic/orders.created") {
		t.Fatalf("external bindings = %q", got)
	}
	data, err := protojson.Marshal(record)
	if err != nil {
		t.Fatalf("protojson marshal: %v", err)
	}
	if !strings.Contains(string(data), `"symbolName"`) || !strings.Contains(string(data), `"systemContext"`) {
		t.Fatalf("expected protobuf JSON names, got %s", data)
	}
}

func TestFlattenMarkdownUsesRulebookOrder(t *testing.T) {
	record := &semanticpb.SymbolContext{
		Urn:                 "urn:tld:symbol:test",
		SymbolName:          "Handle",
		EntityType:          semanticpb.EntityType_ENTITY_TYPE_FUNCTION,
		SourceLanguage:      "go",
		DomainBoundary:      "orders",
		RawSignature:        "public static func Handle() {}",
		NaturalLanguageDocs: "Handles orders.",
		Relationships: []*semanticpb.Relationship{
			{TargetUrn: "urn:tld:symbol:dep", RelationshipType: "depends_on", Description: "Depends On: Charge"},
		},
		SystemContext: &semanticpb.SystemContext{Triggers: []string{"HTTP: POST /orders"}},
	}
	out := FlattenMarkdown(record)
	wantOrder := []string{"# Handle", "Domain: orders", "## Identity", "## Signature", "## Documentation", "## Dependencies", "## System Context"}
	last := -1
	for _, want := range wantOrder {
		idx := strings.Index(out, want)
		if idx < 0 {
			t.Fatalf("missing %q in\n%s", want, out)
		}
		if idx <= last {
			t.Fatalf("%q appeared out of order in\n%s", want, out)
		}
		last = idx
	}
	if !strings.Contains(out, "- Depends On: `urn:tld:symbol:dep`") {
		t.Fatalf("relationship label not standardized:\n%s", out)
	}
}
