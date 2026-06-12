package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/git"
	api "github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
)

const managedViewMarkdownDir = "view-markdown"

const (
	markdownSourcePrivateWorkspace = "PRIVATE_WORKSPACE"
	markdownSourcePrivateApp       = "PRIVATE_APP"
	markdownSourceRepo             = "REPO"
	markdownSourceAttached         = "ATTACHED"
	markdownSourceLegacy           = "LEGACY"

	markdownGitOutsideRepo = "outside_repo"
	markdownGitUnknown     = "unknown"
)

func (a *APIAdapter) GetViewMarkdown(ctx context.Context, viewID int32, _ uuid.UUID) (*diagv1.ViewMarkdownDocument, string, error) {
	if _, err := a.Store.legacy.ViewByID(ctx, int64(viewID)); err != nil {
		return nil, "", err
	}
	doc, err := a.Store.legacy.ViewMarkdownByViewID(ctx, int64(viewID))
	if err != nil {
		return nil, "", err
	}
	if doc == nil {
		return nil, "", sql.ErrNoRows
	}
	absPath, err := a.resolveStoredMarkdownDocumentPath(doc)
	if err != nil {
		return nil, "", err
	}
	content, info, err := readMarkdownFile(absPath)
	if err != nil {
		return nil, "", err
	}
	return a.viewMarkdownToProto(ctx, viewID, doc, absPath, info), content, nil
}

func (a *APIAdapter) CreateViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, fileName *string, initialContent *string, targetKind string, path *string) (*diagv1.View, error) {
	view, err := a.Store.legacy.ViewByID(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	if existing, err := a.Store.legacy.ViewMarkdownByViewID(ctx, int64(viewID)); err != nil {
		return nil, err
	} else if existing != nil {
		return a.GetView(ctx, viewID, workspaceID)
	}
	storedPath, absPath, sourceKind, isManaged, err := a.createMarkdownTargetPath(viewID, view.Name, fileName, targetKind, path)
	if err != nil {
		return nil, err
	}
	content := ""
	if initialContent != nil {
		content = *initialContent
	}
	if err := writeExclusiveMarkdownFile(absPath, content); err != nil {
		return nil, err
	}
	if err := a.Store.legacy.UpsertViewMarkdown(ctx, int64(viewID), storedPath, isManaged, nowString(), sourceKind); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

func (a *APIAdapter) LinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, path string) (*diagv1.View, error) {
	if _, err := a.Store.legacy.ViewByID(ctx, int64(viewID)); err != nil {
		return nil, err
	}
	storedPath, absPath, err := a.normalizeLinkedMarkdownPath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("markdown path must point to a file")
	}
	if err := a.Store.legacy.UpsertViewMarkdown(ctx, int64(viewID), storedPath, false, info.ModTime().UTC().Format(time.RFC3339), markdownSourceAttached); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

func (a *APIAdapter) SaveViewMarkdown(ctx context.Context, viewID int32, _ uuid.UUID, content string, expectedFileVersion *string, force bool) (*diagv1.ViewMarkdownDocument, error) {
	doc, err := a.Store.legacy.ViewMarkdownByViewID(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, sql.ErrNoRows
	}
	absPath, err := a.resolveStoredMarkdownDocumentPath(doc)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && info.IsDir() {
		return nil, fmt.Errorf("markdown path must point to a file")
	}
	if expectedFileVersion != nil && !force {
		currentVersion := ""
		if err == nil {
			currentVersion = markdownFileVersion(info)
		}
		if currentVersion != *expectedFileVersion {
			return nil, api.ErrMarkdownFileChanged
		}
	}
	if err == nil && !markdownFileWritable(info) {
		return nil, os.ErrPermission
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return nil, err
	}
	updatedAt := nowString()
	sourceKind := normalizeMarkdownSourceKind(doc.SourceKind, doc.IsManaged)
	if err := a.Store.legacy.UpsertViewMarkdown(ctx, int64(viewID), doc.Path, doc.IsManaged, updatedAt, sourceKind); err != nil {
		return nil, err
	}
	nextInfo, statErr := os.Stat(absPath)
	if statErr != nil {
		return nil, statErr
	}
	updated := &app.ViewMarkdownDocument{
		Path:       doc.Path,
		IsManaged:  doc.IsManaged,
		UpdatedAt:  updatedAt,
		SourceKind: sourceKind,
	}
	return a.viewMarkdownToProto(ctx, viewID, updated, absPath, nextInfo), nil
}

func (a *APIAdapter) UnlinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, deleteManagedFile bool) (*diagv1.View, error) {
	doc, err := a.Store.legacy.ViewMarkdownByViewID(ctx, int64(viewID))
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return a.GetView(ctx, viewID, workspaceID)
	}
	if deleteManagedFile && doc.IsManaged {
		absPath, err := a.resolveStoredMarkdownDocumentPath(doc)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	if err := a.Store.legacy.DeleteViewMarkdown(ctx, int64(viewID)); err != nil {
		return nil, err
	}
	return a.GetView(ctx, viewID, workspaceID)
}

func viewMarkdownToProto(doc *app.ViewMarkdownDocument) *diagv1.ViewMarkdownDocument {
	if doc == nil {
		return nil
	}
	sourceKind := normalizeMarkdownSourceKind(doc.SourceKind, doc.IsManaged)
	return &diagv1.ViewMarkdownDocument{
		Path:       doc.Path,
		IsManaged:  doc.IsManaged,
		UpdatedAt:  ts(doc.UpdatedAt),
		SourceKind: sourceKind,
		Exists:     true,
		Writable:   true,
		CanEdit:    true,
		GitState:   markdownGitUnknown,
	}
}

func (a *APIAdapter) viewMarkdownToProto(ctx context.Context, viewID int32, doc *app.ViewMarkdownDocument, absPath string, info os.FileInfo) *diagv1.ViewMarkdownDocument {
	proto := viewMarkdownToProto(doc)
	if proto == nil {
		return nil
	}
	exists := info != nil
	writable := exists && markdownFileWritable(info)
	proto.Exists = exists
	proto.Writable = writable
	proto.CanEdit = exists && writable
	proto.FileVersion = markdownFileVersion(info)
	if linkedCount, err := a.Store.legacy.CountViewMarkdownByPath(ctx, doc.Path); err == nil {
		proto.LinkedViewCount = int32(linkedCount)
	}
	if root, ok := a.workspaceContentRoot(); ok {
		gitState, repoRelPath, err := git.FileState(root, absPath)
		if err != nil {
			proto.GitState = markdownGitUnknown
		} else {
			proto.GitState = gitState
			if repoRelPath != "" {
				proto.RepoRelativePath = &repoRelPath
			}
		}
	} else {
		proto.GitState = markdownGitOutsideRepo
	}
	return proto
}

func readMarkdownFile(absPath string) (string, os.FileInfo, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("markdown path must point to a file")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil, err
	}
	return string(content), info, nil
}

func writeExclusiveMarkdownFile(absPath string, content string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func markdownFileWritable(info os.FileInfo) bool {
	return info != nil && !info.IsDir() && info.Mode().Perm()&0o222 != 0
}

func markdownFileVersion(info os.FileInfo) string {
	if info == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UTC().UnixNano(), info.Size())
}

func normalizeMarkdownSourceKind(sourceKind string, isManaged bool) string {
	switch strings.ToUpper(strings.TrimSpace(sourceKind)) {
	case markdownSourcePrivateWorkspace:
		return markdownSourcePrivateWorkspace
	case markdownSourcePrivateApp:
		return markdownSourcePrivateApp
	case markdownSourceRepo:
		return markdownSourceRepo
	case markdownSourceAttached:
		return markdownSourceAttached
	case markdownSourceLegacy:
		return markdownSourceLegacy
	case "":
		if isManaged {
			return markdownSourcePrivateApp
		}
		return markdownSourceLegacy
	default:
		if isManaged {
			return markdownSourcePrivateApp
		}
		return markdownSourceAttached
	}
}

func normalizeMarkdownTargetKind(targetKind string) string {
	switch strings.ToUpper(strings.TrimSpace(targetKind)) {
	case "", "PRIVATE", markdownSourcePrivateWorkspace:
		return markdownSourcePrivateWorkspace
	case markdownSourcePrivateApp:
		return markdownSourcePrivateApp
	case markdownSourceRepo:
		return markdownSourceRepo
	default:
		return strings.ToUpper(strings.TrimSpace(targetKind))
	}
}

func (a *APIAdapter) createMarkdownTargetPath(viewID int32, viewName string, fileName *string, targetKind string, path *string) (string, string, string, bool, error) {
	switch normalizeMarkdownTargetKind(targetKind) {
	case markdownSourcePrivateWorkspace:
		if workspaceDir, ok := a.workspaceNotesRoot(); ok {
			storedPath := defaultManagedMarkdownRelativePath(viewID, viewName, fileName)
			return storedPath, filepath.Join(workspaceDir, storedPath), markdownSourcePrivateWorkspace, true, nil
		}
		storedPath, absPath, err := a.managedMarkdownPath(viewID, viewName, fileName)
		return storedPath, absPath, markdownSourcePrivateApp, true, err
	case markdownSourcePrivateApp:
		storedPath, absPath, err := a.managedMarkdownPath(viewID, viewName, fileName)
		return storedPath, absPath, markdownSourcePrivateApp, true, err
	case markdownSourceRepo:
		root, ok := a.workspaceContentRoot()
		if !ok {
			return "", "", "", false, fmt.Errorf("workspace directory is not configured")
		}
		targetPath := ""
		if path != nil {
			targetPath = strings.TrimSpace(*path)
		}
		if targetPath == "" {
			targetPath = filepath.Join("docs", "diagrams", sanitizeMarkdownBaseName(viewName)+".md")
		}
		storedPath, absPath, err := normalizePathUnderRoot(root, targetPath)
		if err != nil {
			return "", "", "", false, err
		}
		return storedPath, absPath, markdownSourceRepo, true, nil
	default:
		return "", "", "", false, fmt.Errorf("unsupported markdown target kind %q", targetKind)
	}
}

func (a *APIAdapter) resolveStoredMarkdownDocumentPath(doc *app.ViewMarkdownDocument) (string, error) {
	if doc == nil {
		return "", sql.ErrNoRows
	}
	sourceKind := normalizeMarkdownSourceKind(doc.SourceKind, doc.IsManaged)
	switch sourceKind {
	case markdownSourcePrivateWorkspace:
		if filepath.IsAbs(doc.Path) {
			return filepath.Clean(doc.Path), nil
		}
		if workspaceDir, ok := a.workspaceNotesRoot(); ok {
			return filepath.Join(workspaceDir, filepath.Clean(doc.Path)), nil
		}
		return a.resolvePathFromDataDir(doc.Path)
	case markdownSourceRepo, markdownSourceAttached:
		if filepath.IsAbs(doc.Path) {
			return filepath.Clean(doc.Path), nil
		}
		if root, ok := a.workspaceContentRoot(); ok {
			return filepath.Join(root, filepath.Clean(doc.Path)), nil
		}
		return a.resolvePathFromDataDir(doc.Path)
	case markdownSourcePrivateApp, markdownSourceLegacy:
		return a.resolvePathFromDataDir(doc.Path)
	default:
		return a.resolvePathFromDataDir(doc.Path)
	}
}

func (a *APIAdapter) resolvePathFromDataDir(storedPath string) (string, error) {
	if filepath.IsAbs(storedPath) {
		return filepath.Clean(storedPath), nil
	}
	dataDir, err := a.requireDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, filepath.Clean(storedPath)), nil
}

func (a *APIAdapter) normalizeLinkedMarkdownPath(path string) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", fmt.Errorf("markdown path must not be empty")
	}
	if root, ok := a.workspaceContentRoot(); ok {
		if filepath.IsAbs(trimmed) {
			absPath := filepath.Clean(trimmed)
			if relPath, ok := relativePathIfInside(root, absPath); ok {
				return relPath, absPath, nil
			}
			return absPath, absPath, nil
		}
		return normalizePathUnderRoot(root, trimmed)
	}
	dataDir, err := a.requireDataDir()
	if err != nil {
		return "", "", err
	}
	var absPath string
	if filepath.IsAbs(trimmed) {
		absPath = filepath.Clean(trimmed)
	} else {
		absPath = filepath.Clean(filepath.Join(dataDir, trimmed))
	}
	if relPath, ok := relativePathIfInside(dataDir, absPath); ok {
		return relPath, absPath, nil
	}
	return absPath, absPath, nil
}

func (a *APIAdapter) managedMarkdownPath(viewID int32, viewName string, fileName *string) (string, string, error) {
	dataDir, err := a.requireDataDir()
	if err != nil {
		return "", "", err
	}
	storedPath := defaultManagedMarkdownRelativePath(viewID, viewName, fileName)
	return storedPath, filepath.Join(dataDir, storedPath), nil
}

func defaultManagedMarkdownRelativePath(viewID int32, viewName string, fileName *string) string {
	baseName := viewName
	if fileName != nil && strings.TrimSpace(*fileName) != "" {
		baseName = strings.TrimSpace(*fileName)
	}
	baseName = strings.TrimSuffix(filepath.Base(baseName), filepath.Ext(baseName))
	slug := sanitizeMarkdownBaseName(baseName)
	return filepath.Join(managedViewMarkdownDir, fmt.Sprintf("view-%d-%s.md", viewID, slug))
}

func (a *APIAdapter) workspaceNotesRoot() (string, bool) {
	if strings.TrimSpace(a.WorkspaceDir) == "" {
		return "", false
	}
	abs, err := filepath.Abs(a.WorkspaceDir)
	if err != nil {
		return "", false
	}
	base := filepath.Base(abs)
	if base == ".tld" || base == "tld" {
		return abs, true
	}
	if _, err := os.Stat(filepath.Join(abs, ".tld")); err == nil {
		return filepath.Join(abs, ".tld"), true
	}
	if _, err := os.Stat(filepath.Join(abs, "tld")); err == nil {
		return filepath.Join(abs, "tld"), true
	}
	return filepath.Join(abs, ".tld"), true
}

func (a *APIAdapter) workspaceContentRoot() (string, bool) {
	if strings.TrimSpace(a.WorkspaceDir) == "" {
		return "", false
	}
	abs, err := filepath.Abs(a.WorkspaceDir)
	if err != nil {
		return "", false
	}
	base := filepath.Base(abs)
	if base == ".tld" || base == "tld" {
		return filepath.Dir(abs), true
	}
	return abs, true
}

func normalizePathUnderRoot(root string, path string) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", fmt.Errorf("markdown path must not be empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	var absPath string
	if filepath.IsAbs(trimmed) {
		absPath = filepath.Clean(trimmed)
	} else {
		absPath = filepath.Clean(filepath.Join(absRoot, trimmed))
	}
	relPath, ok := relativePathIfInside(absRoot, absPath)
	if !ok {
		return "", "", fmt.Errorf("markdown path must stay inside the workspace")
	}
	return relPath, absPath, nil
}

func relativePathIfInside(root string, absPath string) (string, bool) {
	relPath, err := filepath.Rel(root, absPath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(relPath)), true
}

func (a *APIAdapter) requireDataDir() (string, error) {
	if strings.TrimSpace(a.DataDir) == "" {
		return "", fmt.Errorf("data directory is not configured")
	}
	return a.DataDir, nil
}

func sanitizeMarkdownBaseName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "notes"
	}
	var builder strings.Builder
	lastWasDash := false
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastWasDash = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastWasDash = false
		default:
			if !lastWasDash {
				builder.WriteByte('-')
				lastWasDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "notes"
	}
	return slug
}
