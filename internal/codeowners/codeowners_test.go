package codeowners

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMatchesBasicAndExtendedOwners(t *testing.T) {
	matcher := Parse(`
# comment
/path/to/code @username
/frontend/* @org/web-team:random(2)
/backend/* @org/backend:least_busy(3) # Randomly select reviewers
`)

	assertTags(t, matcher.TagsForPath("path/to/code"), []string{"owner:@username"})
	assertTags(t, matcher.TagsForPath("frontend/app.ts"), []string{"owner:@org/web-team"})
	assertTags(t, matcher.TagsForPath("backend/main.go"), []string{"owner:@org/backend"})
}

func TestLastMatchWinsAndOwnersAreSortedDeduped(t *testing.T) {
	matcher := Parse(`
*.go @zeta @alpha @zeta @ignore:least_busy(1)
/cmd/* @cmd-owner
`)

	assertTags(t, matcher.TagsForPath("internal/app.go"), []string{"owner:@alpha", "owner:@ignore", "owner:@zeta"})
	assertTags(t, matcher.TagsForPath("cmd/main.go"), []string{"owner:@cmd-owner"})
}

func TestDirectoryAndFolderOwnership(t *testing.T) {
	matcher := Parse(`
/frontend/ @org/web-team
/backend/* @org/backend
`)

	assertTags(t, matcher.TagsForPath("frontend"), []string{"owner:@org/web-team"})
	assertTags(t, matcher.TagsForPath("frontend/app.ts"), []string{"owner:@org/web-team"})
	assertTags(t, matcher.TagsForPath("backend"), []string{"owner:@org/backend"})
	assertTags(t, matcher.TagsForPath("backend/service.go"), []string{"owner:@org/backend"})
}

func TestLoadFindsSupportedLocations(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "CODEOWNERS"), []byte("/src/* @owner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertTags(t, matcher.TagsForPath("src/main.go"), []string{"owner:@owner"})
}

func assertTags(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags = %#v, want %#v", got, want)
		}
	}
}
