package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/store"
)

const maxSourcePreviewBytes = 1 << 20

type openEditorRequest struct {
	Editor       string `json:"editor"`
	RepositoryID string `json:"repository_id"`
	Repo         string `json:"repo"`
	FilePath     string `json:"file_path"`
	Line         int    `json:"line"`
}

func registerEditorHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore) {
	fetcher := dbRepositoryFetcher{db: sqliteStore.DB()}
	mux.HandleFunc("POST /api/editor/open", func(w http.ResponseWriter, r *http.Request) {
		var req openEditorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := openInEditor(r.Context(), fetcher, req); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /api/editor/source", func(w http.ResponseWriter, r *http.Request) {
		var req openEditorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(req.FilePath) == "" {
			writeJSONError(w, http.StatusBadRequest, "file_path is required")
			return
		}
		target, err := resolveEditorPath(r.Context(), fetcher, req.RepositoryID, req.Repo, req.FilePath)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		content, err := readSourceFile(target, maxSourcePreviewBytes)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]string{"content": content, "path": target})
	})
}

// readSourceFile returns a text file's contents for in-app preview, rejecting
// binaries and files larger than maxBytes.
func readSourceFile(path string, maxBytes int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("file is too large to preview")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("binary file cannot be previewed")
	}
	return string(data), nil
}

func openInEditor(ctx context.Context, store repositoryFetcher, req openEditorRequest) error {
	editor := strings.TrimSpace(strings.ToLower(req.Editor))
	if editor != "zed" && editor != "vscode" {
		return fmt.Errorf("unsupported editor %q", req.Editor)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return errors.New("file_path is required")
	}

	target, err := resolveEditorPath(ctx, store, req.RepositoryID, req.Repo, req.FilePath)
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
		return fmt.Errorf("%s command not found; install the editor command-line tool or add it to PATH", cmdName)
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
	Repositories(ctx context.Context) ([]repolink.Repository, error)
}

// dbRepositoryFetcher lists indexed repository roots from the codeindex store.
type dbRepositoryFetcher struct{ db *sql.DB }

func (f dbRepositoryFetcher) Repositories(ctx context.Context) ([]repolink.Repository, error) {
	rows, err := f.db.QueryContext(ctx, `SELECT id, root, remote_url FROM codeindex_repositories WHERE root <> '' ORDER BY root`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []repolink.Repository{}
	for rows.Next() {
		var repo repolink.Repository
		if err := rows.Scan(&repo.ID, &repo.Root, &repo.RemoteURL); err != nil {
			return nil, err
		}
		if repo.RemoteURL == "" {
			repo.RemoteURL = repolink.GitRemoteURL(ctx, repo.Root)
		}
		repo.Name = filepath.Base(repo.Root)
		out = append(out, repo)
	}
	return out, rows.Err()
}

func resolveEditorPath(ctx context.Context, store repositoryFetcher, repositoryID, repoValue, filePath string) (string, error) {
	cleanFile := strings.TrimSpace(filePath)
	if before, _, ok := strings.Cut(cleanFile, "#"); ok {
		cleanFile = before
	}

	repos, err := store.Repositories(ctx)
	if err != nil {
		return "", fmt.Errorf("load watched repositories: %w", err)
	}

	if filepath.IsAbs(cleanFile) {
		cleanFile = filepath.Clean(cleanFile)
		for _, repo := range repos {
			root := filepath.Clean(repo.Root)
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
		return "", errors.New("no watched repositories are configured; add a repository in the Workspace panel before opening source files")
	}

	repo, ok := repolink.Resolve(repositoryID, repoValue, relative, repos)
	if !ok && len(repos) == 1 {
		repo = repos[0]
		ok = true
	}
	if !ok {
		return "", errors.New("could not resolve the linked repository to a local worktree")
	}

	root := filepath.Clean(repo.Root)
	target := filepath.Clean(filepath.Join(root, relative))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", errors.New("resolved file path escapes the watched repository")
	}
	return target, nil
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
		for _, candidate := range darwinAppCommandCandidates(name) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func darwinAppCommandCandidates(name string) []string {
	switch name {
	case "code":
		return []string{
			"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
			filepath.Join(os.Getenv("HOME"), "Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"),
		}
	case "zed":
		return []string{
			"/Applications/Zed.app/Contents/MacOS/cli",
			filepath.Join(os.Getenv("HOME"), "Applications/Zed.app/Contents/MacOS/cli"),
		}
	default:
		return nil
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
