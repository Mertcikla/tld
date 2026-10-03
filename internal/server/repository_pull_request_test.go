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
	testGit(t, root, "commit", "--allow-empty", "-m", "feature")
	head := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	bin := t.TempDir()
	raw, err := json.Marshal(map[string]string{"title": "Feature", "url": "https://github.com/test/demo/pull/7", "baseRefOid": initial, "headRefOid": head, "baseRefName": "main", "headRefName": "feature"})
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
