package materialize

import (
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
