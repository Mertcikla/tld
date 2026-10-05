package materialize

import (
	"regexp"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/community"
)

func TestMapFileElementStampsRepositoryID(t *testing.T) {
	m := &mapMaterializer{input: MapInput{
		RepositoryID:   "repo-1",
		RepositoryRoot: "/work/repo",
		Files:          []community.File{{ID: "f1", Path: "src/main.go", DisplayName: "main.go"}},
	}}
	got := m.fileElement(0)
	if got.RepositoryID == nil || *got.RepositoryID != "repo-1" {
		t.Fatalf("repository_id = %v, want repo-1", got.RepositoryID)
	}
	if got.Repo == nil || *got.Repo != "/work/repo" {
		t.Fatalf("repo = %v, want /work/repo", got.Repo)
	}
}

func TestGroupTagMatchesFrontendUUIDPattern(t *testing.T) {
	pattern := regexp.MustCompile(`^group:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-a[0-9a-f]{3}-[0-9a-f]{12}$`)
	tag := groupTag("repo-1", "alpha")
	if !pattern.MatchString(tag) {
		t.Fatalf("groupTag = %q, want a version-4 variant-a UUID group tag", tag)
	}
	if tag != groupTag("repo-1", "alpha") {
		t.Fatalf("groupTag is not deterministic: %q", tag)
	}
	if tag == groupTag("repo-1", "beta") {
		t.Fatalf("distinct groups share a tag: %q", tag)
	}
}
