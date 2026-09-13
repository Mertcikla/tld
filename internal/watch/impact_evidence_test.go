package watch

import (
	"context"
	"testing"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestDetectObservedRelationshipsMapsBindings(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)

	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "services/checkout/service.go", "package checkout\n\nfunc Process() {\n\tCheck()\n}\n")
	writeFile(t, repo, "services/fraud/client.go", "package fraud\n\nfunc Check() {}\n")

	initial, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("initial scan files=%d parsed=%d symbols=%d mode=%s", initial.FilesSeen, initial.FilesParsed, initial.SymbolsSeen, initial.Mode)

	elements := map[string]*workspace.Element{
		"checkout": {Name: "Checkout", FilePath: "services/checkout/**"},
		"fraud":    {Name: "Fraud", FilePath: "services/fraud/**"},
	}
	relationships, err := DetectObservedRelationships(context.Background(), store, RelationshipOptions{
		RepoRoot:     repo,
		ChangedFiles: []string{"services/checkout/service.go"},
		Elements:     elements,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(relationships) == 0 {
		t.Fatalf("expected at least one observed relationship, got none")
	}
	found := false
	for _, rel := range relationships {
		if rel.SourceRef == "checkout" && rel.TargetRef == "fraud" {
			found = true
			if rel.Line == 0 || rel.File == "" {
				t.Fatalf("relationship missing location: %+v", rel)
			}
			if !rel.Observed {
				t.Fatalf("relationship should be observed: %+v", rel)
			}
		}
	}
	if !found {
		t.Fatalf("expected checkout -> fraud relationship, got %+v", relationships)
	}

	report := AnalyzeImpact(ImpactOptions{
		RepoRoot:      repo,
		Elements:      elements,
		ChangedFiles:  map[string]tldgit.WorktreeChange{"services/checkout/service.go": tldgit.WorktreeUpdated},
		Relationships: relationships,
	})
	if !hasFinding(report.Findings, "possible_new_relationship") {
		t.Fatalf("expected possible_new_relationship finding, got %+v", report.Findings)
	}
}
