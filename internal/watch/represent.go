package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultEmbeddingBatchSize     = 256
	maxEmbeddingInputApproxTokens = 8000
	maxEmbeddingInputChars        = maxEmbeddingInputApproxTokens * 4
)

var (
	maxEmbeddingSymbolsPerRun = 5000
	maxDetailedSymbolElements = 5000
)

type Representer struct {
	Store *Store
}

func NewRepresenter(store *Store) *Representer {
	return &Representer{Store: store}
}

func (r *Representer) Represent(ctx context.Context, repositoryID int64, req RepresentRequest) (RepresentResult, error) {
	if r == nil || r.Store == nil {
		return RepresentResult{}, fmt.Errorf("watch representer requires a store")
	}
	req.Embedding = normalizeEmbeddingConfig(req.Embedding)
	req.Thresholds = defaultThresholds(req.Thresholds)
	settingsHash := settingsHash(req)
	rawGraphHash, err := r.Store.RawGraphHash(ctx, repositoryID)
	if err != nil {
		return RepresentResult{}, err
	}
	repo, err := r.Store.Repository(ctx, repositoryID)
	if err != nil {
		return RepresentResult{}, err
	}

	provider, err := NewEmbeddingProvider(req.Embedding)
	if err != nil {
		return RepresentResult{}, err
	}
	model := provider.ModelID()
	modelID, err := r.Store.EnsureEmbeddingModel(ctx, EmbeddingConfig{Provider: model.Provider, Model: model.Model, Dimension: model.Dimension}, model.ConfigHash)
	if err != nil {
		return RepresentResult{}, err
	}
	modelIDPtr := &modelID
	if model.Provider == "none" {
		modelIDPtr = nil
	}

	identityKeys, err := r.Store.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		return RepresentResult{}, err
	}
	filtered, err := runFilter(ctx, r.Store, repositoryID, req.Thresholds, rawGraphHash, settingsHash, nil)
	if err != nil {
		return RepresentResult{}, err
	}

	result := RepresentResult{}
	if model.Provider != "none" {
		embeddingSymbols := embeddingCandidateSymbols(filtered.VisibleSymbols, maxEmbeddingSymbolsPerRun)
		stats, vectors, err := r.cacheEmbeddings(ctx, modelID, provider, repo.RepoRoot, embeddingSymbols, identityKeys, req.Progress)
		if err != nil {
			return RepresentResult{}, err
		}
		result.EmbeddingCacheHits = stats.CacheHits
		result.EmbeddingsCreated = stats.Created
		if len(embeddingSymbols) == len(filtered.VisibleSymbols) {
			filtered, err = runFilter(ctx, r.Store, repositoryID, req.Thresholds, rawGraphHash, settingsHash, vectors)
			if err != nil {
				return RepresentResult{}, err
			}
		}
	}

	representationHash := representationHash(filtered, req)
	result = RepresentResult{
		RepositoryID:       repositoryID,
		FilterRunID:        filtered.RunID,
		RawGraphHash:       rawGraphHash,
		SettingsHash:       settingsHash,
		RepresentationHash: representationHash,
		EmbeddingCacheHits: result.EmbeddingCacheHits,
		EmbeddingsCreated:  result.EmbeddingsCreated,
	}
	runID, err := r.Store.BeginRepresentationRun(ctx, repositoryID, rawGraphHash, settingsHash, modelIDPtr, representationHash)
	if err != nil {
		return RepresentResult{}, err
	}
	result.RepresentationRun = runID
	status := "completed"
	var runErr error
	defer func() {
		if runErr != nil {
			status = "failed"
		}
		_ = r.Store.FinishRepresentationRun(context.Background(), runID, status, result, runErr)
	}()

	stats, err := r.materialize(ctx, repo, filtered, req.Thresholds, settingsHash, identityKeys)
	if err != nil {
		runErr = err
		return result, err
	}
	result.ElementsCreated = stats.ElementsCreated
	result.ElementsUpdated = stats.ElementsUpdated
	result.ConnectorsCreated = stats.ConnectorsCreated
	result.ConnectorsUpdated = stats.ConnectorsUpdated
	result.ViewsCreated = stats.ViewsCreated
	return result, nil
}

type embeddingCacheStats struct {
	CacheHits int
	Created   int
}

func progressStart(progress ProgressSink, label string, total int) {
	if progress != nil {
		progress.Start(label, total)
	}
}

func progressAdvance(progress ProgressSink, label string) {
	if progress != nil {
		progress.Advance(label)
	}
}

func progressFinish(progress ProgressSink) {
	if progress != nil {
		progress.Finish()
	}
}

func (r *Representer) cacheEmbeddings(ctx context.Context, modelID int64, provider Provider, repoRoot string, symbols []Symbol, identityKeys map[string]string, progress ProgressSink) (embeddingCacheStats, map[int64]Vector, error) {
	stats := embeddingCacheStats{}
	vectorsBySymbol := map[int64]Vector{}
	model := provider.ModelID()
	if model.Provider == "none" {
		return stats, vectorsBySymbol, nil
	}
	inputs := make([]EmbeddingInput, 0, len(symbols))
	missingSymbols := make([]Symbol, 0, len(symbols))
	progressStart(progress, "Preparing symbol embeddings", len(symbols))
	for _, sym := range symbols {
		ownerKey := symbolOwnerKey(sym, identityKeys)
		input := EmbeddingInput{OwnerType: "symbol", OwnerKey: ownerKey, Text: symbolEmbeddingText(repoRoot, sym)}
		if data, ok, err := r.Store.Embedding(ctx, modelID, input.OwnerType, input.OwnerKey, inputHash(input)); err != nil {
			progressFinish(progress)
			return stats, vectorsBySymbol, err
		} else if !ok {
			inputs = append(inputs, input)
			missingSymbols = append(missingSymbols, sym)
		} else {
			stats.CacheHits++
			vectorsBySymbol[sym.ID] = bytesToVector(data)
		}
		progressAdvance(progress, sym.QualifiedName)
	}
	progressFinish(progress)
	if len(inputs) == 0 {
		return stats, vectorsBySymbol, nil
	}

	vectors := make([]Vector, 0, len(inputs))
	progressStart(progress, "Embedding symbols", len(inputs))
	for start := 0; start < len(inputs); start += defaultEmbeddingBatchSize {
		end := start + defaultEmbeddingBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		chunk := inputs[start:end]
		chunkVectors, err := provider.Embed(ctx, chunk)
		if err != nil {
			progressFinish(progress)
			return stats, vectorsBySymbol, err
		}
		if len(chunkVectors) != len(chunk) {
			progressFinish(progress)
			return stats, vectorsBySymbol, fmt.Errorf("embedding provider returned %d vectors for %d inputs", len(chunkVectors), len(chunk))
		}
		vectors = append(vectors, chunkVectors...)
		for _, input := range chunk {
			progressAdvance(progress, input.OwnerKey)
		}
	}
	for i, input := range inputs {
		if err := r.Store.SaveEmbedding(ctx, modelID, input.OwnerType, input.OwnerKey, inputHash(input), vectorBytes(vectors[i])); err != nil {
			progressFinish(progress)
			return stats, vectorsBySymbol, err
		}
		stats.Created++
		vectorsBySymbol[missingSymbols[i].ID] = vectors[i]
	}
	progressFinish(progress)
	return stats, vectorsBySymbol, nil
}

func embeddingCandidateSymbols(symbols map[int64]Symbol, limit int) []Symbol {
	out := sortedSymbols(symbols)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func symbolEmbeddingText(repoRoot string, sym Symbol) string {
	body := symbolCodeBody(repoRoot, sym)
	if strings.TrimSpace(body) == "" {
		body = sym.QualifiedName + "\n" + sym.Kind + "\n" + sym.FilePath
	}
	return shrinkEmbeddingText(outdentCode(body))
}

func symbolCodeBody(repoRoot string, sym Symbol) string {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(sym.FilePath) == "" {
		return ""
	}
	cleanRel := filepath.Clean(filepath.FromSlash(sym.FilePath))
	if filepath.IsAbs(cleanRel) || cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, cleanRel))
	if err != nil {
		return ""
	}
	end := sym.StartLine
	if sym.EndLine != nil {
		end = *sym.EndLine
	}
	return lineRange(strings.Split(string(data), "\n"), sym.StartLine, end)
}

func outdentCode(code string) string {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	code = strings.ReplaceAll(code, "\r", "\n")
	lines := strings.Split(code, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingIndentWidth(line)
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return strings.TrimSpace(code)
	}
	for i, line := range lines {
		lines[i] = trimIndentWidth(line, minIndent)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func leadingIndentWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

func trimIndentWidth(line string, maxWidth int) string {
	width := 0
	for i, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return line[i:]
		}
		if width >= maxWidth {
			return line[i+len(string(r)):]
		}
	}
	return ""
}

func shrinkEmbeddingText(text string) string {
	text = strings.TrimSpace(text)
	if approximateTokenCount(text) <= maxEmbeddingInputApproxTokens {
		return text
	}
	text = dropLowSignalCodeLines(text)
	if approximateTokenCount(text) <= maxEmbeddingInputApproxTokens {
		return text
	}
	if len(text) <= maxEmbeddingInputChars {
		return text
	}
	marker := "\n\n/* ... middle omitted for embedding context ... */\n\n"
	keep := maxEmbeddingInputChars - len(marker)
	if keep <= 0 {
		return text[:maxEmbeddingInputChars]
	}
	head := keep * 2 / 3
	tail := keep - head
	return strings.TrimSpace(text[:head]) + marker + strings.TrimSpace(text[len(text)-tail:])
}

func approximateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	fields := strings.Fields(text)
	byChars := (len(text) + 3) / 4
	if byChars > len(fields) {
		return byChars
	}
	return len(fields)
}

func dropLowSignalCodeLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return text
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

type materializeStats struct {
	ElementsCreated   int
	ElementsUpdated   int
	ConnectorsCreated int
	ConnectorsUpdated int
	ViewsCreated      int
}

func (r *Representer) materialize(ctx context.Context, repo Repository, filtered filterResult, thresholds Thresholds, settingsHash string, identityKeys map[string]string) (materializeStats, error) {
	initialLayout, err := r.Store.RepositoryMaterializationCount(ctx, repo.ID)
	if err != nil {
		return materializeStats{}, err
	}
	m := &materializer{store: r.Store, repo: repo, thresholds: thresholds, settingsHash: settingsHash, identityKeys: identityKeys, initialLayout: initialLayout == 0, runMarker: time.Now().UTC().Format(time.RFC3339Nano), newPlacements: map[int64]map[int64]struct{}{}}
	if err := m.ensureTags(ctx); err != nil {
		return m.stats, err
	}
	rootViewID, err := m.workspaceRootViewID(ctx)
	if err != nil {
		return m.stats, err
	}
	repoLanguage := dominantLanguage(filtered.VisibleSymbols)
	repoElem, err := m.upsertElement(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), elementInput{
		Name:       repo.DisplayName,
		Kind:       "repository",
		Technology: technologyLabel(repoLanguage),
		Repo:       repoIdentity(repo),
		Branch:     nullStringValue(repo.Branch),
		Language:   repoLanguage,
	})
	if err != nil {
		return m.stats, err
	}
	if err := m.upsertPlacement(ctx, rootViewID, repoElem, 0, 0); err != nil {
		return m.stats, err
	}
	repoView, err := m.upsertView(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), repoElem, repo.DisplayName, "Repository")
	if err != nil {
		return m.stats, err
	}

	visibleFiles := filesForSymbols(filtered.VisibleSymbols)
	folders := folderSet(visibleFiles)
	folderViews := map[string]int64{}
	for _, folder := range folders {
		parentView := repoView
		if parent := path.Dir(folder); parent != "." && parent != "/" {
			if id, ok := folderViews[parent]; ok {
				parentView = id
			}
		}
		elem, err := m.upsertElement(ctx, "folder", "folder:"+folder, elementInput{
			Name:       path.Base(folder),
			Kind:       "folder",
			Technology: technologyLabel(repoLanguage),
			Repo:       repoIdentity(repo),
			Branch:     nullStringValue(repo.Branch),
			FilePath:   folder,
			Language:   repoLanguage,
		})
		if err != nil {
			return m.stats, err
		}
		x, y := gridPosition(len(folderViews))
		if err := m.upsertPlacement(ctx, parentView, elem, x, y); err != nil {
			return m.stats, err
		}
		view, err := m.upsertView(ctx, "folder", "folder:"+folder, elem, folder, "Folder")
		if err != nil {
			return m.stats, err
		}
		folderViews[folder] = view
	}

	fileElements := map[string]int64{}
	fileViews := map[string]int64{}
	for i, file := range sortedKeys(visibleFiles) {
		fileLanguage := languageForFile(file, filtered.VisibleSymbols)
		parentView := repoView
		if dir := path.Dir(file); dir != "." {
			if id, ok := folderViews[dir]; ok {
				parentView = id
			}
		}
		elem, err := m.upsertElement(ctx, "file", "file:"+file, elementInput{
			Name:       path.Base(file),
			Kind:       "file",
			Technology: technologyLabel(fileLanguage),
			Repo:       repoIdentity(repo),
			Branch:     nullStringValue(repo.Branch),
			FilePath:   file,
			Language:   fileLanguage,
		})
		if err != nil {
			return m.stats, err
		}
		x, y := gridPosition(i)
		if err := m.upsertPlacement(ctx, parentView, elem, x, y); err != nil {
			return m.stats, err
		}
		view, err := m.upsertView(ctx, "file", "file:"+file, elem, file, "File")
		if err != nil {
			return m.stats, err
		}
		fileElements[file] = elem
		fileViews[file] = view
	}

	symbolElements := map[int64]int64{}
	symbolViews := map[int64]int64{}
	detailedSymbols := len(filtered.VisibleSymbols) <= maxDetailedSymbolElements
	for file, symbols := range symbolsByFile(filtered.VisibleSymbols) {
		fileView := fileViews[file]
		if fileView == 0 {
			continue
		}
		chunks := chunkSymbols(symbols, thresholds.MaxElementsPerView)
		for chunkIndex, chunk := range chunks {
			targetView := fileView
			if len(chunks) > 1 {
				keys := make([]string, 0, len(chunk))
				ids := make([]int64, 0, len(chunk))
				for _, sym := range chunk {
					keys = append(keys, sym.StableKey)
					ids = append(ids, sym.ID)
				}
				clusterKey := stableClusterKey(repo.ID, file, settingsHash, keys)
				cluster, err := m.store.UpsertCluster(ctx, repo.ID, clusterKey, nil, fmt.Sprintf("%s cluster %d", path.Base(file), chunkIndex+1), "structural", "deterministic-chunk", settingsHash, ids)
				if err != nil {
					return m.stats, err
				}
				clusterElem, err := m.upsertElement(ctx, "cluster", clusterKey, elementInput{
					Name:       cluster.Name,
					Kind:       "cluster",
					Technology: technologyLabel(languageFromStableKey(chunk[0].StableKey)),
					Repo:       repoIdentity(repo),
					Branch:     nullStringValue(repo.Branch),
					FilePath:   file,
					Language:   languageFromStableKey(chunk[0].StableKey),
				})
				if err != nil {
					return m.stats, err
				}
				x, y := gridPosition(chunkIndex)
				if err := m.upsertPlacement(ctx, fileView, clusterElem, x, y); err != nil {
					return m.stats, err
				}
				targetView, err = m.upsertView(ctx, "cluster", clusterKey, clusterElem, cluster.Name, "Cluster")
				if err != nil {
					return m.stats, err
				}
			}
			if !detailedSymbols {
				continue
			}
				for i, sym := range chunk {
					language := languageFromStableKey(sym.StableKey)
					elem, err := m.upsertElement(ctx, "symbol", symbolOwnerKey(sym, m.identityKeys), elementInput{
						Name:        sym.QualifiedName,
						Kind:        sym.Kind,
						Description: fmt.Sprintf("%s:%d", sym.FilePath, sym.StartLine),
						Technology:  technologyLabel(language),
						Repo:        repoIdentity(repo),
						Branch:      nullStringValue(repo.Branch),
						FilePath:    sym.FilePath,
						Language:    language,
					})
				if err != nil {
					return m.stats, err
				}
				x, y := gridPosition(i)
				if err := m.upsertPlacement(ctx, targetView, elem, x, y); err != nil {
					return m.stats, err
				}
				symbolElements[sym.ID] = elem
				symbolViews[sym.ID] = targetView
			}
		}
	}

	if err := m.materializeConnectors(ctx, filtered.VisibleReferences, filtered.VisibleSymbols, fileElements, symbolElements, symbolViews, repoView); err != nil {
		return m.stats, err
	}
	if err := m.pruneStaleResources(ctx); err != nil {
		return m.stats, err
	}
	if err := m.layoutPlacements(ctx); err != nil {
		return m.stats, err
	}
	return m.stats, nil
}

type materializer struct {
	store         *Store
	repo          Repository
	thresholds    Thresholds
	settingsHash  string
	identityKeys  map[string]string
	initialLayout bool
	runMarker     string
	newPlacements map[int64]map[int64]struct{}
	stats         materializeStats
}

type elementInput struct {
	Name        string
	Kind        string
	Description string
	Technology  string
	Repo        string
	Branch      string
	FilePath    string
	Language    string
}

func (m *materializer) ensureTags(ctx context.Context) error {
	for _, tag := range []string{"tld:watch", "watch:generated", "watch:go"} {
		if _, err := m.store.db.ExecContext(ctx, `
			INSERT INTO tags(name, color, description) VALUES (?, '#64748b', 'Generated by tld watch')
			ON CONFLICT(name) DO NOTHING`, tag); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) workspaceRootViewID(ctx context.Context) (int64, error) {
	var id int64
	err := m.store.db.QueryRowContext(ctx, `SELECT id FROM views WHERE owner_element_id IS NULL ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

func (m *materializer) upsertElement(ctx context.Context, ownerType, ownerKey string, input elementInput) (int64, error) {
	if id, ok, err := m.store.MappingResourceID(ctx, m.repo.ID, ownerType, ownerKey, "element"); err != nil {
		return 0, err
	} else if ok && elementExists(ctx, m.store.db, id) {
		tags, _ := json.Marshal([]string{"tld:watch", "watch:generated", "watch:go"})
		techLinks, _ := json.Marshal([]map[string]string{{"type": "technology", "slug": "go", "label": "Go"}})
		_, err := m.store.db.ExecContext(ctx, `
			UPDATE elements
			SET name = ?, kind = ?, description = ?, technology = ?, technology_connectors = ?, tags = ?, repo = ?, branch = ?, file_path = ?, language = ?, updated_at = ?
			WHERE id = ?`,
			input.Name, nullString(input.Kind), nullString(input.Description), nullString(input.Technology), string(techLinks), string(tags),
			nullString(input.Repo), nullString(input.Branch), nullString(input.FilePath), nullString(input.Language), nowString(), id)
		if err != nil {
			return 0, err
		}
		if err := m.saveMapping(ctx, ownerType, ownerKey, "element", id); err != nil {
			return 0, err
		}
		m.stats.ElementsUpdated++
		return id, nil
	}
	now := nowString()
	tags, _ := json.Marshal([]string{"tld:watch", "watch:generated", "watch:go"})
	techLinks, _ := json.Marshal([]map[string]string{{"type": "technology", "slug": "go", "label": "Go"}})
	res, err := m.store.db.ExecContext(ctx, `
		INSERT INTO elements(name, kind, description, technology, technology_connectors, tags, repo, branch, file_path, language, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Name, nullString(input.Kind), nullString(input.Description), nullString(input.Technology), string(techLinks), string(tags),
		nullString(input.Repo), nullString(input.Branch), nullString(input.FilePath), nullString(input.Language), now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := m.saveMapping(ctx, ownerType, ownerKey, "element", id); err != nil {
		return 0, err
	}
	m.stats.ElementsCreated++
	return id, nil
}

func (m *materializer) upsertView(ctx context.Context, ownerType, ownerKey string, ownerElementID int64, name, label string) (int64, error) {
	if id, ok, err := m.store.MappingResourceID(ctx, m.repo.ID, ownerType, ownerKey, "view"); err != nil {
		return 0, err
	} else if ok && viewExists(ctx, m.store.db, id) {
		if _, err := m.store.db.ExecContext(ctx, `UPDATE views SET owner_element_id = ?, name = ?, level_label = ?, updated_at = ? WHERE id = ?`, ownerElementID, name, label, nowString(), id); err != nil {
			return 0, err
		}
		return id, m.saveMapping(ctx, ownerType, ownerKey, "view", id)
	}
	now := nowString()
	res, err := m.store.db.ExecContext(ctx, `INSERT INTO views(owner_element_id, name, level_label, level, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`, ownerElementID, name, label, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := m.saveMapping(ctx, ownerType, ownerKey, "view", id); err != nil {
		return 0, err
	}
	m.stats.ViewsCreated++
	return id, nil
}

func (m *materializer) upsertPlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	var existingID int64
	err := m.store.db.QueryRowContext(ctx, `SELECT id FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := nowString()
	_, err = m.store.db.ExecContext(ctx, `
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		viewID, elementID, x, y, now, now)
	if err == nil {
		m.markNewPlacement(viewID, elementID)
	}
	return err
}

func (m *materializer) markNewPlacement(viewID, elementID int64) {
	if m.newPlacements == nil {
		m.newPlacements = map[int64]map[int64]struct{}{}
	}
	if m.newPlacements[viewID] == nil {
		m.newPlacements[viewID] = map[int64]struct{}{}
	}
	m.newPlacements[viewID][elementID] = struct{}{}
}

const (
	watchLayoutNodeWidth  = 140.0
	watchLayoutNodeHeight = 80.0
	watchLayoutGapX       = 260.0
	watchLayoutGapY       = 170.0
)

type watchPlacementNode struct {
	ElementID int64
	X         float64
	Y         float64
}

type watchLayoutConnector struct {
	Source int64
	Target int64
}

func (m *materializer) layoutPlacements(ctx context.Context) error {
	targets := m.newPlacements
	if m.initialLayout {
		var err error
		targets, err = m.generatedPlacementsByView(ctx)
		if err != nil {
			return err
		}
	}
	for viewID, elementIDs := range targets {
		if len(elementIDs) == 0 {
			continue
		}
		if err := m.layoutView(ctx, viewID, elementIDs, m.initialLayout); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) generatedPlacementsByView(ctx context.Context) (map[int64]map[int64]struct{}, error) {
	rows, err := m.store.db.QueryContext(ctx, `
		SELECT p.view_id, p.element_id
		FROM placements p
		JOIN watch_materialization wm
		  ON wm.repository_id = ? AND wm.resource_type = 'element' AND wm.resource_id = p.element_id
		ORDER BY p.view_id, p.id`, m.repo.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]map[int64]struct{}{}
	for rows.Next() {
		var viewID, elementID int64
		if err := rows.Scan(&viewID, &elementID); err != nil {
			return nil, err
		}
		if out[viewID] == nil {
			out[viewID] = map[int64]struct{}{}
		}
		out[viewID][elementID] = struct{}{}
	}
	return out, rows.Err()
}

func (m *materializer) layoutView(ctx context.Context, viewID int64, targets map[int64]struct{}, force bool) error {
	placements, err := m.viewPlacementNodes(ctx, viewID)
	if err != nil {
		return err
	}
	connectors, err := m.viewLayoutConnectors(ctx, viewID)
	if err != nil {
		return err
	}
	if force {
		next := layeredWatchLayout(targets, connectors)
		occupied := occupiedWatchCells(placements, targets)
		for _, elementID := range sortedInt64Set(targets) {
			desired := next[elementID]
			x, y := nearestFreeWatchCell(desired.X, desired.Y, occupied)
			occupied[watchCellKey(x, y)] = struct{}{}
			if _, err := m.store.db.ExecContext(ctx, `UPDATE placements SET position_x = ?, position_y = ?, updated_at = ? WHERE view_id = ? AND element_id = ?`, x, y, nowString(), viewID, elementID); err != nil {
				return err
			}
		}
		return nil
	}

	positioned := map[int64]watchPlacementNode{}
	for _, p := range placements {
		if _, isNew := targets[p.ElementID]; !isNew {
			positioned[p.ElementID] = p
		}
	}
	occupied := occupiedWatchCells(placements, targets)
	for _, elementID := range sortedInt64Set(targets) {
		x, y := bestIncrementalWatchPosition(elementID, positioned, occupied, connectors)
		occupied[watchCellKey(x, y)] = struct{}{}
		positioned[elementID] = watchPlacementNode{ElementID: elementID, X: x, Y: y}
		if _, err := m.store.db.ExecContext(ctx, `UPDATE placements SET position_x = ?, position_y = ?, updated_at = ? WHERE view_id = ? AND element_id = ?`, x, y, nowString(), viewID, elementID); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) viewPlacementNodes(ctx context.Context, viewID int64) ([]watchPlacementNode, error) {
	rows, err := m.store.db.QueryContext(ctx, `SELECT element_id, position_x, position_y FROM placements WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []watchPlacementNode
	for rows.Next() {
		var p watchPlacementNode
		if err := rows.Scan(&p.ElementID, &p.X, &p.Y); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *materializer) viewLayoutConnectors(ctx context.Context, viewID int64) ([]watchLayoutConnector, error) {
	rows, err := m.store.db.QueryContext(ctx, `SELECT source_element_id, target_element_id FROM connectors WHERE view_id = ? ORDER BY id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []watchLayoutConnector
	for rows.Next() {
		var c watchLayoutConnector
		if err := rows.Scan(&c.Source, &c.Target); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func layeredWatchLayout(targets map[int64]struct{}, connectors []watchLayoutConnector) map[int64]watchPlacementNode {
	level := map[int64]int{}
	for id := range targets {
		level[id] = 0
	}
	for i := 0; i < len(targets); i++ {
		changed := false
		for _, c := range connectors {
			if _, ok := targets[c.Source]; !ok {
				continue
			}
			if _, ok := targets[c.Target]; !ok {
				continue
			}
			if level[c.Target] <= level[c.Source] {
				level[c.Target] = level[c.Source] + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	columns := map[int][]int64{}
	for id, col := range level {
		if col >= len(targets) {
			col = 0
		}
		columns[col] = append(columns[col], id)
	}
	out := map[int64]watchPlacementNode{}
	for _, col := range sortedIntKeys(columns) {
		ids := columns[col]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for row, id := range ids {
			out[id] = watchPlacementNode{ElementID: id, X: float64(col) * watchLayoutGapX, Y: float64(row) * watchLayoutGapY}
		}
	}
	return out
}

func bestIncrementalWatchPosition(elementID int64, positioned map[int64]watchPlacementNode, occupied map[string]struct{}, connectors []watchLayoutConnector) (float64, float64) {
	candidates := watchLayoutCandidates(positioned)
	bestX, bestY := 0.0, 0.0
	bestScore := math.Inf(1)
	for _, candidate := range candidates {
		if _, blocked := occupied[watchCellKey(candidate.X, candidate.Y)]; blocked {
			continue
		}
		score := incrementalWatchScore(elementID, candidate, positioned, connectors)
		if score < bestScore {
			bestScore = score
			bestX, bestY = candidate.X, candidate.Y
		}
	}
	if math.IsInf(bestScore, 1) {
		return nearestFreeWatchCell(0, 0, occupied)
	}
	return bestX, bestY
}

func incrementalWatchScore(elementID int64, candidate watchPlacementNode, positioned map[int64]watchPlacementNode, connectors []watchLayoutConnector) float64 {
	score := math.Abs(candidate.X)*0.01 + math.Abs(candidate.Y)*0.01
	candidateEdges := [][2]watchPlacementNode{}
	existingEdges := [][2]watchPlacementNode{}
	for _, c := range connectors {
		source, sourceOK := positioned[c.Source]
		target, targetOK := positioned[c.Target]
		if c.Source == elementID {
			source, sourceOK = candidate, true
		}
		if c.Target == elementID {
			target, targetOK = candidate, true
		}
		if sourceOK && targetOK {
			edge := [2]watchPlacementNode{source, target}
			if c.Source == elementID || c.Target == elementID {
				candidateEdges = append(candidateEdges, edge)
				score += watchDistance(source, target)
			} else {
				existingEdges = append(existingEdges, edge)
			}
		}
	}
	if len(candidateEdges) == 0 {
		return score + nearestWatchNeighborDistance(candidate, positioned)
	}
	for _, candidateEdge := range candidateEdges {
		for _, existingEdge := range existingEdges {
			if candidateEdge[0].ElementID == existingEdge[0].ElementID || candidateEdge[0].ElementID == existingEdge[1].ElementID ||
				candidateEdge[1].ElementID == existingEdge[0].ElementID || candidateEdge[1].ElementID == existingEdge[1].ElementID {
				continue
			}
			if watchSegmentsIntersect(candidateEdge[0], candidateEdge[1], existingEdge[0], existingEdge[1]) {
				score += 10000
			}
		}
	}
	return score
}

func watchLayoutCandidates(positioned map[int64]watchPlacementNode) []watchPlacementNode {
	minCol, maxCol, minRow, maxRow := 0, 4, 0, 3
	if len(positioned) > 0 {
		minCol, maxCol, minRow, maxRow = math.MaxInt, math.MinInt, math.MaxInt, math.MinInt
		for _, p := range positioned {
			col := int(math.Round(p.X / watchLayoutGapX))
			row := int(math.Round(p.Y / watchLayoutGapY))
			if col < minCol {
				minCol = col
			}
			if col > maxCol {
				maxCol = col
			}
			if row < minRow {
				minRow = row
			}
			if row > maxRow {
				maxRow = row
			}
		}
		minCol--
		maxCol += 2
		minRow--
		maxRow += 2
	}
	out := make([]watchPlacementNode, 0, (maxCol-minCol+1)*(maxRow-minRow+1))
	for col := minCol; col <= maxCol; col++ {
		for row := minRow; row <= maxRow; row++ {
			out = append(out, watchPlacementNode{X: float64(col) * watchLayoutGapX, Y: float64(row) * watchLayoutGapY})
		}
	}
	return out
}

func occupiedWatchCells(placements []watchPlacementNode, ignored map[int64]struct{}) map[string]struct{} {
	occupied := map[string]struct{}{}
	for _, p := range placements {
		if _, ok := ignored[p.ElementID]; ok {
			continue
		}
		occupied[watchCellKey(p.X, p.Y)] = struct{}{}
	}
	return occupied
}

func nearestFreeWatchCell(x, y float64, occupied map[string]struct{}) (float64, float64) {
	baseCol := int(math.Round(x / watchLayoutGapX))
	baseRow := int(math.Round(y / watchLayoutGapY))
	for radius := 0; radius < 200; radius++ {
		for col := baseCol - radius; col <= baseCol+radius; col++ {
			for row := baseRow - radius; row <= baseRow+radius; row++ {
				if watchAbsInt(col-baseCol) != radius && watchAbsInt(row-baseRow) != radius {
					continue
				}
				nx, ny := float64(col)*watchLayoutGapX, float64(row)*watchLayoutGapY
				if _, ok := occupied[watchCellKey(nx, ny)]; !ok {
					return nx, ny
				}
			}
		}
	}
	return x, y
}

func watchCellKey(x, y float64) string {
	return fmt.Sprintf("%d:%d", int(math.Round(x/watchLayoutGapX)), int(math.Round(y/watchLayoutGapY)))
}

func watchDistance(a, b watchPlacementNode) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func nearestWatchNeighborDistance(candidate watchPlacementNode, positioned map[int64]watchPlacementNode) float64 {
	if len(positioned) == 0 {
		return 0
	}
	best := math.Inf(1)
	for _, p := range positioned {
		if d := watchDistance(candidate, p); d < best {
			best = d
		}
	}
	return best
}

func watchCenter(p watchPlacementNode) (float64, float64) {
	return p.X + watchLayoutNodeWidth/2, p.Y + watchLayoutNodeHeight/2
}

func watchSegmentsIntersect(a, b, c, d watchPlacementNode) bool {
	ax, ay := watchCenter(a)
	bx, by := watchCenter(b)
	cx, cy := watchCenter(c)
	dx, dy := watchCenter(d)
	return segmentOrientation(ax, ay, cx, cy, dx, dy) != segmentOrientation(bx, by, cx, cy, dx, dy) &&
		segmentOrientation(ax, ay, bx, by, cx, cy) != segmentOrientation(ax, ay, bx, by, dx, dy)
}

func segmentOrientation(ax, ay, bx, by, cx, cy float64) int {
	value := (by-ay)*(cx-bx) - (bx-ax)*(cy-by)
	if math.Abs(value) < 0.000001 {
		return 0
	}
	if value > 0 {
		return 1
	}
	return -1
}

func sortedInt64Set(values map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedIntKeys(values map[int][]int64) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func watchAbsInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (m *materializer) materializeConnectors(ctx context.Context, refs []Reference, symbols map[int64]Symbol, fileElements map[string]int64, symbolElements map[int64]int64, symbolViews map[int64]int64, repoView int64) error {
	filePairs := map[string]Reference{}
	symbolConnectorCount := map[int64]int{}
	for _, ref := range refs {
		source := symbols[ref.SourceSymbolID]
		target := symbols[ref.TargetSymbolID]
		if source.FilePath != "" && target.FilePath != "" && source.FilePath != target.FilePath {
			key := source.FilePath + "->" + target.FilePath
			if _, ok := filePairs[key]; !ok {
				filePairs[key] = ref
			}
			continue
		}
		viewID := symbolViews[ref.SourceSymbolID]
		if viewID == 0 || viewID != symbolViews[ref.TargetSymbolID] || symbolConnectorCount[viewID] >= m.thresholds.MaxConnectorsPerView {
			continue
		}
		sourceKey := symbolOwnerKey(source, m.identityKeys)
		targetKey := symbolOwnerKey(target, m.identityKeys)
		if err := m.upsertConnector(ctx, "reference", fmt.Sprintf("symbol:%s:%s:%s", sourceKey, targetKey, ref.Kind), viewID, symbolElements[ref.SourceSymbolID], symbolElements[ref.TargetSymbolID], "calls"); err != nil {
			return err
		}
		symbolConnectorCount[viewID]++
	}
	fileConnectorCount := 0
	for _, key := range sortedKeys(filePairs) {
		if fileConnectorCount >= m.thresholds.MaxConnectorsPerView {
			break
		}
		ref := filePairs[key]
		source := symbols[ref.SourceSymbolID]
		target := symbols[ref.TargetSymbolID]
		if fileElements[source.FilePath] == 0 || fileElements[target.FilePath] == 0 {
			continue
		}
		if err := m.upsertConnector(ctx, "file-reference", "file:"+key, repoView, fileElements[source.FilePath], fileElements[target.FilePath], "references"); err != nil {
			return err
		}
		fileConnectorCount++
	}
	return nil
}

func (m *materializer) upsertConnector(ctx context.Context, ownerType, ownerKey string, viewID, sourceElementID, targetElementID int64, label string) error {
	if sourceElementID == 0 || targetElementID == 0 || sourceElementID == targetElementID {
		return nil
	}
	if id, ok, err := m.store.MappingResourceID(ctx, m.repo.ID, ownerType, ownerKey, "connector"); err != nil {
		return err
	} else if ok && connectorExists(ctx, m.store.db, id) {
		_, err := m.store.db.ExecContext(ctx, `
			UPDATE connectors
			SET view_id = ?, source_element_id = ?, target_element_id = ?, label = ?, relationship = ?, direction = 'forward', style = 'solid', updated_at = ?
			WHERE id = ?`, viewID, sourceElementID, targetElementID, label, label, nowString(), id)
		if err != nil {
			return err
		}
		if err := m.saveMapping(ctx, ownerType, ownerKey, "connector", id); err != nil {
			return err
		}
		m.stats.ConnectorsUpdated++
		return nil
	}
	now := nowString()
	res, err := m.store.db.ExecContext(ctx, `
		INSERT INTO connectors(view_id, source_element_id, target_element_id, label, relationship, direction, style, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'forward', 'solid', ?, ?)`, viewID, sourceElementID, targetElementID, label, label, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := m.saveMapping(ctx, ownerType, ownerKey, "connector", id); err != nil {
		return err
	}
	m.stats.ConnectorsCreated++
	return nil
}

func (m *materializer) saveMapping(ctx context.Context, ownerType, ownerKey, resourceType string, resourceID int64) error {
	return m.store.SaveMappingAt(ctx, m.repo.ID, ownerType, ownerKey, resourceType, resourceID, m.runMarker)
}

func (m *materializer) pruneStaleResources(ctx context.Context) error {
	if m.runMarker == "" {
		return nil
	}
	for _, item := range []struct {
		resourceType string
		tableName    string
	}{
		{resourceType: "connector", tableName: "connectors"},
		{resourceType: "view", tableName: "views"},
		{resourceType: "element", tableName: "elements"},
	} {
		query := fmt.Sprintf(`
			DELETE FROM %s
			WHERE id IN (
				SELECT resource_id
				FROM watch_materialization
				WHERE repository_id = ? AND resource_type = ? AND updated_at != ?
			)`, item.tableName)
		if _, err := m.store.db.ExecContext(ctx, query, m.repo.ID, item.resourceType, m.runMarker); err != nil {
			return err
		}
	}
	if _, err := m.store.db.ExecContext(ctx, `
		DELETE FROM watch_materialization
		WHERE repository_id = ? AND updated_at != ?`, m.repo.ID, m.runMarker); err != nil {
		return err
	}
	return nil
}

func elementExists(ctx context.Context, db *sql.DB, id int64) bool {
	return rowExists(ctx, db, `SELECT 1 FROM elements WHERE id = ?`, id)
}

func viewExists(ctx context.Context, db *sql.DB, id int64) bool {
	return rowExists(ctx, db, `SELECT 1 FROM views WHERE id = ?`, id)
}

func connectorExists(ctx context.Context, db *sql.DB, id int64) bool {
	return rowExists(ctx, db, `SELECT 1 FROM connectors WHERE id = ?`, id)
}

func rowExists(ctx context.Context, db *sql.DB, query string, id int64) bool {
	var one int
	err := db.QueryRowContext(ctx, query, id).Scan(&one)
	return err == nil
}

func filesForSymbols(symbols map[int64]Symbol) map[string]struct{} {
	out := map[string]struct{}{}
	for _, sym := range symbols {
		if sym.FilePath != "" {
			out[sym.FilePath] = struct{}{}
		}
	}
	return out
}

func symbolOwnerKey(sym Symbol, identityKeys map[string]string) string {
	if identityKeys != nil {
		if key := strings.TrimSpace(identityKeys[sym.StableKey]); key != "" {
			return key
		}
	}
	return sym.StableKey
}

func folderSet(files map[string]struct{}) []string {
	set := map[string]struct{}{}
	for file := range files {
		dir := path.Dir(file)
		for dir != "." && dir != "/" {
			set[dir] = struct{}{}
			next := path.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	out := sortedKeys(set)
	sort.SliceStable(out, func(i, j int) bool {
		di := strings.Count(out[i], "/")
		dj := strings.Count(out[j], "/")
		if di == dj {
			return out[i] < out[j]
		}
		return di < dj
	})
	return out
}

func dominantLanguage(symbols map[int64]Symbol) string {
	counts := map[string]int{}
	for _, sym := range symbols {
		language := languageFromStableKey(sym.StableKey)
		if language != "" {
			counts[language]++
		}
	}
	best := "source"
	bestCount := 0
	for language, count := range counts {
		if count > bestCount || (count == bestCount && language < best) {
			best = language
			bestCount = count
		}
	}
	return best
}

func languageForFile(file string, symbols map[int64]Symbol) string {
	counts := map[string]int{}
	for _, sym := range symbols {
		if sym.FilePath != file {
			continue
		}
		language := languageFromStableKey(sym.StableKey)
		if language != "" {
			counts[language]++
		}
	}
	best := dominantLanguage(symbols)
	bestCount := 0
	for language, count := range counts {
		if count > bestCount || (count == bestCount && language < best) {
			best = language
			bestCount = count
		}
	}
	return best
}

func languageFromStableKey(stableKey string) string {
	if idx := strings.Index(stableKey, ":"); idx > 0 {
		return stableKey[:idx]
	}
	return "source"
}

func technologyLabel(language string) string {
	switch language {
	case "go":
		return "Go"
	case "typescript":
		return "TypeScript"
	case "javascript":
		return "JavaScript"
	case "python":
		return "Python"
	case "java":
		return "Java"
	case "cpp":
		return "C++"
	case "c":
		return "C"
	default:
		return "Source"
	}
}

func symbolsByFile(symbols map[int64]Symbol) map[string][]Symbol {
	out := map[string][]Symbol{}
	for _, sym := range sortedSymbols(symbols) {
		out[sym.FilePath] = append(out[sym.FilePath], sym)
	}
	return out
}

func sortedSymbols(symbols map[int64]Symbol) []Symbol {
	out := make([]Symbol, 0, len(symbols))
	for _, sym := range symbols {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FilePath == out[j].FilePath {
			if out[i].StartLine == out[j].StartLine {
				return out[i].StableKey < out[j].StableKey
			}
			return out[i].StartLine < out[j].StartLine
		}
		return out[i].FilePath < out[j].FilePath
	})
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func chunkSymbols(symbols []Symbol, size int) [][]Symbol {
	if size <= 0 || len(symbols) <= size {
		return [][]Symbol{symbols}
	}
	var chunks [][]Symbol
	for start := 0; start < len(symbols); start += size {
		end := start + size
		if end > len(symbols) {
			end = len(symbols)
		}
		chunks = append(chunks, symbols[start:end])
	}
	return chunks
}

func gridPosition(index int) (float64, float64) {
	col := index % 5
	row := index / 5
	return float64(col * 260), float64(row * 160)
}

func nullStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func repoIdentity(repo Repository) string {
	if repo.RemoteURL.Valid && strings.TrimSpace(repo.RemoteURL.String) != "" {
		return repo.RemoteURL.String
	}
	return repo.RepoRoot
}

func representationHash(filtered filterResult, req RepresentRequest) string {
	parts := []string{filtered.RawGraphHash, filtered.SettingsHash, stableHash(req)}
	for _, sym := range sortedSymbols(filtered.VisibleSymbols) {
		parts = append(parts, "s:"+sym.StableKey)
	}
	refs := append([]Reference(nil), filtered.VisibleReferences...)
	sort.Slice(refs, func(i, j int) bool {
		leftSource := filtered.VisibleSymbols[refs[i].SourceSymbolID].StableKey
		rightSource := filtered.VisibleSymbols[refs[j].SourceSymbolID].StableKey
		leftTarget := filtered.VisibleSymbols[refs[i].TargetSymbolID].StableKey
		rightTarget := filtered.VisibleSymbols[refs[j].TargetSymbolID].StableKey
		if leftSource == rightSource {
			if leftTarget == rightTarget {
				return refs[i].EvidenceHash < refs[j].EvidenceHash
			}
			return leftTarget < rightTarget
		}
		return leftSource < rightSource
	})
	for _, ref := range refs {
		source := filtered.VisibleSymbols[ref.SourceSymbolID].StableKey
		target := filtered.VisibleSymbols[ref.TargetSymbolID].StableKey
		parts = append(parts, fmt.Sprintf("r:%s:%s:%s:%s", source, target, ref.Kind, ref.EvidenceHash))
	}
	return stableHash(parts)
}
