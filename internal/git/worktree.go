package git

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type TrackedFilesResult struct {
	Files  []string
	Total  int
	Capped bool
}

func ListTrackedFiles(dir string, limit int) (TrackedFilesResult, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TrackedFilesResult{}, fmt.Errorf("git ls-files: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return TrackedFilesResult{}, fmt.Errorf("git ls-files: %w", err)
	}
	result := TrackedFilesResult{}
	reader := bufio.NewReader(stdout)
	for {
		raw, readErr := reader.ReadString(0)
		entry := strings.TrimSpace(strings.TrimSuffix(raw, "\x00"))
		if entry == "" {
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				_ = cmd.Wait()
				return TrackedFilesResult{}, fmt.Errorf("git ls-files: %w", readErr)
			}
			continue
		}
		result.Total++
		if limit <= 0 || len(result.Files) < limit {
			result.Files = append(result.Files, filepath.ToSlash(entry))
		} else {
			result.Capped = true
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return result, nil
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = cmd.Wait()
			return TrackedFilesResult{}, fmt.Errorf("git ls-files: %w", readErr)
		}
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return TrackedFilesResult{}, fmt.Errorf("git ls-files: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return TrackedFilesResult{}, fmt.Errorf("git ls-files: %w", err)
	}
	return result, nil
}

func EnsureDetachedWorktree(dir, commit, target string) error {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return fmt.Errorf("worktree commit is empty")
	}
	target = filepath.Clean(target)
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		got, err := DetectHeadCommit(target)
		if err == nil && got == commit {
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove stale worktree %q: %w", target, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create worktree parent: %w", err)
	}
	if _, err := run(dir, "worktree", "add", "--detach", target, commit); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	got, err := DetectHeadCommit(target)
	if err != nil {
		return err
	}
	if got != commit {
		return fmt.Errorf("worktree %q checked out %s, want %s", target, got, commit)
	}
	return nil
}
