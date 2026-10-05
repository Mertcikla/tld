package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

func TestPullRequestNumber(t *testing.T) {
	for _, input := range []string{"7", "https://github.com/test/demo/pull/7"} {
		number, err := pullRequestNumber(input, "git@github.com:test/demo.git")
		if err != nil || number != "7" {
			t.Fatalf("%s: %q %v", input, number, err)
		}
	}
	for _, input := range []string{"--help", "0", "https://github.com/other/demo/pull/7", "https://elsewhere.test/test/demo/pull/7"} {
		if _, err := pullRequestNumber(input, "https://github.com/test/demo.git"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestRepositoryPullRequestDoesNotChangeCheckout(t *testing.T) {
	s, root, initial, repoID := prepareFixture(t)
	testGit(t, root, "remote", "add", "origin", "https://github.com/test/demo.git")
	testGit(t, root, "switch", "-c", "feature")
	testGit(t, root, "commit", "--allow-empty", "-m", "feature")
	head := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	testGit(t, root, "switch", "main")
	writeFixtureSource(t, root, "main-only.go", "package sample\nfunc MainOnly() {}\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-m", "unrelated base change")
	base := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	testGit(t, root, "switch", "feature")
	bin := t.TempDir()
	raw, err := json.Marshal(map[string]string{"title": "Feature", "url": "https://github.com/test/demo/pull/7", "baseRefOid": base, "headRefOid": head, "baseRefName": "main", "headRefName": "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\ncat <<'JSON'\n"+string(raw)+"\nJSON\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	svc := &codeIndexRepositoryService{store: s.idx}
	result, err := svc.GetPullRequest(context.Background(), connect.NewRequest(&pb.GetPullRequestRequest{RepositoryId: repoID, PullRequest: "7"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.BaseSha != initial || result.Msg.HeadSha != head {
		t.Fatalf("unexpected refs: %+v", result.Msg)
	}
	if current := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD")); current != head {
		t.Fatal("checkout changed")
	}
	if _, err := svc.GetPullRequest(context.Background(), connect.NewRequest(&pb.GetPullRequestRequest{RepositoryId: repoID, PullRequest: "https://github.com/other/demo/pull/7"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("cross-repository PR: %v", err)
	}
}

func TestRepositoryBrowserURL(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:Mertcikla/tld.git":                              "https://github.com/Mertcikla/tld",
		"ssh://git@github.com/Mertcikla/tld.git":                        "https://github.com/Mertcikla/tld",
		"https://user:secret@github.com/Mertcikla/tld.git?token=secret": "https://github.com/Mertcikla/tld",
		"/tmp/local.git":        "",
		"file:///tmp/local.git": "",
		"javascript:alert(1)":   "",
	} {
		if got := repositoryBrowserURL(remote); got != want {
			t.Errorf("%s: %q != %q", remote, got, want)
		}
	}
}

func TestRepositoryListsOpenPullRequests(t *testing.T) {
	s, root, initial, repoID := prepareFixture(t)
	testGit(t, root, "remote", "add", "origin", "git@github.com:test/demo.git")
	bin := t.TempDir()
	script := `#!/bin/sh
[ "$1" = "pr" ] && [ "$2" = "list" ] && [ "$5" = "--state" ] && [ "$6" = "open" ] || exit 1
cat <<'JSON'
[{"number":7,"title":"Feature","url":"https://github.com/test/demo/pull/7","baseRefName":"main","headRefName":"feature"}]
JSON
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	svc := &codeIndexRepositoryService{store: s.idx}
	history, err := svc.GetGitHistory(context.Background(), connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repoID}))
	if err != nil || history.Msg.RepositoryUrl != "https://github.com/test/demo" {
		t.Fatalf("repository URL: %v %v", history, err)
	}
	result, err := svc.ListPullRequests(context.Background(), connect.NewRequest(&pb.ListPullRequestsRequest{RepositoryId: repoID}))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Msg.PullRequests) != 1 || result.Msg.PullRequests[0].Number != 7 || result.Msg.PullRequests[0].HeadBranch != "feature" {
		t.Fatalf("open PRs: %+v", result.Msg)
	}
	if current := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD")); current != initial {
		t.Fatal("checkout changed")
	}
}
