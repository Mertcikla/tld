package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/internal/watch"
)

type openEditorRequest struct {
	Editor   string `json:"editor"`
	Repo     string `json:"repo"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
}

func registerEditorHandlers(mux *http.ServeMux, store *watch.Store) {
	mux.HandleFunc("POST /api/editor/open", func(w http.ResponseWriter, r *http.Request) {
		var req openEditorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := openInEditor(r.Context(), store, req); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
}

func openInEditor(ctx context.Context, store *watch.Store, req openEditorRequest) error {
	editor := strings.TrimSpace(strings.ToLower(req.Editor))
	if editor != "zed" && editor != "vscode" {
		return fmt.Errorf("unsupported editor %q", req.Editor)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return errors.New("file_path is required")
	}

	target, err := resolveEditorPath(ctx, store, req.Repo, req.FilePath)
	if err != nil {
		return err
	}
	if req.Line > 0 {
		target = target + ":" + strconv.Itoa(req.Line)
	}

	var cmdName string
	var args []string
	if editor == "zed" {
		cmdName = "zed"
		args = []string{target}
	} else {
		cmdName = "code"
		args = []string{"-g", target}
	}

	cmdPath, err := lookPath(cmdName)
	if err != nil {
		return fmt.Errorf("%s command not found in PATH", cmdName)
	}
	cmd := exec.Command(cmdPath, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		return cmd.Process.Release()
	}
	return nil
}

type repositoryFetcher interface {
	Repositories(ctx context.Context) ([]watch.Repository, error)
}

func resolveEditorPath(ctx context.Context, store repositoryFetcher, repoValue string, filePath string) (string, error) {
	cleanFile := strings.TrimSpace(filePath)

	repos, err := store.Repositories(ctx)
	if err != nil {
		return "", fmt.Errorf("load watched repositories: %w", err)
	}

	if filepath.IsAbs(cleanFile) {
		cleanFile = filepath.Clean(cleanFile)
		for _, repo := range repos {
			root := filepath.Clean(repo.RepoRoot)
			if cleanFile == root || strings.HasPrefix(cleanFile, root+string(filepath.Separator)) {
				return cleanFile, nil
			}
		}
		return "", errors.New("absolute file_path must reside within a watched repository")
	}

	if strings.HasPrefix(cleanFile, "~") {
		return "", errors.New("file_path must be absolute or relative to a watched repository")
	}

	relative := filepath.Clean(filepath.FromSlash(cleanFile))
	if relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return "", errors.New("file_path must stay inside the watched repository")
	}

	if len(repos) == 0 {
		return "", errors.New("no watched repositories are available to resolve this relative file path")
	}

	repo, ok := matchRepository(repos, repoValue)
	if !ok && len(repos) == 1 {
		repo = repos[0]
		ok = true
	}
	if !ok {
		return "", errors.New("could not resolve the linked repository to a local worktree")
	}

	root := filepath.Clean(repo.RepoRoot)
	target := filepath.Clean(filepath.Join(root, relative))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", errors.New("resolved file path escapes the watched repository")
	}
	return target, nil
}

func matchRepository(repos []watch.Repository, value string) (watch.Repository, bool) {
	needle := strings.TrimSpace(value)
	needleSlug := githubSlug(needle)
	for _, repo := range repos {
		candidates := []string{repo.RepoRoot}
		if repo.RemoteURL.Valid {
			candidates = append(candidates, repo.RemoteURL.String)
		}
		for _, candidate := range candidates {
			if strings.EqualFold(strings.TrimSpace(candidate), needle) {
				return repo, true
			}
			if needleSlug != "" && strings.EqualFold(githubSlug(candidate), needleSlug) {
				return repo, true
			}
		}
	}
	return watch.Repository{}, false
}

func githubSlug(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.TrimSuffix(cleaned, ".git")
	if strings.HasPrefix(cleaned, "git@github.com:") {
		return strings.ToLower(strings.TrimPrefix(cleaned, "git@github.com:"))
	}
	cleaned = strings.TrimPrefix(cleaned, "https://")
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimPrefix(cleaned, "github.com/")
	cleaned = strings.TrimPrefix(cleaned, "www.github.com/")
	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 && !strings.Contains(parts[0], ".") {
		return strings.ToLower(parts[0] + "/" + parts[1])
	}
	if len(parts) >= 3 && strings.EqualFold(parts[0], "github.com") {
		return strings.ToLower(parts[1] + "/" + parts[2])
	}
	return ""
}

func lookPath(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if runtime.GOOS == "darwin" {
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"} {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
