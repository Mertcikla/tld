package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultEmbeddingBatchSize     = 512
	maxEmbeddingInputApproxTokens = 8000
	maxEmbeddingInputChars        = maxEmbeddingInputApproxTokens * 4
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

	allSymbols, err := r.Store.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return RepresentResult{}, err
	}
	identityKeys, err := r.Store.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		return RepresentResult{}, err
	}
	result := RepresentResult{}
	embeddingVectors := map[int64]Vector{}
	if model.Provider != "none" {
		stats, vectors, err := r.cacheEmbeddings(ctx, modelID, provider, repo.RepoRoot, allSymbols, identityKeys, req.Progress)
		if err != nil {
			return RepresentResult{}, err
		}
		result.EmbeddingCacheHits = stats.CacheHits
		result.EmbeddingsCreated = stats.Created
		embeddingVectors = vectors
	}

	filtered, err := runFilter(ctx, r.Store, repositoryID, req.Thresholds, rawGraphHash, settingsHash, embeddingVectors)
	if err != nil {
		return RepresentResult{}, err
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
	m := &materializer{store: r.Store, repo: repo, thresholds: thresholds, settingsHash: settingsHash, identityKeys: identityKeys}
	if err := m.ensureTags(ctx); err != nil {
		return m.stats, err
	}
	rootViewID, err := m.workspaceRootViewID(ctx)
	if err != nil {
		return m.stats, err
	}
	repoElem, err := m.upsertElement(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), elementInput{
		Name:       repo.DisplayName,
		Kind:       "repository",
		Technology: "Go",
		Repo:       repoIdentity(repo),
		Branch:     nullStringValue(repo.Branch),
		Language:   "go",
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
			Technology: "Go",
			Repo:       repoIdentity(repo),
			Branch:     nullStringValue(repo.Branch),
			FilePath:   folder,
			Language:   "go",
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
		parentView := repoView
		if dir := path.Dir(file); dir != "." {
			if id, ok := folderViews[dir]; ok {
				parentView = id
			}
		}
		elem, err := m.upsertElement(ctx, "file", "file:"+file, elementInput{
			Name:       path.Base(file),
			Kind:       "file",
			Technology: "Go",
			Repo:       repoIdentity(repo),
			Branch:     nullStringValue(repo.Branch),
			FilePath:   file,
			Language:   "go",
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
					Technology: "Go",
					Repo:       repoIdentity(repo),
					Branch:     nullStringValue(repo.Branch),
					FilePath:   file,
					Language:   "go",
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
			for i, sym := range chunk {
				elem, err := m.upsertElement(ctx, "symbol", symbolOwnerKey(sym, m.identityKeys), elementInput{
					Name:        sym.QualifiedName,
					Kind:        sym.Kind,
					Description: fmt.Sprintf("%s:%d", sym.FilePath, sym.StartLine),
					Technology:  "Go",
					Repo:        repoIdentity(repo),
					Branch:      nullStringValue(repo.Branch),
					FilePath:    sym.FilePath,
					Language:    "go",
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
	if err := m.pruneStaleConnectors(ctx); err != nil {
		return m.stats, err
	}
	return m.stats, nil
}

type materializer struct {
	store            *Store
	repo             Repository
	thresholds       Thresholds
	settingsHash     string
	identityKeys     map[string]string
	activeConnectors map[string]struct{}
	stats            materializeStats
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
	if err := m.store.SaveMapping(ctx, m.repo.ID, ownerType, ownerKey, "element", id); err != nil {
		return 0, err
	}
	m.stats.ElementsCreated++
	return id, nil
}

func (m *materializer) upsertView(ctx context.Context, ownerType, ownerKey string, ownerElementID int64, name, label string) (int64, error) {
	if id, ok, err := m.store.MappingResourceID(ctx, m.repo.ID, ownerType, ownerKey, "view"); err != nil {
		return 0, err
	} else if ok && viewExists(ctx, m.store.db, id) {
		_, err := m.store.db.ExecContext(ctx, `UPDATE views SET owner_element_id = ?, name = ?, level_label = ?, updated_at = ? WHERE id = ?`, ownerElementID, name, label, nowString(), id)
		return id, err
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
	if err := m.store.SaveMapping(ctx, m.repo.ID, ownerType, ownerKey, "view", id); err != nil {
		return 0, err
	}
	m.stats.ViewsCreated++
	return id, nil
}

func (m *materializer) upsertPlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	now := nowString()
	_, err := m.store.db.ExecContext(ctx, `
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(view_id, element_id) DO UPDATE SET position_x = excluded.position_x, position_y = excluded.position_y, updated_at = excluded.updated_at`,
		viewID, elementID, x, y, now, now)
	return err
}

func (m *materializer) materializeConnectors(ctx context.Context, refs []Reference, symbols map[int64]Symbol, fileElements map[string]int64, symbolElements map[int64]int64, symbolViews map[int64]int64, repoView int64) error {
	if m.activeConnectors == nil {
		m.activeConnectors = map[string]struct{}{}
	}
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
	for key, ref := range filePairs {
		source := symbols[ref.SourceSymbolID]
		target := symbols[ref.TargetSymbolID]
		if fileElements[source.FilePath] == 0 || fileElements[target.FilePath] == 0 {
			continue
		}
		if err := m.upsertConnector(ctx, "file-reference", "file:"+key, repoView, fileElements[source.FilePath], fileElements[target.FilePath], "references"); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) upsertConnector(ctx context.Context, ownerType, ownerKey string, viewID, sourceElementID, targetElementID int64, label string) error {
	if sourceElementID == 0 || targetElementID == 0 || sourceElementID == targetElementID {
		return nil
	}
	if m.activeConnectors != nil {
		m.activeConnectors[ownerType+"\x00"+ownerKey] = struct{}{}
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
	if err := m.store.SaveMapping(ctx, m.repo.ID, ownerType, ownerKey, "connector", id); err != nil {
		return err
	}
	m.stats.ConnectorsCreated++
	return nil
}

func (m *materializer) pruneStaleConnectors(ctx context.Context) error {
	if m.activeConnectors == nil {
		return nil
	}
	rows, err := m.store.db.QueryContext(ctx, `
		SELECT id, owner_type, owner_key, resource_id
		FROM watch_materialization
		WHERE repository_id = ? AND resource_type = 'connector'`, m.repo.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type staleMapping struct {
		ID         int64
		ResourceID int64
		OwnerType  string
		OwnerKey   string
	}
	var stale []staleMapping
	for rows.Next() {
		var mapping staleMapping
		if err := rows.Scan(&mapping.ID, &mapping.OwnerType, &mapping.OwnerKey, &mapping.ResourceID); err != nil {
			return err
		}
		if _, ok := m.activeConnectors[mapping.OwnerType+"\x00"+mapping.OwnerKey]; !ok {
			stale = append(stale, mapping)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, mapping := range stale {
		if _, err := m.store.db.ExecContext(ctx, `DELETE FROM connectors WHERE id = ?`, mapping.ResourceID); err != nil {
			return err
		}
		if _, err := m.store.db.ExecContext(ctx, `DELETE FROM watch_materialization WHERE id = ?`, mapping.ID); err != nil {
			return err
		}
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
