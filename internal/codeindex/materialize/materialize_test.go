package materialize

import (
	"regexp"
	"testing"

	"github.com/mertcikla/codeindex/community"
)

func TestMapTopElementRepositoryMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input MapInput
		repo  string
	}{
		{"remote", MapInput{RepositoryRemoteURL: "https://github.com/acme/demo.git", RepositoryRoot: "/work/demo", RepositoryName: "demo"}, "https://github.com/acme/demo.git"},
		{"root", MapInput{RepositoryRoot: "/work/demo", RepositoryName: "demo"}, "/work/demo"},
		{"name", MapInput{RepositoryName: "demo"}, "demo"},
		{"unknown", MapInput{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.RepositoryID = "repo-1"
			tc.input.RepositoryBranch = "feature/map"
			got := mapTopElement(tc.input)
			if tc.repo == "" {
				if got.Repo != nil {
					t.Fatalf("repo = %v, want nil", got.Repo)
				}
			} else if got.Repo == nil || *got.Repo != tc.repo {
				t.Fatalf("repo = %v, want %q", got.Repo, tc.repo)
			}
			if got.RepositoryID == nil || *got.RepositoryID != "repo-1" {
				t.Fatalf("repository_id = %v, want repo-1", got.RepositoryID)
			}
			if got.Branch == nil || *got.Branch != "feature/map" {
				t.Fatalf("branch = %v, want feature/map", got.Branch)
			}
			const wantDescription = "Generated map for code grounding"
			if got.Description == nil {
				t.Fatalf("description = nil, want %q", wantDescription)
			}
			if *got.Description != wantDescription {
				t.Fatalf("description = %q, want %q", *got.Description, wantDescription)
			}
		})
	}
	if got := mapTopElement(MapInput{}); got.Branch != nil || got.RepositoryID != nil {
		t.Fatalf("missing metadata should stay nil: %+v", got)
	}
}

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
