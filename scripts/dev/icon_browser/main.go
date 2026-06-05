package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultAddr        = "127.0.0.1:8787"
	defaultArchivePath = "build-assets/icons.tar.gz"
	maxUploadBytes     = 1 << 20
	maxRequestBytes    = maxUploadBytes + 64*1024
)

//go:embed static/index.html
var staticFiles embed.FS

type catalogItem struct {
	IconURL     string   `json:"iconUrl"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider,omitempty"`
	DocsURL     string   `json:"docsUrl,omitempty"`
	Description string   `json:"description,omitempty"`
	WebsiteURL  string   `json:"websiteUrl,omitempty"`
	NameShort   string   `json:"nameShort"`
	DefaultSlug string   `json:"defaultSlug"`
	Aliases     []string `json:"aliases,omitempty"`
}

type iconFile struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type stateResponse struct {
	ArchivePath  string        `json:"archivePath"`
	WorkDir      string        `json:"workDir"`
	Dirty        bool          `json:"dirty"`
	Catalog      []catalogItem `json:"catalog"`
	IconFiles    []iconFile    `json:"iconFiles"`
	MissingIcons []string      `json:"missingIcons"`
	OrphanIcons  []string      `json:"orphanIcons"`
}

type repackResponse struct {
	Message    string `json:"message"`
	BackupPath string `json:"backupPath"`
	Archive    string `json:"archive"`
}

type apiError struct {
	Error string `json:"error"`
}

type iconBrowserServer struct {
	archivePath string
	workDir     string
	mu          sync.Mutex
	dirty       bool
}

var slugPartRE = regexp.MustCompile(`[^a-z0-9]+`)
var validSlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func main() {
	var (
		addr        = flag.String("addr", defaultAddr, "HTTP listen address")
		archivePath = flag.String("archive", defaultArchivePath, "path to icons tar.gz")
	)
	flag.Parse()

	absArchive, err := filepath.Abs(*archivePath)
	if err != nil {
		log.Fatalf("resolve archive path: %v", err)
	}
	if _, err := os.Stat(absArchive); err != nil {
		log.Fatalf("read archive: %v", err)
	}

	workDir, err := os.MkdirTemp("", "tld-icon-browser-*")
	if err != nil {
		log.Fatalf("create workdir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			log.Printf("remove workdir: %v", err)
		}
	}()

	server := &iconBrowserServer{archivePath: absArchive, workDir: workDir}
	if err := server.unpackLocked(); err != nil {
		log.Fatalf("unpack archive: %v", err)
	}

	url := "http://" + *addr
	log.Printf("Icon browser ready: %s", url)
	log.Printf("Archive: %s", absArchive)
	log.Fatal(http.ListenAndServe(*addr, server.routes()))
}

func (s *iconBrowserServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/unpack", s.handleUnpack)
	mux.HandleFunc("/api/repack", s.handleRepack)
	mux.HandleFunc("/api/icons", s.handleCreateIcon)
	mux.HandleFunc("/api/icons/", s.handleIconMetadata)
	mux.HandleFunc("/icons/", s.handleIconFile)
	return mux
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, _ = w.Write(body)
}

func (s *iconBrowserServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.stateLocked()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *iconBrowserServer) handleUnpack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.unpackLocked(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := s.stateLocked()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *iconBrowserServer) handleRepack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	backupPath, err := repackWorkspace(s.archivePath, s.workDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.dirty = false
	writeJSON(w, http.StatusOK, repackResponse{
		Message:    "Archive repacked successfully.",
		BackupPath: backupPath,
		Archive:    s.archivePath,
	})
}

func (s *iconBrowserServer) handleCreateIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(maxRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read upload: %w", err))
		return
	}

	file, header, err := r.FormFile("icon")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("icon SVG file is required"))
		return
	}
	defer func() { _ = file.Close() }()

	body, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(body) > maxUploadBytes {
		writeError(w, http.StatusBadRequest, fmt.Errorf("icon upload exceeds %d bytes", maxUploadBytes))
		return
	}
	if !isSVG(header.Filename, body) {
		writeError(w, http.StatusBadRequest, errors.New("icon upload must be an SVG file"))
		return
	}

	item := catalogItem{
		Name:        r.FormValue("name"),
		NameShort:   r.FormValue("nameShort"),
		DefaultSlug: r.FormValue("defaultSlug"),
		Provider:    r.FormValue("provider"),
		DocsURL:     r.FormValue("docsUrl"),
		WebsiteURL:  r.FormValue("websiteUrl"),
		Description: r.FormValue("description"),
		Aliases:     splitAliases(r.FormValue("aliases")),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	catalog, err := readCatalogFile(s.workDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	normalized, err := normalizeNewItem(item)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if findCatalogIndex(catalog, normalized.DefaultSlug) >= 0 {
		writeError(w, http.StatusConflict, fmt.Errorf("catalog entry %q already exists", normalized.DefaultSlug))
		return
	}
	iconName := normalized.DefaultSlug + ".svg"
	iconPath := filepath.Join(s.workDir, "icons", iconName)
	if _, err := os.Stat(iconPath); err == nil {
		writeError(w, http.StatusConflict, fmt.Errorf("icon file %q already exists", iconName))
		return
	} else if !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(iconPath, body, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	catalog = append(catalog, normalized)
	if err := writeCatalogFile(s.workDir, catalog); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dirty = true
	writeJSON(w, http.StatusCreated, normalized)
}

func (s *iconBrowserServer) handleIconMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	oldSlug := strings.TrimPrefix(r.URL.Path, "/api/icons/")
	if oldSlug == "" || strings.Contains(oldSlug, "/") {
		writeError(w, http.StatusBadRequest, errors.New("icon slug is required"))
		return
	}

	var item catalogItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	updated, err := s.patchIconLocked(oldSlug, item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, fs.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *iconBrowserServer) handleIconFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/icons/")
	cleanName, ok := cleanIconFilename(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	target := filepath.Join(s.workDir, "icons", filepath.FromSlash(cleanName))
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, target)
}

func (s *iconBrowserServer) patchIconLocked(oldSlug string, item catalogItem) (catalogItem, error) {
	oldSlug = strings.TrimSpace(strings.ToLower(oldSlug))
	catalog, err := readCatalogFile(s.workDir)
	if err != nil {
		return catalogItem{}, err
	}
	idx := findCatalogIndex(catalog, oldSlug)
	if idx < 0 {
		return catalogItem{}, fmt.Errorf("catalog entry %q: %w", oldSlug, fs.ErrNotExist)
	}

	normalized, err := normalizeExistingItem(item)
	if err != nil {
		return catalogItem{}, err
	}
	for i, existing := range catalog {
		if i != idx && existing.DefaultSlug == normalized.DefaultSlug {
			return catalogItem{}, fmt.Errorf("catalog entry %q already exists", normalized.DefaultSlug)
		}
	}

	if oldSlug != normalized.DefaultSlug {
		if err := renameIconFile(s.workDir, catalog[idx], normalized); err != nil {
			return catalogItem{}, err
		}
	}
	catalog[idx] = normalized
	if err := writeCatalogFile(s.workDir, catalog); err != nil {
		return catalogItem{}, err
	}
	s.dirty = true
	return normalized, nil
}

func (s *iconBrowserServer) unpackLocked() error {
	if err := os.RemoveAll(s.workDir); err != nil {
		return err
	}
	if err := os.MkdirAll(s.workDir, 0o755); err != nil {
		return err
	}
	if err := unpackArchive(s.archivePath, s.workDir); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

func (s *iconBrowserServer) stateLocked() (stateResponse, error) {
	catalog, err := readCatalogFile(s.workDir)
	if err != nil {
		return stateResponse{}, err
	}
	files, err := listIconFiles(s.workDir)
	if err != nil {
		return stateResponse{}, err
	}
	fileSet := make(map[string]bool, len(files))
	for _, file := range files {
		fileSet[file.URL] = true
	}
	seenCatalogFiles := make(map[string]bool, len(catalog))
	var missing []string
	for _, item := range catalog {
		iconURL := item.IconURL
		if iconURL == "" {
			iconURL = "/icons/" + item.DefaultSlug + ".svg"
		}
		seenCatalogFiles[iconURL] = true
		if !fileSet[iconURL] {
			missing = append(missing, item.DefaultSlug)
		}
	}
	var orphans []string
	for _, file := range files {
		if !seenCatalogFiles[file.URL] {
			orphans = append(orphans, file.Path)
		}
	}
	sort.Strings(missing)
	sort.Strings(orphans)
	return stateResponse{
		ArchivePath:  s.archivePath,
		WorkDir:      s.workDir,
		Dirty:        s.dirty,
		Catalog:      catalog,
		IconFiles:    files,
		MissingIcons: missing,
		OrphanIcons:  orphans,
	}, nil
}

func unpackArchive(archivePath string, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		cleanName, ok := cleanArchiveName(hdr.Name)
		if !ok {
			continue
		}
		target, ok := safeJoin(dst, cleanName)
		if !ok {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, cleanMode(hdr.Mode, 0o755)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, cleanMode(hdr.Mode, 0o644)); err != nil {
				return err
			}
		default:
			continue
		}
	}
}

func repackWorkspace(archivePath string, workDir string) (string, error) {
	catalog, err := readCatalogFile(workDir)
	if err != nil {
		return "", err
	}
	if err := validateCatalog(workDir, catalog); err != nil {
		return "", err
	}
	if err := writeCatalogFile(workDir, catalog); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	gzw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	gzw.Name = filepath.Base(archivePath)
	gzw.ModTime = time.Unix(0, 0)

	tw := tar.NewWriter(gzw)
	if err := writeTarDir(tw, "icons"); err != nil {
		return "", err
	}
	iconPaths, err := collectWorkspaceIconPaths(workDir)
	if err != nil {
		return "", err
	}
	for _, iconPath := range iconPaths {
		rel, err := filepath.Rel(workDir, iconPath)
		if err != nil {
			return "", err
		}
		body, err := os.ReadFile(iconPath)
		if err != nil {
			return "", err
		}
		if !isSVG(iconPath, body) {
			return "", fmt.Errorf("%s is not an SVG", rel)
		}
		if err := writeTarFile(tw, filepath.ToSlash(rel), body, 0o644); err != nil {
			return "", err
		}
	}
	catalogBody, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return "", err
	}
	catalogBody = append(catalogBody, '\n')
	if err := writeTarFile(tw, "icons.json", catalogBody, 0o644); err != nil {
		return "", err
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gzw.Close(); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(filepath.Dir(archivePath), filepath.Base(archivePath)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	validationDir, err := os.MkdirTemp("", "tld-icon-browser-validate-*")
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	defer func() { _ = os.RemoveAll(validationDir) }()
	if err := unpackArchive(tmpPath, validationDir); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("validate repacked archive: %w", err)
	}

	backupPath := archivePath + ".bak." + time.Now().UTC().Format("20060102T150405Z")
	if err := copyPath(backupPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("create backup: %w", err)
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return backupPath, nil
}

func readCatalogFile(workDir string) ([]catalogItem, error) {
	body, err := os.ReadFile(filepath.Join(workDir, "icons.json"))
	if err != nil {
		return nil, err
	}
	var catalog []catalogItem
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}

func writeCatalogFile(workDir string, catalog []catalogItem) error {
	normalized, err := normalizeCatalog(catalog)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	target := filepath.Join(workDir, "icons.json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

func validateCatalog(workDir string, catalog []catalogItem) error {
	if len(catalog) == 0 {
		return errors.New("catalog is empty")
	}
	seen := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		normalized, err := normalizeExistingItem(item)
		if err != nil {
			return err
		}
		if seen[normalized.DefaultSlug] {
			return fmt.Errorf("duplicate catalog slug %q", normalized.DefaultSlug)
		}
		seen[normalized.DefaultSlug] = true
		iconName, ok := iconURLToFilename(normalized.IconURL)
		if !ok {
			return fmt.Errorf("catalog entry %q has unsafe iconUrl %q", normalized.DefaultSlug, normalized.IconURL)
		}
		iconPath := filepath.Join(workDir, "icons", iconName)
		info, err := os.Stat(iconPath)
		if err != nil {
			return fmt.Errorf("catalog entry %q references missing icon %q", normalized.DefaultSlug, normalized.IconURL)
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return fmt.Errorf("catalog entry %q icon is not a regular file", normalized.DefaultSlug)
		}
		body, err := os.ReadFile(iconPath)
		if err != nil {
			return err
		}
		if !isSVG(iconPath, body) {
			return fmt.Errorf("catalog entry %q icon is not an SVG", normalized.DefaultSlug)
		}
	}
	return nil
}

func normalizeCatalog(catalog []catalogItem) ([]catalogItem, error) {
	out := make([]catalogItem, 0, len(catalog))
	for _, item := range catalog {
		normalized, err := normalizeExistingItem(item)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeNewItem(item catalogItem) (catalogItem, error) {
	if strings.TrimSpace(item.DefaultSlug) == "" {
		item.DefaultSlug = slugify(item.Name)
	} else {
		item.DefaultSlug = slugify(item.DefaultSlug)
	}
	return normalizeExistingItem(item)
}

func normalizeExistingItem(item catalogItem) (catalogItem, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.NameShort = strings.TrimSpace(item.NameShort)
	item.DefaultSlug = strings.ToLower(strings.TrimSpace(item.DefaultSlug))
	item.Provider = strings.TrimSpace(item.Provider)
	item.DocsURL = strings.TrimSpace(item.DocsURL)
	item.Description = strings.TrimSpace(item.Description)
	item.WebsiteURL = strings.TrimSpace(item.WebsiteURL)
	item.Aliases = normalizeAliases(item.Aliases)
	if item.DefaultSlug == "" {
		return catalogItem{}, errors.New("defaultSlug is required")
	}
	if !validSlugRE.MatchString(item.DefaultSlug) {
		return catalogItem{}, fmt.Errorf("defaultSlug %q must contain only lowercase letters, numbers, and hyphens", item.DefaultSlug)
	}
	if item.Name == "" {
		item.Name = item.DefaultSlug
	}
	if item.NameShort == "" {
		item.NameShort = item.Name
	}
	if strings.TrimSpace(item.IconURL) == "" {
		item.IconURL = "/icons/" + item.DefaultSlug + ".svg"
	}
	if _, ok := iconURLToFilename(item.IconURL); !ok {
		return catalogItem{}, fmt.Errorf("iconUrl %q must point to /icons/<name>.svg", item.IconURL)
	}
	return item, nil
}

func normalizeAliases(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitAliases(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return normalizeAliases(parts)
}

func renameIconFile(workDir string, oldItem catalogItem, newItem catalogItem) error {
	oldName, ok := iconURLToFilename(oldItem.IconURL)
	if !ok {
		oldName = oldItem.DefaultSlug + ".svg"
	}
	newName, ok := iconURLToFilename(newItem.IconURL)
	if !ok {
		return fmt.Errorf("new iconUrl %q is invalid", newItem.IconURL)
	}
	if oldName == newName {
		return nil
	}
	oldPath := filepath.Join(workDir, "icons", oldName)
	newPath := filepath.Join(workDir, "icons", newName)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("cannot rename icon to %q because that file already exists", newName)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(oldPath); err != nil {
		return fmt.Errorf("cannot rename missing icon file %q", oldName)
	}
	return os.Rename(oldPath, newPath)
}

func listIconFiles(workDir string) ([]iconFile, error) {
	iconRoot := filepath.Join(workDir, "icons")
	entries, err := os.ReadDir(iconRoot)
	if err != nil {
		return nil, err
	}
	files := make([]iconFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := cleanIconFilename(name); !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, iconFile{
			Path: "icons/" + name,
			URL:  "/icons/" + name,
			Size: info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func collectWorkspaceIconPaths(workDir string) ([]string, error) {
	iconRoot := filepath.Join(workDir, "icons")
	entries, err := os.ReadDir(iconRoot)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := cleanIconFilename(name); !ok {
			continue
		}
		paths = append(paths, filepath.Join(iconRoot, name))
	}
	sort.Strings(paths)
	return paths, nil
}

func writeTarDir(tw *tar.Writer, name string) error {
	return tw.WriteHeader(&tar.Header{
		Name:     name + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
		ModTime:  time.Unix(0, 0),
	})
}

func writeTarFile(tw *tar.Writer, name string, body []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
		ModTime:  time.Unix(0, 0),
	}); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}

func writeFile(filePath string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func copyPath(dst string, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func cleanArchiveName(name string) (string, bool) {
	cleanName := path.Clean(strings.TrimSpace(name))
	if cleanName == "." || cleanName == ".." || path.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") {
		return "", false
	}
	if cleanName == "icons.json" || cleanName == "icons" || strings.HasPrefix(cleanName, "icons/") {
		return cleanName, true
	}
	return "", false
}

func safeJoin(root string, cleanName string) (string, bool) {
	target := filepath.Join(root, filepath.FromSlash(cleanName))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}

func cleanIconFilename(name string) (string, bool) {
	cleanName := path.Clean(strings.TrimSpace(name))
	if cleanName == "." || cleanName == ".." || path.IsAbs(cleanName) || strings.Contains(cleanName, "/") {
		return "", false
	}
	if !strings.EqualFold(path.Ext(cleanName), ".svg") {
		return "", false
	}
	return cleanName, true
}

func iconURLToFilename(iconURL string) (string, bool) {
	iconURL = strings.TrimSpace(iconURL)
	if !strings.HasPrefix(iconURL, "/icons/") {
		return "", false
	}
	return cleanIconFilename(strings.TrimPrefix(iconURL, "/icons/"))
}

func cleanMode(mode int64, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return os.FileMode(mode).Perm()
}

func slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugPartRE.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

func isSVG(filePath string, body []byte) bool {
	if !strings.EqualFold(filepath.Ext(filePath), ".svg") {
		return false
	}
	content := strings.TrimSpace(string(body))
	return strings.HasPrefix(content, "<svg") || strings.HasPrefix(content, "<?xml")
}

func findCatalogIndex(catalog []catalogItem, slug string) int {
	for i, item := range catalog {
		if item.DefaultSlug == slug {
			return i
		}
	}
	return -1
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST, PATCH")
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiError{Error: err.Error()})
}
