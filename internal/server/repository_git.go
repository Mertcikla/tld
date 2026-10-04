package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

func repositoryGit(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	raw, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git: %s: %w", strings.TrimSpace(string(exit.Stderr)), err)
		}
		return "", err
	}
	return string(raw), nil
}

func resolveRevision(ctx context.Context, root, revision string) (string, error) {
	raw, err := repositoryGit(ctx, root, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("commit %q is unavailable locally: %w", revision, err))
	}
	return strings.TrimSpace(raw), nil
}

const commitFormat = "--format=%H%x00%s%x00%an%x00%ae%x00%at%x00%P%x00%D%x00%B%x00"

func readCommits(ctx context.Context, root, sha string, limit int) ([]*pb.GitCommit, error) {
	args := []string{"log", "--topo-order"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, commitFormat, sha, "--")
	raw, err := repositoryGit(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	commits := []*pb.GitCommit{}
	for _, record := range strings.Split(raw, "\x00\n") {
		fields := strings.Split(strings.TrimPrefix(record, "\n"), "\x00")
		if len(fields) < 8 {
			continue
		}
		stamp, _ := strconv.ParseInt(fields[4], 10, 64)
		refs := []string{}
		for _, ref := range strings.Split(fields[6], ", ") {
			if ref != "" {
				refs = append(refs, ref)
			}
		}
		commits = append(commits, &pb.GitCommit{Sha: fields[0], Subject: fields[1], Author: fields[2], AuthorEmail: fields[3], CreatedUnix: stamp, Parents: strings.Fields(fields[5]), Refs: refs, Body: strings.TrimSpace(fields[7])})
	}
	return commits, nil
}

func (s *codeIndexRepositoryService) GetGitHistory(ctx context.Context, req *connect.Request[pb.GetGitHistoryRequest]) (*connect.Response[pb.GetGitHistoryResponse], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if info, e := os.Stat(repo.Root); e != nil || !info.IsDir() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository checkout is unavailable: %s", repo.Root))
	}
	result := &pb.GetGitHistoryResponse{}
	if _, err := repositoryGit(ctx, repo.Root, "rev-parse", "--git-dir"); err != nil {
		return connect.NewResponse(result), nil
	}
	result.IsGit = true
	remote, _ := repositoryGit(ctx, repo.Root, "remote", "get-url", "origin")
	result.RepositoryUrl = repositoryBrowserURL(remote)
	branch, _ := repositoryGit(ctx, repo.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	result.CurrentBranch = strings.TrimSpace(branch)
	head, _ := repositoryGit(ctx, repo.Root, "rev-parse", "--verify", "HEAD")
	result.HeadSha = strings.TrimSpace(head)
	refs, err := repositoryGit(ctx, repo.Root, "for-each-ref", "--format=%(refname:short)%00%(objectname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(refs), "\n") {
		fields := strings.SplitN(line, "\x00", 2)
		if len(fields) == 2 {
			result.Branches = append(result.Branches, &pb.GitBranch{Name: fields[0], Sha: fields[1]})
		}
	}
	if result.HeadSha == "" && req.Msg.GetBranch() == "" {
		return connect.NewResponse(result), nil
	}
	target := result.HeadSha
	if req.Msg.GetBranch() != "" {
		target, err = resolveRevision(ctx, repo.Root, req.Msg.GetBranch())
		if err != nil {
			return nil, err
		}
	}
	limit := int(req.Msg.GetLimit())
	if limit > 1000 {
		limit = 1000
	}
	readLimit := 0
	if limit > 0 {
		readLimit = limit + 1
	}
	result.Commits, err = readCommits(ctx, repo.Root, target, readLimit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	result.HasMore = limit > 0 && len(result.Commits) > limit
	if result.HasMore {
		result.Commits = result.Commits[:limit]
	}
	return connect.NewResponse(result), nil
}

func (s *codeIndexRepositoryService) GetCommitDetails(ctx context.Context, req *connect.Request[pb.GetCommitDetailsRequest]) (*connect.Response[pb.GetCommitDetailsResponse], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if strings.TrimSpace(req.Msg.GetRevision()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("revision is required"))
	}
	sha, err := resolveRevision(ctx, repo.Root, req.Msg.GetRevision())
	if err != nil {
		return nil, err
	}
	commits, err := readCommits(ctx, repo.Root, sha, 1)
	if err != nil || len(commits) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("commit not found"))
	}
	result := &pb.GetCommitDetailsResponse{Commit: commits[0]}
	raw, err := repositoryGit(ctx, repo.Root, "show", "--numstat", "-z", "--format=", "--no-renames", sha, "--")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, line := range strings.Split(raw, "\x00") {
		fields := strings.SplitN(strings.TrimPrefix(line, "\n"), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, _ := strconv.ParseUint(fields[0], 10, 32)
		removed, _ := strconv.ParseUint(fields[1], 10, 32)
		result.Files = append(result.Files, &pb.CommitFileChange{Path: fields[2], Added: uint32(added), Removed: uint32(removed), Binary: fields[0] == "-"})
	}
	return connect.NewResponse(result), nil
}

// repositoryBrowserURL converts Git origins to safe browser links without credentials.
func repositoryBrowserURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.HasPrefix(remote, "git@") {
		remote = "https://" + strings.Replace(strings.TrimPrefix(remote, "git@"), ":", "/", 1)
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.Host == "" {
		return ""
	}
	switch parsed.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return ""
	}
	path := strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/"), ".git")
	if len(strings.Split(strings.Trim(path, "/"), "/")) != 2 {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: parsed.Host, Path: path}).String()
}

func repositoryGitHub(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		message := "GitHub CLI (gh) is required and must be signed in to load PRs"
		if exit, ok := err.(*exec.ExitError); ok {
			message = strings.TrimSpace(string(exit.Stderr))
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s", message))
	}
	return raw, nil
}

// pullRequestNumber scopes URLs to the selected checkout's origin before invoking gh.
func pullRequestNumber(input, remote string) (string, error) {
	input = strings.TrimSpace(input)
	if regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(input) {
		return input, nil
	}
	origin, err := url.Parse(repositoryBrowserURL(remote))
	if err != nil || origin.Host == "" {
		return "", fmt.Errorf("invalid repository origin")
	}
	pr, err := url.Parse(input)
	if err != nil || (pr.Scheme != "https" && pr.Scheme != "http") || !strings.EqualFold(pr.Host, origin.Host) {
		return "", fmt.Errorf("enter a PR number or a URL from this repository")
	}
	prefix := strings.TrimSuffix(strings.TrimSuffix(origin.Path, "/"), ".git") + "/pull/"
	if !strings.HasPrefix(pr.Path, prefix) {
		return "", fmt.Errorf("PR URL belongs to another repository")
	}
	number := strings.TrimPrefix(pr.Path, prefix)
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(number) {
		return "", fmt.Errorf("invalid PR number")
	}
	return number, nil
}

func (s *codeIndexRepositoryService) GetPullRequest(ctx context.Context, req *connect.Request[pb.GetPullRequestRequest]) (*connect.Response[pb.GetPullRequestResponse], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	remote, err := repositoryGit(ctx, repo.Root, "remote", "get-url", "origin")
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	remote = strings.TrimSpace(remote)
	number, err := pullRequestNumber(req.Msg.GetPullRequest(), remote)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	raw, err := repositoryGitHub(ctx, repo.Root, "pr", "view", number, "--repo", remote, "--json", "title,url,baseRefOid,headRefOid,baseRefName,headRefName")
	if err != nil {
		return nil, err
	}
	var pr struct{ Title, URL, BaseRefOid, HeadRefOid, BaseRefName, HeadRefName string }
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	validSHA := regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	if !validSHA.MatchString(pr.BaseRefOid) || !validSHA.MatchString(pr.HeadRefOid) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("PR has no available base and head commits"))
	}
	// Fetch objects without changing the checkout or local branches.
	if _, err := resolveRevision(ctx, repo.Root, pr.HeadRefOid); err != nil {
		if _, err = repositoryGit(ctx, repo.Root, "fetch", "--no-tags", "origin", "refs/pull/"+number+"/head"); err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
	}
	if _, err := resolveRevision(ctx, repo.Root, pr.BaseRefOid); err != nil {
		if _, err = repositoryGit(ctx, repo.Root, "fetch", "--no-tags", "origin", pr.BaseRefOid); err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
	}
	if _, err := resolveRevision(ctx, repo.Root, pr.HeadRefOid); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.GetPullRequestResponse{Title: pr.Title, Url: pr.URL, BaseSha: pr.BaseRefOid, HeadSha: pr.HeadRefOid, BaseBranch: pr.BaseRefName, HeadBranch: pr.HeadRefName}), nil
}

func (s *codeIndexRepositoryService) ListPullRequests(ctx context.Context, req *connect.Request[pb.ListPullRequestsRequest]) (*connect.Response[pb.ListPullRequestsResponse], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	remote, err := repositoryGit(ctx, repo.Root, "remote", "get-url", "origin")
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	raw, err := repositoryGitHub(ctx, repo.Root, "pr", "list", "--repo", strings.TrimSpace(remote), "--state", "open", "--limit", "1000", "--json", "number,title,url,baseRefName,headRefName")
	if err != nil {
		return nil, err
	}
	var prs []struct {
		Number                               uint32
		Title, URL, BaseRefName, HeadRefName string
	}
	if err := json.Unmarshal(raw, &prs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	result := &pb.ListPullRequestsResponse{}
	for _, pr := range prs {
		result.PullRequests = append(result.PullRequests, &pb.OpenPullRequest{Number: pr.Number, Title: pr.Title, Url: pr.URL, BaseBranch: pr.BaseRefName, HeadBranch: pr.HeadRefName})
	}
	return connect.NewResponse(result), nil
}
