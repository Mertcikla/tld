package server

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

func TestRepositoryHistoryPagination(t *testing.T) {
	s, root, _, repositoryID := prepareFixture(t)
	for i := 0; i < 104; i++ {
		testGit(t, root, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", fmt.Sprintf("commit %d", i))
	}
	// Include both parents of a merge so pagination must retain topological order.
	testGit(t, root, "checkout", "-b", "feature", "HEAD~3")
	testGit(t, root, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "feature")
	testGit(t, root, "checkout", "main")
	testGit(t, root, "-c", "commit.gpgsign=false", "merge", "--no-ff", "feature", "-m", "merge")
	head := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
	ctx := context.Background()
	expected, err := readCommits(ctx, root, head, 0)
	if err != nil {
		t.Fatal(err)
	}
	svc := &repositoryService{store: s.store}
	first, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID}))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Msg.Commits) != 100 || !first.Msg.HasMore || first.Msg.NextPageToken == "" || first.Msg.Commits[0].Sha != head {
		t.Fatalf("first page: %v", first.Msg)
	}
	// A new commit must not shift the continuation and duplicate or omit a row.
	testGit(t, root, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "new head")
	second, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, PageToken: first.Msg.NextPageToken}))
	if err != nil {
		t.Fatal(err)
	}
	if second.Msg.HasMore || second.Msg.NextPageToken != "" {
		t.Fatalf("last page should not have a continuation: %v", second.Msg)
	}
	got := append(first.Msg.Commits, second.Msg.Commits...)
	if !slices.EqualFunc(got, expected, func(a, b *pb.GitCommit) bool { return a.Sha == b.Sha }) {
		t.Fatalf("paginated history differs: got %d commits, want %d", len(got), len(expected))
	}
	for _, limit := range []uint32{1, 100, 1000} {
		page, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, Limit: limit}))
		if err != nil || len(page.Msg.GetCommits()) != min(int(limit), 100) || !page.Msg.HasMore {
			t.Fatalf("limit %d: %v %v", limit, page, err)
		}
	}
	for name, request := range map[string]*pb.GetGitHistoryRequest{
		"malformed":        {RepositoryId: repositoryID, PageToken: "invalid"},
		"different branch": {RepositoryId: repositoryID, Branch: "feature", PageToken: first.Msg.NextPageToken},
		"changed query":    {RepositoryId: repositoryID, Query: "commit", PageToken: first.Msg.NextPageToken},
		"changed author":   {RepositoryId: repositoryID, Author: "Test <test@example.com>", PageToken: first.Msg.NextPageToken},
		"changed tag":      {RepositoryId: repositoryID, Tag: "v1", PageToken: first.Msg.NextPageToken},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.GetGitHistory(ctx, connect.NewRequest(request)); connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("invalid token should be rejected: %v", err)
			}
		})
	}
	t.Run("search and filters", func(t *testing.T) {
		testGit(t, root, "tag", "archived", expected[len(expected)-1].Sha)
		testGit(t, root, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "--author=Alice <alice@example.com>", "-m", "A fix", "-m", "Body-only Needle.[literal]")
		alice := strings.TrimSpace(testGit(t, root, "rev-parse", "HEAD"))
		testGit(t, root, "tag", "v-needle")
		for _, request := range []*pb.GetGitHistoryRequest{
			{Query: "BODY-ONLY needle.[literal]"},
			{Query: strings.ToUpper(alice[:12])},
			{Query: "v-needle"},
			{Author: "Alice <alice@example.com>"},
			{Tag: "v-needle"},
			{Query: "fix", Author: "Alice <alice@example.com>", Tag: "v-needle"},
		} {
			request.RepositoryId = repositoryID
			result, err := svc.GetGitHistory(ctx, connect.NewRequest(request))
			if err != nil || len(result.Msg.GetCommits()) != 1 || result.Msg.Commits[0].Sha != alice {
				t.Fatalf("search %v: %v %v", request, result, err)
			}
		}
		old, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, Query: "archived"}))
		if err != nil || len(old.Msg.GetCommits()) != 1 || old.Msg.Commits[0].Sha != expected[len(expected)-1].Sha {
			t.Fatalf("search must find commits beyond the first page: %v %v", old, err)
		}
		for _, request := range []*pb.GetGitHistoryRequest{{Query: "missing needle"}, {Branch: "feature", Author: "Alice <alice@example.com>"}, {Query: "fix", Author: "Test <test@example.com>"}} {
			request.RepositoryId = repositoryID
			result, err := svc.GetGitHistory(ctx, connect.NewRequest(request))
			if err != nil || len(result.Msg.GetCommits()) != 0 || result.Msg.HasMore {
				t.Fatalf("expected no matches: %v %v", result, err)
			}
		}
		matches, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, Query: "commit"}))
		if err != nil || len(matches.Msg.GetCommits()) != 100 || !matches.Msg.HasMore {
			t.Fatalf("paginated matches: %v %v", matches, err)
		}
		next, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, Query: "commit", PageToken: matches.Msg.NextPageToken}))
		if err != nil || len(next.Msg.GetCommits()) != 4 || next.Msg.HasMore {
			t.Fatalf("remaining matches: %v %v", next, err)
		}
		if _, err := svc.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: repositoryID, Query: "different", PageToken: matches.Msg.NextPageToken})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("cursor accepted a changed search: %v", err)
		}
		filters, err := svc.GetGitHistoryFilters(ctx, connect.NewRequest(&pb.GetGitHistoryFiltersRequest{RepositoryId: repositoryID}))
		if err != nil || !slices.Contains(filters.Msg.GetAuthors(), "Alice <alice@example.com>") || !slices.Contains(filters.Msg.GetTags(), "archived") {
			t.Fatalf("filter choices: %v %v", filters, err)
		}
	})
}
