package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

func parseGitCommit(record string) *pb.GitCommit {
	fields := strings.Split(strings.TrimPrefix(record, "\n"), "\x00")
	if len(fields) < 8 {
		return nil
	}
	stamp, _ := strconv.ParseInt(fields[4], 10, 64)
	var refs []string
	for _, ref := range strings.Split(fields[6], ", ") {
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return &pb.GitCommit{Sha: fields[0], Subject: fields[1], Author: fields[2], AuthorEmail: fields[3], CreatedUnix: stamp, Parents: strings.Fields(fields[5]), Refs: refs, Body: strings.TrimSpace(fields[7])}
}

// Search streams Git's output and stops after one page of matches. Older matches
// can be found without sending or retaining the entire history in the UI/server.
func searchCommitPage(ctx context.Context, root, revision, query, author, tag string, limit, offset int) ([]*pb.GitCommit, error) {
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "git", "-C", root, "log", "--topo-order", commitFormat, revision, "--")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.Index(data, []byte("\x00\n")); i >= 0 {
			return i + 2, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	commits := []*pb.GitCommit{}
	complete := false
	for scanner.Scan() {
		commit := parseGitCommit(scanner.Text())
		if commit == nil || !strings.Contains(strings.ToLower(strings.Join([]string{commit.Sha, commit.Subject, commit.Body, commit.Author, commit.AuthorEmail, strings.Join(commit.Refs, " ")}, " ")), query) {
			continue
		}
		if author != "" && commit.Author+" <"+commit.AuthorEmail+">" != author {
			continue
		}
		if tag != "" && !slices.Contains(commit.Refs, "tag: "+tag) {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		commits = append(commits, commit)
		if len(commits) >= limit {
			complete = true
			break
		}
	}
	scanErr := scanner.Err()
	// Cancel before waiting when stopping early, including scanner errors, so a
	// child blocked writing the remaining history cannot hang this request.
	if complete || scanErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if !complete && waitErr != nil {
		return nil, fmt.Errorf("git history search: %s: %w", strings.TrimSpace(stderr.String()), waitErr)
	}
	return commits, nil
}

func (s *repositoryService) GetGitHistoryFilters(ctx context.Context, req *connect.Request[pb.GetGitHistoryFiltersRequest]) (*connect.Response[pb.GetGitHistoryFiltersResponse], error) {
	history, err := s.GetGitHistory(ctx, connect.NewRequest(&pb.GetGitHistoryRequest{RepositoryId: req.Msg.GetRepositoryId(), Branch: req.Msg.GetBranch(), Limit: 1}))
	if err != nil {
		return nil, err
	}
	result := &pb.GetGitHistoryFiltersResponse{}
	if !history.Msg.IsGit || len(history.Msg.Commits) == 0 {
		return connect.NewResponse(result), nil
	}
	repo, err := s.store.Repository(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "git", "-C", repo.Root, "log", "--format=%an <%ae>", history.Msg.Commits[0].Sha, "--")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	authors := map[string]bool{}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		authors[scanner.Text()] = true
	}
	if scanner.Err() != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if scanner.Err() != nil {
		return nil, connect.NewError(connect.CodeInternal, scanner.Err())
	}
	if waitErr != nil {
		return nil, connect.NewError(connect.CodeInternal, waitErr)
	}
	for author := range authors {
		result.Authors = append(result.Authors, author)
	}
	sort.Strings(result.Authors)
	tags, err := repositoryGit(ctx, repo.Root, "tag", "--list")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, tag := range strings.Split(strings.TrimSpace(tags), "\n") {
		if tag != "" {
			result.Tags = append(result.Tags, tag)
		}
	}
	sort.Strings(result.Tags)
	return connect.NewResponse(result), nil
}
