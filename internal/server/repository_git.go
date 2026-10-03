package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
			return "", fmt.Errorf("git: %s", strings.TrimSpace(string(exit.Stderr)))
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
	raw, err := repositoryGit(ctx, root, "log", "--topo-order", "-n", strconv.Itoa(limit), commitFormat, sha, "--")
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
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	result.Commits, err = readCommits(ctx, repo.Root, target, limit+1)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	result.HasMore = len(result.Commits) > limit
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
