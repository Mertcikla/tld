package workspacesource

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	mermaidcore "github.com/mertcikla/tld/v2/internal/mermaid"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func GetStatus(opts Options) Status {
	root, viewsDir, err := workspace.ResolveWorkspaceSourceRoot(opts.WorkspaceDir, opts.RepositoryName)
	if err != nil {
		return Status{Available: false, Message: err.Error()}
	}
	return Status{Available: true, RootPath: root, ViewsDir: viewsDir}
}

func Export(ctx context.Context, store Store, opts Options) (*Result, error) {
	root, viewsDir, err := workspace.ResolveWorkspaceSourceRoot(opts.WorkspaceDir, opts.RepositoryName)
	if err != nil {
		return nil, err
	}
	state, err := loadState(ctx, store, opts.WorkspaceID)
	if err != nil {
		return nil, err
	}
	lock, sourceLock, err := loadWorkspaceSourceLock(opts.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	refs := newRefResolver(sourceLock, state)
	exported, err := buildExportDocuments(state, refs)
	if err != nil {
		return nil, err
	}

	if err := preflightExport(root, sourceLock, exported.docs); err != nil {
		return nil, err
	}
	if err := writeExportDocuments(root, exported.docs); err != nil {
		return nil, err
	}
	hash, err := HashTree(root)
	if err != nil {
		return nil, err
	}

	sourceLock.ViewsDir = viewsDir
	sourceLock.LastHash = hash
	sourceLock.ManagedElements = exported.elementMetadata
	sourceLock.ManagedViews = exported.viewMetadata
	sourceLock.ManagedConnectors = exported.connectorMetadata
	lock.WorkspaceSource = sourceLock
	if err := workspace.WriteLockFile(opts.WorkspaceDir, lock); err != nil {
		return nil, err
	}

	return &Result{
		Available: true,
		RootPath:  root,
		ViewsDir:  viewsDir,
		Hash:      hash,
		Views: ChangeCounts{
			Applied: len(exported.viewMetadata),
			Updated: len(exported.viewMetadata),
		},
		Elements: ChangeCounts{
			Applied: len(exported.elementMetadata),
			Updated: len(exported.elementMetadata),
		},
		Connectors: ChangeCounts{
			Applied: len(exported.connectorMetadata),
			Updated: len(exported.connectorMetadata),
		},
		Warnings: exported.warnings,
	}, nil
}

func preflightExport(root string, sourceLock *workspace.WorkspaceSourceLock, docs []exportDocument) error {
	legacy, err := legacyMMDRelPaths(root)
	if err != nil {
		return err
	}
	if len(legacy) > 0 {
		return fmt.Errorf("workspace-source views dir contains legacy .mmd files; rename them to %s before exporting: %s", ViewFileName, strings.Join(legacy, ", "))
	}
	existing, rootExists, err := existingWorkspaceSourceRelPaths(root)
	if err != nil {
		return err
	}
	lastHash := ""
	if sourceLock != nil {
		lastHash = strings.TrimSpace(sourceLock.LastHash)
	}
	if lastHash != "" && !rootExists {
		return fmt.Errorf("workspace-source views dir %s is missing; aborting export because a previous source hash is recorded", root)
	}
	currentHash, err := HashTree(root)
	if err != nil {
		return fmt.Errorf("hash workspace-source views dir: %w", err)
	}
	if lastHash != "" && currentHash != lastHash {
		return fmt.Errorf("workspace-source views dir changed since last import/export; aborting export (expected %s, got %s)", lastHash, currentHash)
	}
	existingPaths := sortedStringSet(existing)
	if lastHash == "" && len(existingPaths) > 0 {
		return fmt.Errorf("workspace-source views dir contains unmanaged %s files; import or move them before exporting: %s", ViewFileName, strings.Join(existingPaths, ", "))
	}

	expected := expectedExportWorkspaceSourceRelPaths(docs)
	stale := make([]string, 0)
	for rel := range existing {
		if _, ok := expected[rel]; !ok {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		return fmt.Errorf("workspace-source export would leave stale %s files; remove or import them before exporting: %s", ViewFileName, strings.Join(stale, ", "))
	}
	return nil
}

func expectedExportWorkspaceSourceRelPaths(docs []exportDocument) map[string]struct{} {
	expected := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		expected[exportDocumentRelPath(doc)] = struct{}{}
	}
	return expected
}

func existingWorkspaceSourceRelPaths(root string) (map[string]struct{}, bool, error) {
	paths := map[string]struct{}{}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, false, nil
		}
		return nil, false, fmt.Errorf("stat workspace-source views dir: %w", err)
	}
	if !info.IsDir() {
		return nil, true, fmt.Errorf("workspace-source views dir %s is not a directory", root)
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(path) != ViewFileName {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = struct{}{}
		return nil
	}); err != nil {
		return nil, true, fmt.Errorf("scan workspace-source views dir: %w", err)
	}
	return paths, true, nil
}

func legacyMMDRelPaths(root string) ([]string, error) {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".mmd" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan legacy .mmd files: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func writeExportDocuments(root string, docs []exportDocument) error {
	for _, doc := range docs {
		path := exportDocumentPath(root, doc)
		if err := prepareExportTarget(path); err != nil {
			return err
		}
	}
	for _, doc := range docs {
		path := exportDocumentPath(root, doc)
		if err := writeFileAtomic(path, []byte(doc.content)); err != nil {
			return err
		}
	}
	return nil
}

func prepareExportTarget(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("workspace-source target directory %s is not a directory", dir)
	}
	if dirInfo.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("workspace-source target directory %s is not writable", dir)
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace-source target %s is not a regular file", path)
	}
	if info.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("workspace-source target %s is not writable", path)
	}
	return nil
}

func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".view-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := file.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod temp for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	tmpPath = ""
	return nil
}

func exportDocumentPath(root string, doc exportDocument) string {
	return filepath.Join(root, filepath.FromSlash(doc.relDir), ViewFileName)
}

func exportDocumentRelPath(doc exportDocument) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(doc.relDir), ViewFileName))
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func Import(ctx context.Context, store Store, opts Options, dryRun bool) (*Result, error) {
	root, viewsDir, err := workspace.ResolveWorkspaceSourceRoot(opts.WorkspaceDir, opts.RepositoryName)
	if err != nil {
		return nil, err
	}
	desired, err := ParseTree(root)
	if err != nil {
		return nil, err
	}
	state, err := loadState(ctx, store, opts.WorkspaceID)
	if err != nil {
		return nil, err
	}
	lock, sourceLock, err := loadWorkspaceSourceLock(opts.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	plan := planImport(desired, state, sourceLock)
	result := &Result{
		Available:  true,
		DryRun:     dryRun,
		RootPath:   root,
		ViewsDir:   viewsDir,
		Hash:       desired.SourceHash,
		Views:      plan.views,
		Elements:   plan.elements,
		Connectors: plan.connectors,
		Warnings:   append([]string{}, desired.Warnings...),
	}
	result.Warnings = append(result.Warnings, plan.warnings...)
	if dryRun {
		return result, nil
	}

	applied, err := applyImport(ctx, store, opts.WorkspaceID, desired, state, sourceLock)
	if err != nil {
		return nil, err
	}
	result.Views = applied.views
	result.Elements = applied.elements
	result.Connectors = applied.connectors
	result.Warnings = append(result.Warnings, applied.warnings...)

	sourceLock.ViewsDir = viewsDir
	sourceLock.LastHash = desired.SourceHash
	lock.WorkspaceSource = sourceLock
	if err := workspace.WriteLockFile(opts.WorkspaceDir, lock); err != nil {
		return nil, err
	}
	return result, nil
}

type sqliteState struct {
	views             []*diagv1.View
	elements          []*diagv1.Element
	placements        []*diagv1.PlacedElement
	connectors        []*diagv1.Connector
	viewsByID         map[int32]*diagv1.View
	elementsByID      map[int32]*diagv1.Element
	connectorsByID    map[int32]*diagv1.Connector
	elementsByName    map[string][]*diagv1.Element
	viewsByName       map[string][]*diagv1.View
	placementsByView  map[int32]map[int32]*diagv1.PlacedElement
	connectorsByShape map[string]*diagv1.Connector
}

func loadState(ctx context.Context, store Store, workspaceID uuid.UUID) (*sqliteState, error) {
	views, err := store.ListViews(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	elements, _, err := store.ListElements(ctx, workspaceID, 0, 0, "")
	if err != nil {
		return nil, err
	}
	placements, err := store.ListAllPlacements(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	connectors, err := store.ListAllConnectors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	state := &sqliteState{
		views:             views,
		elements:          elements,
		placements:        placements,
		connectors:        connectors,
		viewsByID:         map[int32]*diagv1.View{},
		elementsByID:      map[int32]*diagv1.Element{},
		connectorsByID:    map[int32]*diagv1.Connector{},
		elementsByName:    map[string][]*diagv1.Element{},
		viewsByName:       map[string][]*diagv1.View{},
		placementsByView:  map[int32]map[int32]*diagv1.PlacedElement{},
		connectorsByShape: map[string]*diagv1.Connector{},
	}
	for _, view := range views {
		state.viewsByID[view.GetId()] = view
		name := normalizedName(view.GetName())
		state.viewsByName[name] = append(state.viewsByName[name], view)
	}
	for _, element := range elements {
		state.elementsByID[element.GetId()] = element
		name := normalizedName(element.GetName())
		state.elementsByName[name] = append(state.elementsByName[name], element)
	}
	for _, placement := range placements {
		byElement := state.placementsByView[placement.GetViewId()]
		if byElement == nil {
			byElement = map[int32]*diagv1.PlacedElement{}
			state.placementsByView[placement.GetViewId()] = byElement
		}
		byElement[placement.GetElementId()] = placement
	}
	for _, connector := range connectors {
		state.connectorsByID[connector.GetId()] = connector
		state.connectorsByShape[connectorShape(connector.GetViewId(), connector.GetSourceElementId(), connector.GetTargetElementId(), connector.GetLabel())] = connector
	}
	return state, nil
}

func loadWorkspaceSourceLock(workspaceDir string) (*workspace.LockFile, *workspace.WorkspaceSourceLock, error) {
	lock, err := workspace.LoadLockFile(workspaceDir)
	if err != nil {
		return nil, nil, err
	}
	if lock == nil {
		lock = &workspace.LockFile{Version: "v1"}
	}
	if lock.Version == "" {
		lock.Version = "v1"
	}
	if lock.WorkspaceSource == nil {
		lock.WorkspaceSource = &workspace.WorkspaceSourceLock{}
	}
	if lock.WorkspaceSource.ManagedElements == nil {
		lock.WorkspaceSource.ManagedElements = map[string]*workspace.ResourceMetadata{}
	}
	if lock.WorkspaceSource.ManagedViews == nil {
		lock.WorkspaceSource.ManagedViews = map[string]*workspace.ResourceMetadata{}
	}
	if lock.WorkspaceSource.ManagedConnectors == nil {
		lock.WorkspaceSource.ManagedConnectors = map[string]*workspace.ResourceMetadata{}
	}
	return lock, lock.WorkspaceSource, nil
}

type exportDocument struct {
	relDir  string
	content string
}

type exportBundle struct {
	docs              []exportDocument
	elementMetadata   map[string]*workspace.ResourceMetadata
	viewMetadata      map[string]*workspace.ResourceMetadata
	connectorMetadata map[string]*workspace.ResourceMetadata
	warnings          []string
}

func buildExportDocuments(state *sqliteState, refs *refResolver) (*exportBundle, error) {
	out := &exportBundle{
		docs:              []exportDocument{},
		elementMetadata:   map[string]*workspace.ResourceMetadata{},
		viewMetadata:      map[string]*workspace.ResourceMetadata{},
		connectorMetadata: map[string]*workspace.ResourceMetadata{},
	}
	children := map[int32][]*diagv1.View{}
	var roots []*diagv1.View
	for _, view := range state.views {
		if view.ParentViewId == nil {
			roots = append(roots, view)
			continue
		}
		children[view.GetParentViewId()] = append(children[view.GetParentViewId()], view)
	}
	sortViews(roots)
	for _, views := range children {
		sortViews(views)
	}
	var visit func(view *diagv1.View, parentRef, relParent string)
	visit = func(view *diagv1.View, parentRef, relParent string) {
		viewRef := refs.viewRef(view)
		dirName := safePathSegment(viewRef)
		relDir := dirName
		if relParent != "" {
			relDir = relParent + "/" + dirName
		}
		content, elementRefs, connectorRefs := renderViewDocument(view, parentRef, viewRef, state, refs)
		out.docs = append(out.docs, exportDocument{relDir: relDir, content: mermaidcore.MermaidBlock(content)})
		out.viewMetadata[viewRef] = resourceMetadata(view.GetId(), view.GetUpdatedAt())
		for ref, meta := range elementRefs {
			out.elementMetadata[ref] = meta
		}
		for ref, meta := range connectorRefs {
			out.connectorMetadata[ref] = meta
		}
		for _, child := range children[view.GetId()] {
			visit(child, viewRef, relDir)
		}
	}
	for _, root := range roots {
		visit(root, "root", "")
	}
	return out, nil
}

func renderViewDocument(view *diagv1.View, parentRef, viewRef string, state *sqliteState, refs *refResolver) (string, map[string]*workspace.ResourceMetadata, map[string]*workspace.ResourceMetadata) {
	var buffer bytes.Buffer
	elementMetadata := map[string]*workspace.ResourceMetadata{}
	connectorMetadata := map[string]*workspace.ResourceMetadata{}
	ownerRef := ""
	if view.OwnerElementId != nil {
		if owner := state.elementsByID[view.GetOwnerElementId()]; owner != nil {
			ownerRef = refs.elementRef(owner)
		}
	}
	viewEntries := []metadataEntry{
		{key: "ref", value: viewRef},
		{key: "parent", value: parentRef},
		{key: "name", value: view.GetName()},
	}
	if ownerRef != "" {
		viewEntries = append(viewEntries, metadataEntry{key: "owner", value: ownerRef})
	}
	buffer.WriteString("flowchart LR\n")
	buffer.WriteString(metadataComment("tld/v1 view", viewEntries))
	buffer.WriteByte('\n')

	placements := state.placementsByView[view.GetId()]
	placementList := make([]*diagv1.PlacedElement, 0, len(placements))
	for _, placement := range placements {
		placementList = append(placementList, placement)
	}
	sort.Slice(placementList, func(i, j int) bool {
		return placementList[i].GetElementId() < placementList[j].GetElementId()
	})
	aliases := map[int32]string{}
	for _, placement := range placementList {
		element := state.elementsByID[placement.GetElementId()]
		if element == nil {
			continue
		}
		ref := refs.elementRef(element)
		alias := mermaidAlias(ref, aliases)
		aliases[element.GetId()] = alias
		buffer.WriteString("  ")
		buffer.WriteString(alias)
		buffer.WriteString("[\"")
		buffer.WriteString(escapeMermaidLabel(placement.GetName()))
		buffer.WriteString("\"]\n")
		buffer.WriteString(metadataComment("tld-element", elementEntries(ref, placement)))
		buffer.WriteByte('\n')
		elementMetadata[ref] = resourceMetadata(element.GetId(), element.GetUpdatedAt())
	}

	connectorList := make([]*diagv1.Connector, 0)
	for _, connector := range state.connectors {
		if connector.GetViewId() == view.GetId() {
			if aliases[connector.GetSourceElementId()] != "" && aliases[connector.GetTargetElementId()] != "" {
				connectorList = append(connectorList, connector)
			}
		}
	}
	sort.Slice(connectorList, func(i, j int) bool { return connectorList[i].GetId() < connectorList[j].GetId() })
	if len(placementList) > 0 && len(connectorList) > 0 {
		buffer.WriteByte('\n')
	}
	for _, connector := range connectorList {
		sourceRef := refs.elementRef(state.elementsByID[connector.GetSourceElementId()])
		targetRef := refs.elementRef(state.elementsByID[connector.GetTargetElementId()])
		ref := refs.connectorRef(connector, viewRef, sourceRef, targetRef)
		buffer.WriteString("  ")
		buffer.WriteString(aliases[connector.GetSourceElementId()])
		if label := strings.TrimSpace(connector.GetLabel()); label != "" {
			buffer.WriteString(" -- \"")
			buffer.WriteString(escapeMermaidLabel(label))
			buffer.WriteString("\" --> ")
		} else {
			buffer.WriteString(" --> ")
		}
		buffer.WriteString(aliases[connector.GetTargetElementId()])
		buffer.WriteByte('\n')
		buffer.WriteString(metadataComment("tld-connector", connectorEntries(ref, viewRef, sourceRef, targetRef, connector)))
		buffer.WriteByte('\n')
		connectorMetadata[ref] = resourceMetadata(connector.GetId(), connector.GetUpdatedAt())
	}
	return buffer.String(), elementMetadata, connectorMetadata
}

func elementEntries(ref string, element *diagv1.PlacedElement) []metadataEntry {
	entries := []metadataEntry{
		{key: "ref", value: ref},
		{key: "x", value: compactNumber(element.GetPositionX())},
		{key: "y", value: compactNumber(element.GetPositionY())},
	}
	if value := strings.TrimSpace(element.GetKind()); value != "" && value != "system" {
		entries = append(entries, metadataEntry{key: "kind", value: value})
	}
	appendStringEntry := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			entries = append(entries, metadataEntry{key: key, value: value})
		}
	}
	appendStringEntry("desc", element.GetDescription())
	appendStringEntry("tech", element.GetTechnology())
	appendStringEntry("url", element.GetUrl())
	appendStringEntry("logo", element.GetLogoUrl())
	if tags := encodeStringList(element.GetTags()); tags != "" {
		entries = append(entries, metadataEntry{key: "tags", value: tags, encoded: true})
	}
	appendStringEntry("repo", element.GetRepo())
	appendStringEntry("branch", element.GetBranch())
	appendStringEntry("file", element.GetFilePath())
	appendStringEntry("lang", element.GetLanguage())
	if element.GetBypassNoiseGate() {
		entries = append(entries, metadataEntry{key: "bypass", value: "1"})
	}
	if element.GetHasView() {
		entries = append(entries, metadataEntry{key: "hasView", value: "1"})
	}
	appendStringEntry("viewLabel", element.GetViewLabel())
	return entries
}

func connectorEntries(ref, viewRef, sourceRef, targetRef string, connector *diagv1.Connector) []metadataEntry {
	entries := []metadataEntry{
		{key: "ref", value: ref},
		{key: "source", value: sourceRef},
		{key: "target", value: targetRef},
	}
	appendStringEntry := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			entries = append(entries, metadataEntry{key: key, value: value})
		}
	}
	appendStringEntry("label", connector.GetLabel())
	appendStringEntry("desc", connector.GetDescription())
	appendStringEntry("rel", connector.GetRelationship())
	if direction := strings.TrimSpace(connector.GetDirection()); direction != "" && direction != "forward" {
		entries = append(entries, metadataEntry{key: "dir", value: direction})
	}
	if style := strings.TrimSpace(connector.GetStyle()); style != "" && style != "bezier" {
		entries = append(entries, metadataEntry{key: "style", value: style})
	}
	appendStringEntry("url", connector.GetUrl())
	if sourceHandle := strings.TrimSpace(connector.GetSourceHandle()); sourceHandle != "" && sourceHandle != "right" {
		entries = append(entries, metadataEntry{key: "sourceHandle", value: sourceHandle})
	}
	if targetHandle := strings.TrimSpace(connector.GetTargetHandle()); targetHandle != "" && targetHandle != "left" {
		entries = append(entries, metadataEntry{key: "targetHandle", value: targetHandle})
	}
	_ = viewRef
	return entries
}

type importPlan struct {
	views      ChangeCounts
	elements   ChangeCounts
	connectors ChangeCounts
	warnings   []string
}

func planImport(desired *desiredWorkspace, state *sqliteState, lock *workspace.WorkspaceSourceLock) importPlan {
	plan := importPlan{}
	for _, ref := range desired.ElementOrder {
		plan.elements.Planned++
		if resolveElementID(ref, desired.Elements[ref], state, lock) != 0 {
			plan.elements.Updated++
		} else {
			plan.elements.Created++
		}
	}
	for _, ref := range desired.ViewOrder {
		plan.views.Planned++
		if resolveViewID(ref, desired.Views[ref], state, lock) != 0 {
			plan.views.Updated++
		} else {
			plan.views.Created++
		}
	}
	for _, ref := range desired.ConnectorOrder {
		plan.connectors.Planned++
		if resolveConnectorID(ref, desired.Connectors[ref], state, lock, nil, nil) != 0 {
			plan.connectors.Updated++
		} else {
			plan.connectors.Created++
		}
	}
	for ref := range lock.ManagedConnectors {
		if _, ok := desired.Connectors[ref]; !ok {
			plan.connectors.Deleted++
			plan.connectors.Planned++
		}
	}
	for ref := range lock.ManagedViews {
		if _, ok := desired.Views[ref]; !ok {
			plan.views.Deleted++
			plan.views.Planned++
		}
	}
	for ref := range lock.ManagedElements {
		if _, ok := desired.Elements[ref]; !ok {
			plan.elements.Deleted++
			plan.elements.Planned++
		}
	}
	return plan
}

func applyImport(ctx context.Context, store Store, workspaceID uuid.UUID, desired *desiredWorkspace, state *sqliteState, lock *workspace.WorkspaceSourceLock) (importPlan, error) {
	applied := importPlan{}
	elementIDs := map[string]int32{}
	viewIDs := map[string]int32{}

	for _, ref := range desired.ElementOrder {
		element := desired.Elements[ref]
		input := elementInput(element)
		id := resolveElementID(ref, element, state, lock)
		var saved *diagv1.Element
		var err error
		if id != 0 {
			saved, err = store.UpdateElement(ctx, id, workspaceID, input)
			if errors.Is(err, sql.ErrNoRows) {
				saved, err = store.CreateElement(ctx, workspaceID, input)
				applied.elements.Created++
			} else {
				applied.elements.Updated++
			}
		} else {
			saved, err = store.CreateElement(ctx, workspaceID, input)
			applied.elements.Created++
		}
		if err != nil {
			return applied, fmt.Errorf("sync element %s: %w", ref, err)
		}
		elementIDs[ref] = saved.GetId()
		lock.ManagedElements[ref] = resourceMetadata(saved.GetId(), saved.GetUpdatedAt())
		applied.elements.Applied++
	}

	for _, ref := range desired.ViewOrder {
		view := desired.Views[ref]
		var ownerID *int32
		if view.OwnerRef != "" {
			if id := elementIDs[view.OwnerRef]; id != 0 {
				ownerID = &id
			}
		}
		id := resolveViewID(ref, view, state, lock)
		var saved *diagv1.View
		var err error
		if id != 0 {
			saved, err = store.UpdateView(ctx, id, workspaceID, view.Name, nil, nil, nil)
			if errors.Is(err, sql.ErrNoRows) {
				saved, err = store.CreateView(ctx, workspaceID, ownerID, view.Name, nil, ownerID == nil)
				applied.views.Created++
			} else {
				applied.views.Updated++
			}
		} else {
			saved, err = store.CreateView(ctx, workspaceID, ownerID, view.Name, nil, ownerID == nil)
			applied.views.Created++
		}
		if err != nil {
			return applied, fmt.Errorf("sync view %s: %w", ref, err)
		}
		viewIDs[ref] = saved.GetId()
		lock.ManagedViews[ref] = resourceMetadata(saved.GetId(), saved.GetUpdatedAt())
		applied.views.Applied++
	}

	for _, placement := range desired.Placements {
		viewID := viewIDs[placement.ViewRef]
		elementID := elementIDs[placement.ElementRef]
		if viewID == 0 || elementID == 0 {
			continue
		}
		if _, err := store.AddPlacement(ctx, viewID, elementID, placement.X, placement.Y); err != nil {
			return applied, fmt.Errorf("sync placement %s in %s: %w", placement.ElementRef, placement.ViewRef, err)
		}
	}
	if err := pruneManagedPlacements(ctx, store, workspaceID, desired, state, lock, viewIDs, elementIDs); err != nil {
		return applied, err
	}

	for _, ref := range desired.ConnectorOrder {
		connector := desired.Connectors[ref]
		viewID := viewIDs[connector.ViewRef]
		sourceID := elementIDs[connector.SourceRef]
		targetID := elementIDs[connector.TargetRef]
		if viewID == 0 || sourceID == 0 || targetID == 0 {
			applied.warnings = append(applied.warnings, fmt.Sprintf("connector %q skipped because source, target, or view was not resolved", ref))
			continue
		}
		id := resolveConnectorID(ref, connector, state, lock, elementIDs, viewIDs)
		input := connectorInput(connector, viewID, sourceID, targetID)
		var saved *diagv1.Connector
		var err error
		if id != 0 {
			saved, err = store.UpdateConnector(ctx, id, workspaceID, input)
			if errors.Is(err, sql.ErrNoRows) {
				saved, err = store.CreateConnector(ctx, workspaceID, input)
				applied.connectors.Created++
			} else {
				applied.connectors.Updated++
			}
		} else {
			saved, err = store.CreateConnector(ctx, workspaceID, input)
			applied.connectors.Created++
		}
		if err != nil {
			return applied, fmt.Errorf("sync connector %s: %w", ref, err)
		}
		lock.ManagedConnectors[ref] = resourceMetadata(saved.GetId(), saved.GetUpdatedAt())
		applied.connectors.Applied++
	}

	for _, item := range sortedMetadata(lock.ManagedConnectors) {
		ref, meta := item.ref, item.meta
		if _, ok := desired.Connectors[ref]; ok {
			continue
		}
		if meta == nil || meta.ID == 0 {
			delete(lock.ManagedConnectors, ref)
			continue
		}
		if err := store.DeleteConnector(ctx, int32(meta.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			applied.warnings = append(applied.warnings, fmt.Sprintf("connector %q was not deleted: %v", ref, err))
			continue
		}
		delete(lock.ManagedConnectors, ref)
		applied.connectors.Deleted++
		applied.connectors.Applied++
	}
	for _, item := range sortedMetadata(lock.ManagedViews) {
		ref, meta := item.ref, item.meta
		if _, ok := desired.Views[ref]; ok {
			continue
		}
		if meta == nil || meta.ID == 0 {
			delete(lock.ManagedViews, ref)
			continue
		}
		if err := store.DeleteView(ctx, int32(meta.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			applied.warnings = append(applied.warnings, fmt.Sprintf("view %q was not deleted: %v", ref, err))
			continue
		}
		delete(lock.ManagedViews, ref)
		applied.views.Deleted++
		applied.views.Applied++
	}
	for _, item := range sortedMetadata(lock.ManagedElements) {
		ref, meta := item.ref, item.meta
		if _, ok := desired.Elements[ref]; ok {
			continue
		}
		if meta == nil || meta.ID == 0 {
			delete(lock.ManagedElements, ref)
			continue
		}
		if err := store.DeleteElement(ctx, int32(meta.ID), workspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			applied.warnings = append(applied.warnings, fmt.Sprintf("element %q was not deleted: %v", ref, err))
			continue
		}
		delete(lock.ManagedElements, ref)
		applied.elements.Deleted++
		applied.elements.Applied++
	}
	return applied, nil
}

func pruneManagedPlacements(ctx context.Context, store Store, workspaceID uuid.UUID, desired *desiredWorkspace, state *sqliteState, lock *workspace.WorkspaceSourceLock, viewIDs, elementIDs map[string]int32) error {
	desiredByView := map[int32]map[int32]struct{}{}
	for _, placement := range desired.Placements {
		viewID := viewIDs[placement.ViewRef]
		elementID := elementIDs[placement.ElementRef]
		if viewID == 0 || elementID == 0 {
			continue
		}
		if desiredByView[viewID] == nil {
			desiredByView[viewID] = map[int32]struct{}{}
		}
		desiredByView[viewID][elementID] = struct{}{}
	}
	managedElementIDs := map[int32]struct{}{}
	for _, meta := range lock.ManagedElements {
		if meta != nil && meta.ID != 0 {
			managedElementIDs[int32(meta.ID)] = struct{}{}
		}
	}
	for ref, view := range desired.Views {
		viewID := viewIDs[ref]
		if viewID == 0 {
			continue
		}
		for elementID := range state.placementsByView[viewID] {
			if _, managed := managedElementIDs[elementID]; !managed {
				continue
			}
			if _, keep := desiredByView[viewID][elementID]; keep {
				continue
			}
			if err := store.RemovePlacement(ctx, viewID, elementID); err != nil {
				return fmt.Errorf("remove stale placement from view %q: %w", view.Ref, err)
			}
		}
	}
	return nil
}

func elementInput(element *desiredElement) api.ElementInput {
	kind := optionalString(element.Kind)
	if kind == nil {
		defaultKind := "system"
		kind = &defaultKind
	}
	return api.ElementInput{
		Name:            element.Name,
		Description:     optionalString(element.Description),
		Kind:            kind,
		Technology:      optionalString(element.Technology),
		URL:             optionalString(element.URL),
		LogoURL:         optionalString(element.LogoURL),
		Tags:            append([]string{}, element.Tags...),
		Repo:            optionalString(element.Repo),
		Branch:          optionalString(element.Branch),
		Language:        optionalString(element.Language),
		FilePath:        optionalString(element.FilePath),
		BypassNoiseGate: element.BypassNoiseGate,
		HasView:         element.HasView,
		ViewLabel:       optionalString(element.ViewLabel),
	}
}

func connectorInput(connector *desiredConnector, viewID, sourceID, targetID int32) api.ConnectorInput {
	return api.ConnectorInput{
		ViewID:       viewID,
		SourceID:     sourceID,
		TargetID:     targetID,
		Label:        optionalString(connector.Label),
		Description:  optionalString(connector.Description),
		Relationship: optionalString(connector.Relationship),
		Direction:    stringDefault(connector.Direction, "forward"),
		Style:        stringDefault(connector.Style, "bezier"),
		URL:          optionalString(connector.URL),
		SourceHandle: optionalString(connector.SourceHandle),
		TargetHandle: optionalString(connector.TargetHandle),
	}
}

func resolveElementID(ref string, element *desiredElement, state *sqliteState, lock *workspace.WorkspaceSourceLock) int32 {
	if meta := lock.ManagedElements[ref]; meta != nil && meta.ID != 0 {
		if _, ok := state.elementsByID[int32(meta.ID)]; ok {
			return int32(meta.ID)
		}
	}
	matches := state.elementsByName[normalizedName(element.Name)]
	if len(matches) == 1 {
		return matches[0].GetId()
	}
	return 0
}

func resolveViewID(ref string, view *desiredView, state *sqliteState, lock *workspace.WorkspaceSourceLock) int32 {
	if meta := lock.ManagedViews[ref]; meta != nil && meta.ID != 0 {
		if _, ok := state.viewsByID[int32(meta.ID)]; ok {
			return int32(meta.ID)
		}
	}
	matches := state.viewsByName[normalizedName(view.Name)]
	if len(matches) == 1 {
		return matches[0].GetId()
	}
	return 0
}

func resolveConnectorID(ref string, connector *desiredConnector, state *sqliteState, lock *workspace.WorkspaceSourceLock, elementIDs map[string]int32, viewIDs map[string]int32) int32 {
	if meta := lock.ManagedConnectors[ref]; meta != nil && meta.ID != 0 {
		if _, ok := state.connectorsByID[int32(meta.ID)]; ok {
			return int32(meta.ID)
		}
	}
	if id := numericRefID(ref); id != 0 {
		if _, ok := state.connectorsByID[id]; ok {
			return id
		}
	}
	if elementIDs == nil || viewIDs == nil {
		return 0
	}
	key := connectorShape(viewIDs[connector.ViewRef], elementIDs[connector.SourceRef], elementIDs[connector.TargetRef], connector.Label)
	if existing := state.connectorsByShape[key]; existing != nil {
		return existing.GetId()
	}
	return 0
}

func numericRefID(ref string) int32 {
	id, err := strconv.ParseInt(strings.TrimSpace(ref), 10, 32)
	if err != nil || id <= 0 {
		return 0
	}
	return int32(id)
}

func resourceMetadata(id int32, updatedAt *timestamppb.Timestamp) *workspace.ResourceMetadata {
	t := time.Now().UTC()
	if updatedAt != nil && updatedAt.IsValid() {
		t = updatedAt.AsTime()
	}
	return &workspace.ResourceMetadata{ID: workspace.ResourceID(id), UpdatedAt: t}
}

func sortedMetadata(source map[string]*workspace.ResourceMetadata) []struct {
	ref  string
	meta *workspace.ResourceMetadata
} {
	refs := make([]string, 0, len(source))
	for ref := range source {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	out := make([]struct {
		ref  string
		meta *workspace.ResourceMetadata
	}, 0, len(refs))
	for _, ref := range refs {
		out = append(out, struct {
			ref  string
			meta *workspace.ResourceMetadata
		}{ref: ref, meta: source[ref]})
	}
	return out
}

func normalizedName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func stringDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func connectorShape(viewID, sourceID, targetID int32, label string) string {
	return fmt.Sprintf("%d:%d:%d:%s", viewID, sourceID, targetID, normalizedName(label))
}

func sortViews(views []*diagv1.View) {
	sort.Slice(views, func(i, j int) bool { return views[i].GetId() < views[j].GetId() })
}

var refSegmentPattern = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func safePathSegment(ref string) string {
	segment := strings.Trim(refSegmentPattern.ReplaceAllString(ref, "-"), "-.")
	if segment == "" {
		return "view"
	}
	return segment
}

func escapeMermaidLabel(value string) string {
	return mermaidcore.EscapeMermaidLabel(value)
}

func mermaidAlias(ref string, existing map[int32]string) string {
	base := regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(ref, "_")
	if base == "" || !regexp.MustCompile(`^[A-Za-z_]`).MatchString(base) {
		base = "node_" + base
	}
	used := map[string]struct{}{}
	for _, alias := range existing {
		used[alias] = struct{}{}
	}
	alias := base
	for index := 2; ; index++ {
		if _, ok := used[alias]; !ok {
			return alias
		}
		alias = fmt.Sprintf("%s_%d", base, index)
	}
}
