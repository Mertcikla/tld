package cmdutil

import (
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRepoScopeMatchesElementByIdentity(t *testing.T) {
	scope := RepoScope{
		Root:         "/work/alpha",
		RemoteURL:    "git@github.com:owner/alpha.git",
		RepositoryID: "repo-alpha",
	}

	if !scope.MatchesElement(&workspace.Element{RepositoryID: "repo-alpha"}) {
		t.Fatal("expected explicit repository id to match")
	}
	if scope.MatchesElement(&workspace.Element{RepositoryID: "repo-beta"}) {
		t.Fatal("did not expect a different repository id to match")
	}
	if !scope.MatchesElement(&workspace.Element{Repo: "owner/alpha"}) {
		t.Fatal("expected normalized remote slug to match")
	}
	if !scope.MatchesElement(&workspace.Element{Repo: "https://github.com/Owner/Alpha"}) {
		t.Fatal("expected normalized remote URL to match")
	}
	if scope.MatchesElement(&workspace.Element{Repo: "owner/beta"}) {
		t.Fatal("did not expect a different remote to match")
	}
	if !scope.MatchesElement(&workspace.Element{FilePath: filepath.Join("/work/alpha", "src", "main.go")}) {
		t.Fatal("expected file path under root to match")
	}
}

func TestRepoScopeInactiveMatchesEverything(t *testing.T) {
	var scope RepoScope
	if !scope.MatchesElement(&workspace.Element{Name: "anything"}) {
		t.Fatal("inactive scope should match every element")
	}
}
