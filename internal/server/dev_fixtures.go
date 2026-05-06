package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type devFixtureManifest struct {
	SchemaVersion  int        `json:"schema_version"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Language       string     `json:"language,omitempty"`
	Domain         string     `json:"domain,omitempty"`
	Framework      string     `json:"framework,omitempty"`
	Type           string     `json:"type,omitempty"`
	Notes          []string   `json:"notes,omitempty"`
	ReviewStatus   string     `json:"review_status,omitempty"`
	Accuracy       string     `json:"accuracy,omitempty"`
	ReviewComments []string   `json:"review_comments,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	RepoPath       string     `json:"repo_path,omitempty"`
	SnapshotPath   string     `json:"snapshot_path"`
}

func registerDevFixtureHandlers(mux *http.ServeMux, root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	mux.HandleFunc("GET /api/dev/fixtures/snapshot", func(w http.ResponseWriter, r *http.Request) {
		fixtureDir, manifest, ok := resolveDevFixture(w, r, rootAbs)
		if !ok {
			return
		}
		snapshotPath := manifest.SnapshotPath
		if snapshotPath == "" {
			snapshotPath = filepath.ToSlash(filepath.Join("golden", "snapshot.json"))
		}
		snapshotFile, err := safeJoin(fixtureDir, snapshotPath)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid snapshot path")
			return
		}
		var snapshot any
		if err := readJSON(snapshotFile, &snapshot); err != nil {
			writeJSONError(w, http.StatusNotFound, "snapshot not found")
			return
		}
		writeJSON(w, map[string]any{"manifest": manifest, "snapshot": snapshot})
	})
	mux.HandleFunc("GET /api/dev/fixtures/source", func(w http.ResponseWriter, r *http.Request) {
		fixtureDir, manifest, ok := resolveDevFixture(w, r, rootAbs)
		if !ok {
			return
		}
		repoPath := manifest.RepoPath
		if repoPath == "" {
			repoPath = "repo"
		}
		repoDir, err := safeJoin(fixtureDir, repoPath)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid repo path")
			return
		}
		sourcePath := strings.TrimSpace(r.URL.Query().Get("path"))
		if sourcePath == "" {
			sourcePath = firstSourceFile(repoDir)
		}
		sourceFile, err := safeJoin(repoDir, sourcePath)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid source path")
			return
		}
		data, err := os.ReadFile(sourceFile)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "source not found")
			return
		}
		writeJSON(w, map[string]any{"path": filepath.ToSlash(sourcePath), "content": string(data)})
	})
}

func resolveDevFixture(w http.ResponseWriter, r *http.Request, root string) (string, devFixtureManifest, bool) {
	rel := strings.TrimSpace(r.URL.Query().Get("fixture"))
	if rel == "" {
		writeJSONError(w, http.StatusBadRequest, "fixture is required")
		return "", devFixtureManifest{}, false
	}
	fixtureDir, err := safeJoin(root, rel)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid fixture path")
		return "", devFixtureManifest{}, false
	}
	var manifest devFixtureManifest
	if err := readJSON(filepath.Join(fixtureDir, "fixture.json"), &manifest); err != nil {
		writeJSONError(w, http.StatusNotFound, "fixture not found")
		return "", devFixtureManifest{}, false
	}
	return fixtureDir, manifest, true
}

func safeJoin(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("path escapes root")
	}
	path := filepath.Join(rootAbs, rel)
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator)) {
		return "", errors.New("path escapes root")
	}
	return pathAbs, nil
}

func firstSourceFile(root string) string {
	allowed := map[string]bool{".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".json": true, ".py": true, ".java": true, ".rs": true}
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if allowed[strings.ToLower(filepath.Ext(path))] {
			if rel, err := filepath.Rel(root, path); err == nil {
				found = rel
			}
		}
		return nil
	})
	return found
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
