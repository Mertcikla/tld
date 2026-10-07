package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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

func (s *repositoryService) GetGitHistory(ctx context.Context, req *connect.Request[pb.GetGitHistoryRequest]) (*connect.Response[pb.GetGitHistoryResponse], error) {
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

func (s *repositoryService) GetCommitDetails(ctx context.Context, req *connect.Request[pb.GetCommitDetailsRequest]) (*connect.Response[pb.GetCommitDetailsResponse], error) {
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

// GetRevisionRangeSummary reports the commit count and line-level diffstat
// between two commit-ish revisions using local Git only: it neither indexes nor
// builds a map, so the history footer can preview a range before comparing.
func (s *repositoryService) GetRevisionRangeSummary(ctx context.Context, req *connect.Request[pb.GetRevisionRangeSummaryRequest]) (*connect.Response[pb.RevisionRangeSummary], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	base := strings.TrimSpace(req.Msg.GetBase())
	head := strings.TrimSpace(req.Msg.GetHead())
	baseWorkingTree := req.Msg.GetBaseWorkingTree()
	headWorkingTree := req.Msg.GetHeadWorkingTree()
	if baseWorkingTree && headWorkingTree {
		return connect.NewResponse(&pb.RevisionRangeSummary{}), nil
	}
	var baseSHA, headSHA string
	if !baseWorkingTree {
		if base == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("base revision is required"))
		}
		if baseSHA, err = resolveRevision(ctx, repo.Root, base); err != nil {
			return nil, err
		}
	}
	if !headWorkingTree {
		if head == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("head revision is required"))
		}
		if headSHA, err = resolveRevision(ctx, repo.Root, head); err != nil {
			return nil, err
		}
	}
	result := &pb.RevisionRangeSummary{}
	// The diff describes BASE -> HEAD. A working-tree side compares the checkout
	// against the other revision directly; when BASE is the working tree the diff
	// is taken from HEAD's perspective and the line counts are flipped.
	reverse := false
	var diffTargets []string
	switch {
	case headWorkingTree:
		diffTargets = []string{baseSHA}
		if raw, countErr := repositoryGit(ctx, repo.Root, "rev-list", "--count", baseSHA+"..HEAD"); countErr == nil {
			if count, parseErr := strconv.ParseUint(strings.TrimSpace(raw), 10, 32); parseErr == nil {
				result.Commits = uint32(count)
			}
		}
	case baseWorkingTree:
		diffTargets = []string{headSHA}
		reverse = true
	default:
		diffTargets = []string{baseSHA, headSHA}
		if raw, countErr := repositoryGit(ctx, repo.Root, "rev-list", "--count", baseSHA+".."+headSHA); countErr == nil {
			if count, parseErr := strconv.ParseUint(strings.TrimSpace(raw), 10, 32); parseErr == nil {
				result.Commits = uint32(count)
			}
		}
	}
	numstat, err := repositoryGit(ctx, repo.Root, append(append([]string{"diff", "--numstat", "-z", "--no-renames"}, diffTargets...), "--")...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	status, err := repositoryGit(ctx, repo.Root, append(append([]string{"diff", "--name-status", "-z", "--no-renames"}, diffTargets...), "--")...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	lines := parseNumstat(numstat)
	for _, count := range lines {
		result.Additions = addLineCount(result.Additions, count.added)
		result.Deletions = addLineCount(result.Deletions, count.removed)
	}
	if reverse {
		result.Additions, result.Deletions = result.Deletions, result.Additions
	}
	for _, file := range parseNameStatus(status) {
		result.Files++
		// "changed" is the churn in files that already existed on both sides.
		if file.status == 'M' {
			if count, ok := lines[file.path]; ok {
				result.Changed = addLineCount(result.Changed, count.added)
				result.Changed = addLineCount(result.Changed, count.removed)
			}
		}
	}
	return connect.NewResponse(result), nil
}

type rangeLineCount struct {
	added, removed uint64
}

// addLineCount saturates summary totals at the protobuf field's upper bound.
func addLineCount(total uint32, count uint64) uint32 {
	if count > math.MaxUint32 {
		return math.MaxUint32
	}
	increment := uint32(count)
	if increment > math.MaxUint32-total {
		return math.MaxUint32
	}
	return total + increment
}

// parseNumstat reads `git diff --numstat -z` records (added\tremoved\tpath\0).
// Binary files report "-" counts and are skipped.
func parseNumstat(raw string) map[string]rangeLineCount {
	counts := make(map[string]rangeLineCount)
	for _, record := range strings.Split(raw, "\x00") {
		record = strings.TrimPrefix(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, addedOK := parseLineCount(fields[0])
		removed, removedOK := parseLineCount(fields[1])
		if !addedOK || !removedOK {
			continue
		}
		counts[fields[2]] = rangeLineCount{added: added, removed: removed}
	}
	return counts
}

func parseLineCount(field string) (uint64, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(field), 10, 64)
	return value, err == nil
}

type rangeFileStatus struct {
	status byte
	path   string
}

// parseNameStatus reads `git diff --name-status -z` records (status\0path\0).
func parseNameStatus(raw string) []rangeFileStatus {
	fields := strings.Split(raw, "\x00")
	out := make([]rangeFileStatus, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		status := strings.TrimSpace(fields[i])
		path := fields[i+1]
		if status == "" || path == "" {
			continue
		}
		out = append(out, rangeFileStatus{status: status[0], path: path})
	}
	return out
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

func (s *repositoryService) GetPullRequest(ctx context.Context, req *connect.Request[pb.GetPullRequestRequest]) (*connect.Response[pb.GetPullRequestResponse], error) {
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
	mergeBase, err := repositoryGit(ctx, repo.Root, "merge-base", pr.BaseRefOid, pr.HeadRefOid)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("PR merge base is unavailable: %w", err))
	}
	return connect.NewResponse(&pb.GetPullRequestResponse{Title: pr.Title, Url: pr.URL, BaseSha: strings.TrimSpace(mergeBase), HeadSha: pr.HeadRefOid, BaseBranch: pr.BaseRefName, HeadBranch: pr.HeadRefName}), nil
}

func (s *repositoryService) ListPullRequests(ctx context.Context, req *connect.Request[pb.ID]) (*connect.Response[pb.ListPullRequestsResponse], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetId())
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
