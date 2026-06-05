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

	"github.com/mertcikla/tld/v2/internal/codeowners"
	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/tagcolors"
	"github.com/mertcikla/tld/v2/internal/tech"
)

const (
	defaultEmbeddingBatchSize     = 16
	maxEmbeddingInputApproxTokens = 4000
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
	req.Visibility = defaultVisibilityConfig(req.Visibility)
	settingsHash := settingsHash(req)
	prepareStarted := time.Now()
	logInfo(ctx, req.Logger, "watch.representation.prepare.started", "repository_id", repositoryID)
	progressStart(req.Progress, "Preparing representation graph", 8)
	rawGraphHash, err := r.Store.RawGraphHash(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Raw graph hashed")
	repo, err := r.Store.Repository(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Repository loaded")

	provider, err := NewEmbeddingProvider(req.Embedding)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	if closer, ok := provider.(ClosableProvider); ok {
		defer func() { _ = closer.Close() }()
	}
	progressAdvance(req.Progress, "Embedding provider configured")
	model := provider.ModelID()
	modelID, err := r.Store.EnsureEmbeddingModel(ctx, EmbeddingConfig{Provider: model.Provider, Model: model.Model, Dimension: model.Dimension}, model.ConfigHash)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Embedding model registered")
	modelIDPtr := &modelID
	if model.Provider == "none" {
		modelIDPtr = nil
	}

	contextPolicies, err := r.Store.ActiveContextPolicySet(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Context policies loaded")
	contextExpansions, err := r.Store.ActiveContextExpansionSet(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	blastRadiusFiles := normalizeBlastRadiusFiles(req.BlastRadiusFiles)
	reuseAllowed := req.AssumeNoRawChanges && len(blastRadiusFiles) == 0 && len(contextPolicies.Show) == 0 && len(contextPolicies.Hide) == 0 && len(contextExpansions.Tiers) == 0
	if reuseAllowed {
		cached, reused, err := r.reuseRepresentation(ctx, repositoryID, rawGraphHash, settingsHash, modelIDPtr, prepareStarted, req)
		if err != nil {
			progressFinish(req.Progress)
			logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
			return RepresentResult{}, err
		}
		if reused {
			progressFinish(req.Progress)
			return cached, nil
		}
	}
	identityKeys, err := r.Store.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Symbol identities loaded")
	changedRaw, err := r.Store.ChangedRawResourcesSinceLatest(ctx, repositoryID)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	progressAdvance(req.Progress, "Changed resources loaded")
	if len(changedRaw.Files) == 0 && len(changedRaw.Symbols) == 0 && len(blastRadiusFiles) == 0 && len(contextPolicies.Show) == 0 && len(contextPolicies.Hide) == 0 && len(contextExpansions.Tiers) == 0 {
		cached, reused, err := r.reuseRepresentation(ctx, repositoryID, rawGraphHash, settingsHash, modelIDPtr, prepareStarted, req)
		if err != nil {
			progressFinish(req.Progress)
			logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
			return RepresentResult{}, err
		}
		if reused {
			progressFinish(req.Progress)
			return cached, nil
		}
	}
	filtered, err := runFilter(ctx, r.Store, repositoryID, req.Thresholds, req.Visibility, rawGraphHash, settingsHash, nil, changedRaw.Symbols, blastRadiusFiles, contextPolicies, identityKeys)
	if err != nil {
		progressFinish(req.Progress)
		logError(ctx, req.Logger, "watch.representation.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID)
		return RepresentResult{}, err
	}
	filtered.Dependencies = req.Dependencies
	filtered.ChangedFiles = changedRaw.Files
	progressAdvance(req.Progress, "Architecture view filtered")
	progressFinish(req.Progress)
	logInfo(ctx, req.Logger, "watch.representation.prepare.completed", "elapsed", logElapsed(prepareStarted), "repository_id", repositoryID, "raw_graph_hash", rawGraphHash, "provider", model.Provider, "model", model.Model, "visible_symbols", len(filtered.VisibleSymbols), "visible_references", len(filtered.VisibleReferences), "visible_files", len(filtered.VisibleFiles), "visible_facts", len(filtered.VisibleFacts), "changed_files", len(changedRaw.Files), "changed_symbols", len(changedRaw.Symbols))

	result := RepresentResult{}
	if model.Provider != "none" {
		embeddingSymbols := embeddingCandidateSymbols(filtered.VisibleSymbols, maxEmbeddingSymbolsPerRun)
		embeddingStarted := time.Now()
		logInfo(ctx, req.Logger, "watch.representation.embeddings.started", "repository_id", repositoryID, "symbols", len(embeddingSymbols))
		stats, vectors, err := r.cacheEmbeddings(ctx, modelID, provider, repo.RepoRoot, embeddingSymbols, identityKeys, req.Progress, time.Duration(req.Embedding.TimeoutSeconds)*time.Second, req.Embedding.MaxTokens)
		if err != nil {
			logError(ctx, req.Logger, "watch.representation.embeddings.failed", err, "elapsed", logElapsed(embeddingStarted), "repository_id", repositoryID)
			return RepresentResult{}, err
		}
		logInfo(ctx, req.Logger, "watch.representation.embeddings.completed", "elapsed", logElapsed(embeddingStarted), "repository_id", repositoryID, "cache_hits", stats.CacheHits, "created", stats.Created)
		result.EmbeddingCacheHits = stats.CacheHits
		result.EmbeddingsCreated = stats.Created
		if len(embeddingSymbols) == len(filtered.VisibleSymbols) {
			progressStart(req.Progress, "Refreshing semantic filter", 1)
			filtered, err = runFilter(ctx, r.Store, repositoryID, req.Thresholds, req.Visibility, rawGraphHash, settingsHash, vectors, changedRaw.Symbols, blastRadiusFiles, contextPolicies, identityKeys)
			if err != nil {
				progressFinish(req.Progress)
				logError(ctx, req.Logger, "watch.representation.semantic_filter.failed", err, "repository_id", repositoryID)
				return RepresentResult{}, err
			}
			filtered.Dependencies = req.Dependencies
			filtered.ChangedFiles = changedRaw.Files
			progressAdvance(req.Progress, "Semantic filter refreshed")
			progressFinish(req.Progress)
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
		logError(ctx, req.Logger, "watch.representation.run_begin.failed", err, "repository_id", repositoryID)
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

	materializeStarted := time.Now()
	logInfo(ctx, req.Logger, "watch.representation.materialize.started", "repository_id", repositoryID, "representation_run_id", runID)
	progressStart(req.Progress, "Materializing representation", 3)
	ownerMatcher, err := codeowners.Load(repo.RepoRoot)
	if err != nil {
		progressFinish(req.Progress)
		runErr = err
		logError(ctx, req.Logger, "watch.representation.materialize.failed", err, "elapsed", logElapsed(materializeStarted), "repository_id", repositoryID)
		return result, err
	}
	progressAdvance(req.Progress, "Ownership metadata loaded")
	applyToken := randomToken()
	if err := r.Store.AcquireApplyLock(ctx, repositoryID, os.Getpid(), applyToken, LockHeartbeatTimeout); err != nil {
		progressFinish(req.Progress)
		runErr = err
		logError(ctx, req.Logger, "watch.representation.materialize.failed", err, "elapsed", logElapsed(materializeStarted), "repository_id", repositoryID)
		return result, err
	}
	progressAdvance(req.Progress, "Apply lock acquired")
	defer func() {
		_ = r.Store.ReleaseApplyLock(context.Background(), repositoryID, applyToken)
	}()
	stats, err := r.materialize(ctx, repo, filtered, req.Thresholds, settingsHash, identityKeys, ownerMatcher)
	if err != nil {
		progressFinish(req.Progress)
		runErr = err
		logError(ctx, req.Logger, "watch.representation.materialize.failed", err, "elapsed", logElapsed(materializeStarted), "repository_id", repositoryID)
		return result, err
	}
	if model.Provider != "none" {
		resourceStats, err := r.cachePopulateResourceEmbeddings(ctx, modelID, provider, repo.ID, req.Progress, time.Duration(req.Embedding.TimeoutSeconds)*time.Second)
		if err != nil {
			progressFinish(req.Progress)
			runErr = err
			logError(ctx, req.Logger, "watch.representation.populate_resource_embeddings.failed", err, "repository_id", repositoryID)
			return result, err
		}
		result.EmbeddingCacheHits += resourceStats.CacheHits
		result.EmbeddingsCreated += resourceStats.Created
	}
	progressAdvance(req.Progress, "Resources materialized")
	progressFinish(req.Progress)
	result.ElementsCreated = stats.ElementsCreated
	result.ElementsUpdated = stats.ElementsUpdated
	result.ConnectorsCreated = stats.ConnectorsCreated
	result.ConnectorsUpdated = stats.ConnectorsUpdated
	result.ViewsCreated = stats.ViewsCreated
	result.ElementsPreserved = stats.ElementsPreserved
	result.ConnectorsPreserved = stats.ConnectorsPreserved
	result.ViewsPreserved = stats.ViewsPreserved
	result.DeletesPreserved = stats.DeletesPreserved
	logInfo(ctx, req.Logger, "watch.representation.materialize.completed", "elapsed", logElapsed(materializeStarted), "repository_id", repositoryID, "representation_run_id", runID, "elements_created", result.ElementsCreated, "elements_updated", result.ElementsUpdated, "connectors_created", result.ConnectorsCreated, "connectors_updated", result.ConnectorsUpdated, "views_created", result.ViewsCreated, "elements_preserved", result.ElementsPreserved, "connectors_preserved", result.ConnectorsPreserved, "views_preserved", result.ViewsPreserved, "deletes_preserved", result.DeletesPreserved)
	return result, nil
}

func (r *Representer) reuseRepresentation(ctx context.Context, repositoryID int64, rawGraphHash, settingsHash string, modelID *int64, started time.Time, req RepresentRequest) (RepresentResult, bool, error) {
	latest, found, err := r.Store.LatestWatchVersion(ctx, repositoryID)
	if err != nil || !found {
		return RepresentResult{}, false, err
	}
	cached, cachedFound, err := r.Store.LatestCompletedRepresentationRun(ctx, repositoryID, rawGraphHash, settingsHash, modelID)
	if err != nil || !cachedFound {
		return RepresentResult{}, false, err
	}
	if cached.RepresentationHash != latest.RepresentationHash {
		return RepresentResult{}, false, nil
	}
	cached.ElementsCreated = 0
	cached.ElementsUpdated = 0
	cached.ConnectorsCreated = 0
	cached.ConnectorsUpdated = 0
	cached.ViewsCreated = 0
	logInfo(ctx, req.Logger, "watch.representation.reused", "elapsed", logElapsed(started), "repository_id", repositoryID, "representation_run_id", cached.RepresentationRun, "raw_graph_hash", rawGraphHash, "representation_hash", cached.RepresentationHash, "assume_no_raw_changes", req.AssumeNoRawChanges)
	return cached, true, nil
}

func normalizeBlastRadiusFiles(files []string) map[string]string {
	out := map[string]string{}
	for _, file := range files {
		file = strings.TrimSpace(filepathToSlash(file))
		file = strings.TrimPrefix(file, "file:")
		if file == "" || file == "." || strings.HasPrefix(file, "../") || filepath.IsAbs(file) {
			continue
		}
		out[file] = "blast radius of changed file"
	}
	return out
}

func (r *Representer) RepresentArchitecture(ctx context.Context, repo Repository, architecture architectureModel, thresholds Thresholds, progress ProgressSink) (RepresentResult, error) {
	if r == nil || r.Store == nil {
		return RepresentResult{}, fmt.Errorf("watch representer requires a store")
	}
	thresholds = defaultThresholds(thresholds)
	rawGraphHash := stableHash(architecture)
	settingsHash := stableHash(thresholds)
	representationHash := stableHash([]any{rawGraphHash, settingsHash, "architecture"})
	result := RepresentResult{
		RepositoryID:       repo.ID,
		RawGraphHash:       rawGraphHash,
		SettingsHash:       settingsHash,
		RepresentationHash: representationHash,
	}
	runID, err := r.Store.BeginRepresentationRun(ctx, repo.ID, rawGraphHash, settingsHash, nil, representationHash)
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

	progressStart(progress, "Materializing architecture view", 7)
	applyToken := randomToken()
	if err := r.Store.AcquireApplyLock(ctx, repo.ID, os.Getpid(), applyToken, LockHeartbeatTimeout); err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Apply lock acquired")
	defer func() {
		_ = r.Store.ReleaseApplyLock(context.Background(), repo.ID, applyToken)
	}()

	initialLayout, err := r.Store.RepositoryMaterializationCount(ctx, repo.ID)
	if err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Existing materialization inspected")
	m := &materializer{
		store:         r.Store,
		repo:          repo,
		thresholds:    thresholds,
		settingsHash:  settingsHash,
		identityKeys:  map[string]string{},
		tagPlan:       semanticTagPlan{approved: map[string]struct{}{}, byOwner: map[string][]string{}},
		initialLayout: initialLayout == 0,
		runMarker:     time.Now().UTC().Format(time.RFC3339Nano),
		newPlacements: map[int64]map[int64]struct{}{},
	}
	rootViewID, err := m.workspaceRootViewID(ctx)
	if err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Workspace root loaded")
	repoElem, err := m.upsertElement(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), elementInput{
		Name:       repo.DisplayName,
		Kind:       "repository",
		Technology: "Runtime",
		Repo:       repoIdentity(repo),
		Branch:     nullStringValue(repo.Branch),
		Tags:       []string{"view:architecture"},
	})
	if err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	if err := m.upsertPlacement(ctx, rootViewID, repoElem, 0, 0); err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	repoView, err := m.upsertView(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), repoElem, repo.DisplayName, "Architecture")
	if err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Repository view materialized")
	if err := m.materializeArchitecture(ctx, architecture, repoView); err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Architecture resources materialized")
	if err := m.pruneStaleResources(ctx); err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Stale generated resources pruned")
	if err := m.layoutPlacements(ctx); err != nil {
		progressFinish(progress)
		runErr = err
		return result, err
	}
	progressAdvance(progress, "Layout updated")
	progressFinish(progress)
	result.ElementsCreated = m.stats.ElementsCreated
	result.ElementsUpdated = m.stats.ElementsUpdated
	result.ConnectorsCreated = m.stats.ConnectorsCreated
	result.ConnectorsUpdated = m.stats.ConnectorsUpdated
	result.ViewsCreated = m.stats.ViewsCreated
	result.ElementsPreserved = m.stats.ElementsPreserved
	result.ConnectorsPreserved = m.stats.ConnectorsPreserved
	result.ViewsPreserved = m.stats.ViewsPreserved
	result.DeletesPreserved = m.stats.DeletesPreserved
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

func (r *Representer) cacheEmbeddings(ctx context.Context, modelID int64, provider Provider, repoRoot string, symbols []Symbol, identityKeys map[string]string, progress ProgressSink, timeout time.Duration, maxTokens int) (embeddingCacheStats, map[int64]Vector, error) {
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
		input := EmbeddingInput{OwnerType: "symbol", OwnerKey: ownerKey, Text: symbolEmbeddingText(repoRoot, sym, maxTokens)}
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

	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	vectors := make([]Vector, 0, len(inputs))
	progressStart(progress, "Embedding symbols", len(inputs))
	for start := 0; start < len(inputs); start += defaultEmbeddingBatchSize {
		if err := ctx.Err(); err != nil {
			progressFinish(progress)
			return stats, vectorsBySymbol, err
		}
		end := min(start+defaultEmbeddingBatchSize, len(inputs))
		chunk := inputs[start:end]
		embedCtx, cancel := context.WithTimeout(ctx, timeout)
		chunkVectors, err := provider.Embed(embedCtx, chunk)
		cancel()
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

func (r *Representer) cachePopulateResourceEmbeddings(ctx context.Context, modelID int64, provider Provider, repositoryID int64, progress ProgressSink, timeout time.Duration) (embeddingCacheStats, error) {
	stats := embeddingCacheStats{}
	model := provider.ModelID()
	if model.Provider == "none" {
		return stats, nil
	}
	inputs, err := r.populateResourceEmbeddingInputs(ctx, repositoryID)
	if err != nil {
		return stats, err
	}
	if len(inputs) == 0 {
		return stats, nil
	}
	missing := make([]EmbeddingInput, 0, len(inputs))
	progressStart(progress, "Preparing populate resource embeddings", len(inputs))
	for _, input := range inputs {
		if _, ok, err := r.Store.Embedding(ctx, modelID, input.OwnerType, input.OwnerKey, inputHash(input)); err != nil {
			progressFinish(progress)
			return stats, err
		} else if ok {
			stats.CacheHits++
		} else {
			missing = append(missing, input)
		}
		progressAdvance(progress, input.OwnerKey)
	}
	progressFinish(progress)
	if len(missing) == 0 {
		return stats, nil
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	progressStart(progress, "Embedding populate resources", len(missing))
	for start := 0; start < len(missing); start += defaultEmbeddingBatchSize {
		if err := ctx.Err(); err != nil {
			progressFinish(progress)
			return stats, err
		}
		end := min(start+defaultEmbeddingBatchSize, len(missing))
		chunk := missing[start:end]
		embedCtx, cancel := context.WithTimeout(ctx, timeout)
		vectors, err := provider.Embed(embedCtx, chunk)
		cancel()
		if err != nil {
			progressFinish(progress)
			return stats, err
		}
		if len(vectors) != len(chunk) {
			progressFinish(progress)
			return stats, fmt.Errorf("embedding provider returned %d vectors for %d populate resources", len(vectors), len(chunk))
		}
		for i, input := range chunk {
			if err := r.Store.SaveEmbedding(ctx, modelID, input.OwnerType, input.OwnerKey, inputHash(input), vectorBytes(vectors[i])); err != nil {
				progressFinish(progress)
				return stats, err
			}
			stats.Created++
			progressAdvance(progress, input.OwnerKey)
		}
	}
	progressFinish(progress)
	return stats, nil
}

func (r *Representer) populateResourceEmbeddingInputs(ctx context.Context, repositoryID int64) ([]EmbeddingInput, error) {
	rows, err := r.Store.rowsRaw(ctx, `
		SELECT m.owner_type, m.owner_key, el.name, COALESCE(el.kind, ''), COALESCE(el.description, ''), COALESCE(el.technology, ''), COALESCE(el.file_path, ''), COALESCE(el.language, ''), el.tags
		FROM watch_materialization m
		JOIN elements el ON el.id = m.resource_id
		WHERE m.repository_id = ?
		  AND m.resource_type = 'element'
		  AND COALESCE(el.kind, '') IN ('architecture-component', 'repository-section', 'folder', 'cluster', 'dependency-group', 'fact-summary', 'repository', 'file')
		ORDER BY m.owner_type, m.owner_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	type resourceRow struct {
		ownerType, ownerKey, name, kind, description, technology, filePath, language, tags string
	}
	resources := []resourceRow{}
	for rows.Next() {
		var resource resourceRow
		if err := rows.Scan(&resource.ownerType, &resource.ownerKey, &resource.name, &resource.kind, &resource.description, &resource.technology, &resource.filePath, &resource.language, &resource.tags); err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	inputs := []EmbeddingInput{}
	for _, resource := range resources {
		text, err := r.populateResourceEmbeddingText(ctx, repositoryID, resource.ownerType, resource.ownerKey, resource.name, resource.kind, resource.description, resource.technology, resource.filePath, resource.language, resource.tags)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, EmbeddingInput{
			OwnerType: "populate_resource",
			OwnerKey:  resource.ownerType + ":" + resource.ownerKey,
			Text:      text,
			Language:  resource.language,
		})
	}
	return inputs, nil
}

func (r *Representer) populateResourceEmbeddingText(ctx context.Context, repositoryID int64, ownerType, ownerKey, name, kind, description, technology, filePath, language, tags string) (string, error) {
	parts := []string{}
	if strings.TrimSpace(name) != "" {
		parts = append(parts, "name "+name)
	}
	if kind != "" {
		parts = append(parts, "kind "+kind)
	}
	parts = append(parts, fmt.Sprintf("owner %s %s", ownerType, ownerKey))
	if signals := embeddingSemanticSignals(name, kind, filePath, tags); len(signals) > 0 {
		parts = append(parts, "signals "+strings.Join(signals, ", "))
	}
	for _, s := range []string{description, technology, filePath} {
		s = strings.TrimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}
	children, err := r.populateResourceChildSummary(ctx, ownerType, ownerKey)
	if err != nil {
		return "", err
	}
	if children != "" {
		parts = append(parts, "children "+children)
	}
	refs, err := r.populateResourceReferenceSummary(ctx, repositoryID, filePath)
	if err != nil {
		return "", err
	}
	if refs != "" {
		parts = append(parts, "refs "+refs)
	}
	return shrinkEmbeddingText(strings.Join(parts, " "), maxEmbeddingInputApproxTokens), nil
}

func (r *Representer) populateResourceChildSummary(ctx context.Context, ownerType, ownerKey string) (string, error) {
	rows, err := r.Store.rowsRaw(ctx, `
		SELECT child.name
		FROM watch_materialization parent
		JOIN views v ON v.owner_element_id = parent.resource_id
		JOIN placements p ON p.view_id = v.id
		JOIN elements child ON child.id = p.element_id
		WHERE parent.owner_type = ? AND parent.owner_key = ? AND parent.resource_type = 'element'
		ORDER BY p.id
		LIMIT 16`, ownerType, ownerKey)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	values := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		values = append(values, name)
	}
	return strings.Join(compactStrings(values), ", "), rows.Err()
}

func (r *Representer) populateResourceReferenceSummary(ctx context.Context, repositoryID int64, filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", nil
	}
	rows, err := r.Store.rowsRaw(ctx, `
		SELECT DISTINCT target.name
		FROM watch_symbols source
		JOIN watch_references ref ON ref.source_symbol_id = source.id
		JOIN watch_symbols target ON target.id = ref.target_symbol_id
		WHERE source.repository_id = ? AND source.file_id IN (SELECT id FROM watch_files WHERE repository_id = ? AND path = ?)
		ORDER BY target.name
		LIMIT 12`, repositoryID, repositoryID, filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	values := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		values = append(values, name)
	}
	return strings.Join(compactStrings(values), ", "), rows.Err()
}

func embeddingCandidateSymbols(symbols map[int64]Symbol, limit int) []Symbol {
	out := sortedSymbols(symbols)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func symbolEmbeddingText(repoRoot string, sym Symbol, maxTokens int) string {
	body := symbolCodeBody(repoRoot, sym)
	name := strings.TrimSpace(sym.QualifiedName)
	if name == "" {
		name = strings.TrimSpace(sym.Name)
	}
	parts := []string{}
	if name != "" {
		parts = append(parts, "symbol name "+name)
	}
	if strings.TrimSpace(sym.Kind) != "" {
		parts = append(parts, "kind "+sym.Kind)
	}
	if strings.TrimSpace(sym.FilePath) != "" {
		parts = append(parts, "path "+sym.FilePath)
	}
	if signals := embeddingSemanticSignals(name, sym.Kind, sym.FilePath, ""); len(signals) > 0 {
		parts = append(parts, "signals "+strings.Join(signals, ", "))
	}
	if strings.TrimSpace(body) == "" {
		return shrinkEmbeddingText(strings.Join(parts, "\n"), maxTokens)
	}
	parts = append(parts, "code\n"+outdentCode(body))
	return shrinkEmbeddingText(strings.Join(parts, "\n"), maxTokens)
}

func embeddingSemanticSignals(name, kind, filePath, tagsJSON string) []string {
	signals := []string{}
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(strings.ReplaceAll(value, ":", " "))
			if value != "" && !containsEmbeddingSignal(signals, value) {
				signals = append(signals, value)
			}
		}
	}
	if kind != "" {
		add("granularity " + kind)
	}
	sourceParts := []string{name, kind, filePath}
	for _, tag := range decodeEmbeddingTags(tagsJSON) {
		sourceParts = append(sourceParts, strings.ReplaceAll(tag, ":", " "))
	}
	source := strings.ToLower(strings.Join(sourceParts, " "))
	for _, rule := range embeddingSignalRules {
		if hasEmbeddingSignalCue(source, rule.cues) {
			add(rule.signals...)
		}
	}
	return signals
}

type embeddingSignalRule struct {
	cues    []string
	signals []string
}

var embeddingSignalRules = []embeddingSignalRule{
	{
		cues:    []string{"api", "http", "rpc", "grpc", "rest", "graphql", "endpoint", "route", "handler", "controller", "websocket", "webhook"},
		signals: []string{"responsibility interface boundary", "intent request handling", "intent service endpoint"},
	},
	{
		cues:    []string{"cli", "cmd", "command", "terminal", "shell"},
		signals: []string{"responsibility operator interface", "intent command workflow"},
	},
	{
		cues:    []string{"ui", "frontend", "front end", "view", "page", "component", "canvas", "render", "screen", "layout", "style"},
		signals: []string{"responsibility presentation", "intent user interaction", "intent rendering"},
	},
	{
		cues:    []string{"store", "storage", "persist", "database", "db", "sql", "sqlite", "postgres", "mysql", "mongo", "redis", "cache", "migration", "schema", "repository", "dao"},
		signals: []string{"responsibility data storage", "intent persistence", "intent data access"},
	},
	{
		cues:    []string{"client", "adapter", "gateway", "connector", "provider", "sdk", "integration", "external", "third party"},
		signals: []string{"responsibility external integration", "intent dependency boundary", "intent protocol adaptation"},
	},
	{
		cues:    []string{"job", "worker", "queue", "scheduler", "cron", "event", "stream", "message", "consumer", "producer", "pubsub", "workflow", "state machine"},
		signals: []string{"responsibility orchestration", "intent asynchronous processing", "intent event flow"},
	},
	{
		cues:    []string{"parser", "analyzer", "compiler", "extractor", "mapper", "transformer", "serializer", "validator", "generator", "import", "export", "sync"},
		signals: []string{"responsibility transformation", "intent data interpretation", "intent validation"},
	},
	{
		cues:    []string{"service", "usecase", "use case", "domain", "entity", "aggregate", "policy", "rule", "manager", "coordinator"},
		signals: []string{"responsibility domain logic", "intent business rule", "intent coordination"},
	},
	{
		cues:    []string{"config", "setting", "option", "flag", "env", "environment", "yaml", "toml", "properties", "preference"},
		signals: []string{"responsibility configuration", "intent runtime tuning", "intent environment setup"},
	},
	{
		cues:    []string{"test", "spec", "mock", "fixture", "assert", "benchmark", "e2e", "integration test"},
		signals: []string{"responsibility validation", "intent test coverage", "intent fixture setup"},
	},
	{
		cues:    []string{"build", "release", "deploy", "docker", "container", "ci", "cd", "workflow", "pipeline", "package", "publish", "install"},
		signals: []string{"responsibility delivery", "intent build automation", "intent deployment"},
	},
	{
		cues:    []string{"log", "metric", "trace", "telemetry", "monitor", "health", "alert", "diagnostic"},
		signals: []string{"responsibility observability", "intent operational insight", "intent health reporting"},
	},
	{
		cues:    []string{"security", "permission", "token", "secret", "credential", "crypto", "certificate", "policy"},
		signals: []string{"responsibility security boundary", "intent access protection", "intent sensitive data handling"},
	},
}

func hasEmbeddingSignalCue(source string, cues []string) bool {
	tokens := embeddingSignalTokens(source)
	for _, cue := range cues {
		cue = strings.ToLower(strings.TrimSpace(cue))
		if cue == "" {
			continue
		}
		if strings.Contains(cue, " ") {
			if strings.Contains(source, cue) {
				return true
			}
			continue
		}
		if len(cue) <= 3 {
			if tokens[cue] {
				return true
			}
			continue
		}
		for token := range tokens {
			if token == cue || strings.Contains(token, cue) {
				return true
			}
		}
	}
	return false
}

func embeddingSignalTokens(source string) map[string]bool {
	tokens := map[string]bool{}
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens[current.String()] = true
			current.Reset()
		}
	}
	for _, r := range strings.ToLower(source) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func containsEmbeddingSignal(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func decodeEmbeddingTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
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

func shrinkEmbeddingText(text string, maxTokens int) string {
	text = strings.TrimSpace(text)
	if maxTokens <= 0 {
		maxTokens = maxEmbeddingInputApproxTokens
	}
	if approximateTokenCount(text) <= maxTokens {
		return text
	}
	text = dropLowSignalCodeLines(text)
	if approximateTokenCount(text) <= maxTokens {
		return text
	}
	maxChars := maxTokens * 4
	if len(text) <= maxChars {
		return text
	}
	marker := "\n\n/* ... middle omitted for embedding context ... */\n\n"
	keep := maxChars - len(marker)
	if keep <= 0 {
		return text[:maxChars]
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
	ElementsCreated     int
	ElementsUpdated     int
	ConnectorsCreated   int
	ConnectorsUpdated   int
	ViewsCreated        int
	ElementsPreserved   int
	ConnectorsPreserved int
	ViewsPreserved      int
	DeletesPreserved    int
}

const (
	minSemanticTagCoverage      = 2
	maxSemanticTagsPerElement   = 5
	maxUsefulSemanticTagRatio   = 0.70
	semanticTagOwnerKeyJoinChar = "\x00"
)

type semanticTagPlan struct {
	approved map[string]struct{}
	byOwner  map[string][]string
}

func buildSemanticTagPlan(repo Repository, filtered filterResult, thresholds Thresholds, settingsHash string, identityKeys map[string]string, ownerMatcher *codeowners.Matcher, facts []Fact) semanticTagPlan {
	candidates := map[string][]string{}
	add := func(ownerType, ownerKey string, tags ...string) {
		key := semanticTagOwnerKey(ownerType, ownerKey)
		candidates[key] = roleSemanticTags(uniqueSemanticTags(append(candidates[key], tags...)))
	}

	languages := newSymbolLanguageIndex(filtered.VisibleSymbols)
	repoLanguage := languages.repoLanguage
	add("repository", fmt.Sprintf("repository:%d", repo.ID), semanticLanguageTag(repoLanguage))

	visibleFiles := filesForSymbols(filtered.VisibleSymbols)
	for file := range filtered.VisibleFiles {
		visibleFiles[file] = struct{}{}
	}
	for _, folder := range folderSet(visibleFiles) {
		add("folder", "folder:"+folder, append(semanticPathTags(folder, repoLanguage), ownerMatcher.TagsForPath(folder)...)...)
	}
	for file := range visibleFiles {
		add("file", "file:"+file, append(semanticPathTags(file, languages.languageForFile(file)), ownerMatcher.TagsForPath(file)...)...)
	}

	for file, symbols := range symbolsByFile(filtered.VisibleSymbols) {
		chunks := chunkSymbols(symbols, thresholds.MaxElementsPerView)
		for _, chunk := range chunks {
			if len(chunks) <= 1 || len(chunk) == 0 {
				continue
			}
			keys := make([]string, 0, len(chunk))
			for _, sym := range chunk {
				keys = append(keys, sym.StableKey)
			}
			clusterKey := stableClusterKey(repo.ID, file, settingsHash, keys)
			add("cluster", clusterKey, semanticPathTags(file, languageFromStableKey(chunk[0].StableKey))...)
		}
	}

	for _, sym := range sortedSymbols(filtered.VisibleSymbols) {
		tags := semanticPathTags(sym.FilePath, languageFromStableKey(sym.StableKey))
		tags = append(tags, semanticKindTag(sym.Kind))
		tags = append(tags, semanticSymbolRoleTags(sym, filtered.Incoming[sym.ID], filtered.Outgoing[sym.ID])...)
		tags = append(tags, ownerMatcher.TagsForPath(sym.FilePath)...)
		add("symbol", symbolOwnerKey(sym, identityKeys), tags...)
	}

	addFactSemanticTags(facts, filtered.VisibleSymbols, identityKeys, add)

	for _, fact := range facts {
		if dependencyImportFact(fact) {
			module := dependencyImportName(fact)
			if strings.TrimSpace(module) != "" {
				ownerKey := dependencyModuleOwnerKey(module)
				add("dependency-module", ownerKey, "role:dependency")
			}
		}
	}

	counts := map[string]int{}
	forced := map[string]struct{}{}
	for _, tags := range candidates {
		for _, tag := range tags {
			counts[tag]++
			if strings.HasPrefix(tag, "owner:") || forceFactSemanticTag(tag) {
				forced[tag] = struct{}{}
			}
		}
	}
	total := len(candidates)
	maxCoverage := int(math.Floor(float64(total) * maxUsefulSemanticTagRatio))
	if maxCoverage < minSemanticTagCoverage {
		maxCoverage = total - 1
	}
	approved := map[string]struct{}{}
	for tag, count := range counts {
		if _, ok := forced[tag]; ok {
			approved[tag] = struct{}{}
			continue
		}
		if count < minSemanticTagCoverage {
			continue
		}
		if total > 1 && count > maxCoverage {
			continue
		}
		approved[tag] = struct{}{}
	}

	byOwner := map[string][]string{}
	for key, tags := range candidates {
		var kept []string
		for _, tag := range tags {
			if _, ok := approved[tag]; ok {
				kept = append(kept, tag)
			}
		}
		sort.SliceStable(kept, func(i, j int) bool {
			left, right := semanticTagPriority(kept[i]), semanticTagPriority(kept[j])
			if left == right {
				return kept[i] < kept[j]
			}
			return left < right
		})
		byOwner[key] = limitSemanticTags(kept)
	}
	return semanticTagPlan{approved: approved, byOwner: byOwner}
}

func addFactSemanticTags(facts []Fact, symbols map[int64]Symbol, identityKeys map[string]string, add func(ownerType, ownerKey string, tags ...string)) {
	symbolOwners := map[string]string{}
	for _, sym := range symbols {
		symbolOwners[sym.StableKey] = symbolOwnerKey(sym, identityKeys)
	}
	for _, fact := range facts {
		tags := factSemanticTags(fact)
		if len(tags) == 0 {
			continue
		}
		add("fact", factOwnerKey(fact), tags...)
		if fact.SubjectKind == "symbol" {
			if owner, ok := symbolOwners[fact.SubjectStableKey]; ok {
				add("symbol", owner, tags...)
				continue
			}
		}
		if strings.TrimSpace(fact.FilePath) != "" {
			add("file", "file:"+fact.FilePath, tags...)
		}
	}
}

func factSemanticTags(fact Fact) []string {
	tags := append([]string{}, fact.Tags...)
	switch fact.Type {
	case "http.route":
		tags = append(tags, "http:route")
	case "frontend.route":
		tags = append(tags, "frontend:route")
	case "orm.query":
		if !hasStringPrefix(tags, "orm:") {
			tags = append(tags, "orm:query")
		}
	}
	return uniqueSemanticTags(tags)
}

func roleSemanticTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.HasPrefix(tag, "role:") {
			out = append(out, tag)
		}
	}
	return out
}

func forceFactSemanticTag(tag string) bool {
	return strings.HasPrefix(tag, "framework:") ||
		strings.HasPrefix(tag, "orm:") ||
		strings.HasPrefix(tag, "technology:") ||
		tag == "http:route" ||
		tag == "frontend:route" ||
		tag == "role:dependency"
}

func hasStringPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func limitSemanticTags(tags []string) []string {
	if len(tags) <= maxSemanticTagsPerElement {
		return tags
	}
	var forced []string
	var regular []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "owner:") {
			forced = append(forced, tag)
			continue
		}
		regular = append(regular, tag)
	}
	limit := max(maxSemanticTagsPerElement-len(forced), 0)
	if len(regular) > limit {
		regular = regular[:limit]
	}
	out := append(regular, forced...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := semanticTagPriority(out[i]), semanticTagPriority(out[j])
		if left == right {
			return out[i] < out[j]
		}
		return left < right
	})
	return out
}

func (p semanticTagPlan) tagsFor(ownerType, ownerKey string) []string {
	tags := p.byOwner[semanticTagOwnerKey(ownerType, ownerKey)]
	return append([]string(nil), tags...)
}

func (p semanticTagPlan) approvedTags() []string {
	tags := make([]string, 0, len(p.approved))
	for tag := range p.approved {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func semanticTagOwnerKey(ownerType, ownerKey string) string {
	return ownerType + semanticTagOwnerKeyJoinChar + ownerKey
}

func uniqueSemanticTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !strings.HasPrefix(tag, "owner:") {
			tag = strings.ToLower(tag)
		}
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func semanticPathTags(filePath, language string) []string {
	var tags []string
	if area := semanticAreaTag(filePath); area != "" {
		tags = append(tags, area)
	}
	tags = append(tags, semanticRoleTags(filePath)...)
	if tag := semanticLanguageTag(language); tag != "" {
		tags = append(tags, tag)
	}
	return tags
}

func semanticAreaTag(filePath string) string {
	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) == 1 {
		return ""
	}
	return "area:" + semanticTagSlug(parts[0])
}

func semanticLanguageTag(language string) string {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" || language == "source" {
		return ""
	}
	return "lang:" + semanticTagSlug(language)
}

func semanticKindTag(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return ""
	}
	return "kind:" + semanticTagSlug(kind)
}

func semanticSymbolRoleTags(sym Symbol, incoming, outgoing int) []string {
	var tags []string
	if isExportedSymbol(sym) {
		tags = append(tags, "graph:entrypoint")
	}
	if incoming >= 3 {
		tags = append(tags, "graph:fan-in")
	}
	if outgoing >= 3 {
		tags = append(tags, "graph:fan-out")
	}
	nameText := strings.ToLower(sym.Name + " " + sym.QualifiedName + " " + sym.Kind)
	tags = append(tags, semanticRoleTags(nameText)...)
	return tags
}

func semanticRoleTags(text string) []string {
	lower := strings.ToLower(text)
	rules := []struct {
		tag      string
		keywords []string
	}{
		{"role:watch", []string{"watch", "watcher", "scan", "scanner", "represent", "materializ", "embedding"}},
		{"role:cli", []string{"cmd/", "/cmd/", "cli", "command", "cobra"}},
		{"role:api", []string{"api", "http", "server", "handler", "route", "rpc", "websocket"}},
		{"role:persistence", []string{"store", "db", "database", "sqlite", "migration", "schema", "repository"}},
		{"role:ui", []string{"frontend", "component", "view", "react", "canvas", "zui"}},
		{"role:analysis", []string{"analyzer", "symbol", "parser", "importer", "planner", "dependency"}},
		{"role:versioning", []string{"git", "version", "history", "commit", "branch"}},
		{"role:config", []string{"config", "setting", "option", "threshold"}},
		{"role:test", []string{"test", "_test.go", "fixture", "mock"}},
	}
	var tags []string
	for _, rule := range rules {
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				tags = append(tags, rule.tag)
				break
			}
		}
	}
	return tags
}

func semanticTagSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func semanticTagPriority(tag string) int {
	switch {
	case strings.HasPrefix(tag, "role:"):
		return 0
	case strings.HasPrefix(tag, "framework:"):
		return 1
	case strings.HasPrefix(tag, "technology:"):
		return 1
	case tag == "http:route" || tag == "frontend:route" || strings.HasPrefix(tag, "orm:"):
		return 1
	case strings.HasPrefix(tag, "area:"):
		return 2
	case strings.HasPrefix(tag, "kind:"):
		return 3
	case strings.HasPrefix(tag, "graph:"):
		return 4
	case strings.HasPrefix(tag, "lang:"):
		return 5
	case strings.HasPrefix(tag, "owner:"):
		return 6
	default:
		return 7
	}
}

func (r *Representer) materialize(ctx context.Context, repo Repository, filtered filterResult, thresholds Thresholds, settingsHash string, identityKeys map[string]string, ownerMatcher *codeowners.Matcher) (materializeStats, error) {
	initialLayout, err := r.Store.RepositoryMaterializationCount(ctx, repo.ID)
	if err != nil {
		return materializeStats{}, err
	}
	facts, err := r.Store.FactsForRepository(ctx, repo.ID)
	if err != nil {
		return materializeStats{}, err
	}
	tagPlan := buildSemanticTagPlan(repo, filtered, thresholds, settingsHash, identityKeys, ownerMatcher, facts)
	symbolLanguages := newSymbolLanguageIndex(filtered.VisibleSymbols)
	m := &materializer{store: r.Store, repo: repo, thresholds: thresholds, settingsHash: settingsHash, identityKeys: identityKeys, contextPolicies: filtered.ContextPolicies, tagPlan: tagPlan, symbolLanguages: symbolLanguages, initialLayout: initialLayout == 0, runMarker: time.Now().UTC().Format(time.RFC3339Nano), newPlacements: map[int64]map[int64]struct{}{}}
	if err := m.ensureTags(ctx); err != nil {
		return m.stats, err
	}
	rootViewID, err := m.workspaceRootViewID(ctx)
	if err != nil {
		return m.stats, err
	}
	repoLanguage := symbolLanguages.repoLanguage
	repoElem, err := m.upsertElement(ctx, "repository", fmt.Sprintf("repository:%d", repo.ID), elementInput{
		Name:       repo.DisplayName,
		Kind:       "repository",
		Technology: technologyLabel(repoLanguage),
		Repo:       repoIdentity(repo),
		Branch:     nullStringValue(repo.Branch),
		Language:   repoLanguage,
		Tags:       tagPlan.tagsFor("repository", fmt.Sprintf("repository:%d", repo.ID)),
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

	architectureView, structuralView, err := m.materializeRepositorySections(ctx, repoView, repoLanguage)
	if err != nil {
		return m.stats, err
	}

	architecture := pruneDisconnectedArchitecture(canonicalizeArchitecture(mergeArchitectureModels(inferArchitecture(repo.RepoRoot), architectureFromFacts(facts))))

	visibleFiles := filesForSymbols(filtered.VisibleSymbols)
	for file := range filtered.VisibleFiles {
		visibleFiles[file] = struct{}{}
	}
	for file := range filtered.ChangedFiles {
		visibleFiles[file] = struct{}{}
	}
	for file := range filtered.BlastRadiusFiles {
		visibleFiles[file] = struct{}{}
	}
	folders := folderSet(visibleFiles)
	folderElements := map[string]int64{}
	folderViews := map[string]int64{}
	for _, folder := range folders {
		parentView := structuralView
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
			Tags:       tagPlan.tagsFor("folder", "folder:"+folder),
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
		folderElements[folder] = elem
		folderViews[folder] = view
	}

	fileElements := map[string]int64{}
	fileViews := map[string]int64{}
	fileLanguages, err := m.store.FileLanguages(ctx, repo.ID)
	if err != nil {
		return m.stats, err
	}
	for i, file := range sortedKeys(visibleFiles) {
		fileLanguage := m.languageForFile(file)
		if language := strings.TrimSpace(fileLanguages[file]); language != "" {
			fileLanguage = language
		}
		parentView := structuralView
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
			Tags:       tagPlan.tagsFor("file", "file:"+file),
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
	symbolPositions := map[int64]layoutPoint{}
	symbolDisplayNames := collidingSymbolDisplayNames(filtered.VisibleSymbols)
	occupied := map[int64]map[string]struct{}{}
	detailedSymbols := len(filtered.VisibleSymbols) <= maxDetailedSymbolElements
	for file, symbols := range symbolsByFile(filtered.VisibleSymbols) {
		fileView := fileViews[file]
		if fileView == 0 {
			continue
		}
		chunks := chunkSymbols(symbols, effectiveMaxElementsPerView(thresholds, filtered.Visibility, filtered.ContextExpansions.fileTier(file)))
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
					Tags:       tagPlan.tagsFor("cluster", clusterKey),
				})
				if err != nil {
					return m.stats, err
				}
				x, y := gridPosition(chunkIndex)
				if err := m.upsertPlacement(ctx, fileView, clusterElem, x, y); err != nil {
					return m.stats, err
				}
				markOccupied(occupied, fileView, layoutPoint{X: x, Y: y})
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
					Name:        symbolDisplayNames[sym.ID],
					Kind:        sym.Kind,
					Description: fmt.Sprintf("%s:%d", sym.FilePath, sym.StartLine),
					Technology:  technologyLabel(language),
					Repo:        repoIdentity(repo),
					Branch:      nullStringValue(repo.Branch),
					FilePath:    sym.FilePath,
					Language:    language,
					Tags:        tagPlan.tagsFor("symbol", symbolOwnerKey(sym, m.identityKeys)),
				})
				if err != nil {
					return m.stats, err
				}
				x, y := gridPosition(i)
				if err := m.upsertPlacement(ctx, targetView, elem, x, y); err != nil {
					return m.stats, err
				}
				point := layoutPoint{X: x, Y: y}
				markOccupied(occupied, targetView, point)
				symbolElements[sym.ID] = elem
				symbolViews[sym.ID] = targetView
				symbolPositions[sym.ID] = point
			}
		}
	}
	if err := m.materializeFacts(ctx, filtered.VisibleFacts, filtered.VisibleSymbols, structuralView, fileElements, fileViews, symbolElements, symbolViews, symbolPositions, occupied, filtered); err != nil {
		return m.stats, err
	}

	if err := m.materializeConnectors(ctx, filtered.VisibleReferences, filtered.VisibleSymbols, folderElements, folderViews, fileElements, symbolElements, symbolViews, structuralView); err != nil {
		return m.stats, err
	}
	if len(architecture.Components) > 0 {
		if err := m.materializeArchitecture(ctx, architecture, architectureView); err != nil {
			return m.stats, err
		}
	}
	if err := m.pruneStaleResources(ctx); err != nil {
		return m.stats, err
	}
	if err := m.layoutPlacements(ctx); err != nil {
		return m.stats, err
	}
	return m.stats, nil
}

func (m *materializer) materializeArchitecture(ctx context.Context, architecture architectureModel, repoView int64) error {
	componentElements := map[string]int64{}
	for i, component := range sortedArchitectureComponents(architecture.Components) {
		tags := appendUnique(component.Tags, "view:architecture")
		elem, err := m.upsertElement(ctx, "architecture-component", component.Key, elementInput{
			Name:        component.Name,
			Kind:        component.Kind,
			Description: component.Description,
			Technology:  firstNonEmpty(component.Technology, "Runtime"),
			Repo:        repoIdentity(m.repo),
			Branch:      nullStringValue(m.repo.Branch),
			FilePath:    component.FilePath,
			Tags:        tags,
		})
		if err != nil {
			return err
		}
		x, y := gridPosition(i)
		if err := m.upsertPlacement(ctx, repoView, elem, x, y); err != nil {
			return err
		}
		componentElements[component.Key] = elem
	}

	for _, connector := range sortedArchitectureConnectors(architecture.Connectors) {
		sourceID := componentElements[connector.SourceKey]
		targetID := componentElements[connector.TargetKey]
		if sourceID == 0 || targetID == 0 {
			continue
		}
		if err := m.upsertConnectorDetailedWithDirection(ctx, "architecture-connector", connector.Key, repoView, sourceID, targetID, connector.Label, connector.Relationship, connector.Direction, ""); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) materializeRepositorySections(ctx context.Context, repoView int64, repoLanguage string) (int64, int64, error) {
	structuralElem, err := m.upsertElement(ctx, "repository-section", fmt.Sprintf("repository-structural:%d", m.repo.ID), elementInput{
		Name:        "Structural",
		Kind:        "view",
		Description: "Generated structural code view",
		Technology:  "Structural",
		Repo:        repoIdentity(m.repo),
		Branch:      nullStringValue(m.repo.Branch),
		Language:    repoLanguage,
		Tags:        []string{"view:structural"},
	})
	if err != nil {
		return 0, 0, err
	}
	if err := m.upsertPlacement(ctx, repoView, structuralElem, 0, 0); err != nil {
		return 0, 0, err
	}
	structuralView, err := m.upsertView(ctx, "repository-section", fmt.Sprintf("repository-structural:%d", m.repo.ID), structuralElem, m.repo.DisplayName+" Structural", "Structural")
	if err != nil {
		return 0, 0, err
	}

	architectureElem, err := m.upsertElement(ctx, "repository-section", fmt.Sprintf("repository-architecture:%d", m.repo.ID), elementInput{
		Name:        "Architecture",
		Kind:        "view",
		Description: "Generated architecture view",
		Technology:  "Architecture",
		Repo:        repoIdentity(m.repo),
		Branch:      nullStringValue(m.repo.Branch),
		Language:    repoLanguage,
		Tags:        []string{"view:architecture"},
	})
	if err != nil {
		return 0, 0, err
	}
	if err := m.upsertPlacement(ctx, structuralView, architectureElem, 0, 0); err != nil {
		return 0, 0, err
	}
	architectureView, err := m.upsertView(ctx, "repository-section", fmt.Sprintf("repository-architecture:%d", m.repo.ID), architectureElem, m.repo.DisplayName+" Architecture", "Architecture")
	if err != nil {
		return 0, 0, err
	}

	return architectureView, structuralView, nil
}

func sortedArchitectureComponents(values map[string]*architectureComponent) []*architectureComponent {
	out := make([]*architectureComponent, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return architectureKindRank(out[i].Kind) < architectureKindRank(out[j].Kind)
	})
	return out
}

func sortedArchitectureConnectors(values map[string]*architectureConnector) []*architectureConnector {
	out := make([]*architectureConnector, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func architectureKindRank(kind string) int {
	switch kind {
	case "external":
		return 0
	case "service":
		return 1
	case "interface":
		return 2
	case "datastore":
		return 3
	case "queue":
		return 4
	default:
		return 5
	}
}

type materializer struct {
	store           *Store
	repo            Repository
	thresholds      Thresholds
	settingsHash    string
	identityKeys    map[string]string
	contextPolicies contextPolicySet
	tagPlan         semanticTagPlan
	symbolLanguages symbolLanguageIndex
	initialLayout   bool
	runMarker       string
	newPlacements   map[int64]map[int64]struct{}
	stats           materializeStats
}

type layoutPoint struct {
	X float64
	Y float64
}

type factPlacement struct {
	Point        layoutPoint
	SourceHandle string
	TargetHandle string
}

func collidingSymbolDisplayNames(symbols map[int64]Symbol) map[int64]string {
	counts := map[string]int{}
	for _, sym := range symbols {
		counts[sym.QualifiedName]++
	}
	out := map[int64]string{}
	nameCounts := map[string]int{}
	for id, sym := range symbols {
		name := sym.QualifiedName
		if counts[name] > 1 {
			if file := symbolFileQualifier(sym.FilePath); file != "" {
				name = file + "." + name
			}
		}
		out[id] = name
		nameCounts[name]++
	}
	for id, sym := range symbols {
		if nameCounts[out[id]] <= 1 || counts[sym.QualifiedName] <= 1 {
			continue
		}
		if file := symbolPathQualifier(sym.FilePath); file != "" {
			out[id] = file + "." + sym.QualifiedName
		}
	}
	return out
}

func symbolFileQualifier(filePath string) string {
	base := strings.TrimSuffix(path.Base(filepathToSlash(filePath)), path.Ext(filePath))
	return strings.TrimSpace(base)
}

func symbolPathQualifier(filePath string) string {
	clean := filepathToSlash(strings.TrimSuffix(filePath, path.Ext(filePath)))
	clean = strings.Trim(clean, "/.")
	return strings.ReplaceAll(clean, "/", ".")
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
	Tags        []string
}

type materializedTechnologyLink struct {
	Type          string `json:"type"`
	Slug          string `json:"slug,omitempty"`
	Label         string `json:"label"`
	IsPrimaryIcon bool   `json:"is_primary_icon,omitempty"`
}

func (m *materializer) ensureTags(ctx context.Context) error {
	return tagcolors.EnsureBun(ctx, m.store.bun, m.tagPlan.approvedTags())
}

func (m *materializer) workspaceRootViewID(ctx context.Context) (int64, error) {
	var id int64
	err := m.store.rowRaw(ctx, `SELECT id FROM views WHERE owner_element_id IS NULL ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

func (m *materializer) upsertElement(ctx context.Context, ownerType, ownerKey string, input elementInput) (int64, error) {
	input.Technology, input.Tags = extractTechnologyFromTags(input.Technology, input.Tags)
	if state, ok, err := m.store.MappingState(ctx, m.repo.ID, ownerType, ownerKey, "element"); err != nil {
		return 0, err
	} else if ok && elementExists(ctx, m.store.db, state.ResourceID) {
		dirty, err := m.mappingDirty(ctx, ownerType, ownerKey, "element", state)
		if err != nil {
			return 0, err
		}
		if dirty {
			m.stats.ElementsPreserved++
			return state.ResourceID, m.saveMapping(ctx, ownerType, ownerKey, "element", state.ResourceID)
		}
		tags, _ := json.Marshal(input.Tags)
		techLinks, _ := json.Marshal(technologyLinksForElement(input.Technology, input.Language))
		_, err = m.store.execRaw(ctx, `
			UPDATE elements
			SET name = ?, kind = ?, description = ?, technology = ?, technology_connectors = ?, tags = ?, repo = ?, branch = ?, file_path = ?, language = ?, updated_at = ?
			WHERE id = ?`,
			input.Name, nullString(input.Kind), nullString(input.Description), nullString(input.Technology), string(techLinks), string(tags),
			nullString(input.Repo), nullString(input.Branch), nullString(input.FilePath), nullString(input.Language), nowString(), state.ResourceID)
		if err != nil {
			return 0, err
		}
		if err := m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "element", state.ResourceID); err != nil {
			return 0, err
		}
		m.stats.ElementsUpdated++
		return state.ResourceID, nil
	}
	now := nowString()
	tags, _ := json.Marshal(input.Tags)
	techLinks, _ := json.Marshal(technologyLinksForElement(input.Technology, input.Language))
	id, err := m.store.insertReturningID(ctx, `
		INSERT INTO elements(name, kind, description, technology, technology_connectors, tags, repo, branch, file_path, language, bypass_noise_gate, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Name, nullString(input.Kind), nullString(input.Description), nullString(input.Technology), string(techLinks), string(tags),
		nullString(input.Repo), nullString(input.Branch), nullString(input.FilePath), nullString(input.Language), true, now, now)
	if err != nil {
		return 0, err
	}
	if err := m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "element", id); err != nil {
		return 0, err
	}
	m.stats.ElementsCreated++
	return id, nil
}

func (m *materializer) upsertView(ctx context.Context, ownerType, ownerKey string, ownerElementID int64, name, label string) (int64, error) {
	if state, ok, err := m.store.MappingState(ctx, m.repo.ID, ownerType, ownerKey, "view"); err != nil {
		return 0, err
	} else if ok && viewExists(ctx, m.store.db, state.ResourceID) {
		dirty, err := m.mappingDirty(ctx, ownerType, ownerKey, "view", state)
		if err != nil {
			return 0, err
		}
		if dirty {
			m.stats.ViewsPreserved++
			return state.ResourceID, m.saveMapping(ctx, ownerType, ownerKey, "view", state.ResourceID)
		}
		if _, err := m.store.execRaw(ctx, `UPDATE views SET owner_element_id = ?, name = ?, level_label = ?, updated_at = ? WHERE id = ?`, ownerElementID, name, label, nowString(), state.ResourceID); err != nil {
			return 0, err
		}
		return state.ResourceID, m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "view", state.ResourceID)
	}
	now := nowString()
	id, err := m.store.insertReturningID(ctx, `INSERT INTO views(owner_element_id, name, level_label, level, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`, ownerElementID, name, label, now, now)
	if err != nil {
		return 0, err
	}
	if err := m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "view", id); err != nil {
		return 0, err
	}
	m.stats.ViewsCreated++
	return id, nil
}

func (m *materializer) upsertPlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	var existingID int64
	err := m.store.rowRaw(ctx, `SELECT id FROM placements WHERE view_id = ? AND element_id = ?`, viewID, elementID).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = m.store.rowRaw(ctx, `
		SELECT p.id
		FROM placements p
		JOIN watch_materialization wm
		  ON wm.repository_id = ? AND wm.resource_type = 'element' AND wm.resource_id = p.element_id
		WHERE p.element_id = ?
		ORDER BY p.id
		LIMIT 1`, m.repo.ID, elementID).Scan(&existingID)
	if err == nil {
		_, err = m.store.execRaw(ctx, `UPDATE placements SET view_id = ?, position_x = ?, position_y = ?, updated_at = ? WHERE id = ?`, viewID, x, y, nowString(), existingID)
		if err == nil {
			m.markNewPlacement(viewID, elementID)
		}
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := nowString()
	_, err = m.store.execRaw(ctx, `
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
	watchLayoutGapX             = layout.PlacementGapX
	watchLayoutGapY             = layout.PlacementGapY
	watchLayoutMaxRowsPerColumn = layout.PlacementMaxRowsPerColumn
)

type watchPlacementNode = layout.Placement
type watchLayoutConnector = layout.Connector

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
	rows, err := m.store.rowsRaw(ctx, `
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
	next := layout.LayoutPlacements(placements, targets, connectors, force)
	for _, elementID := range layout.SortedInt64Set(targets) {
		pos := next[elementID]
		if _, err := m.store.execRaw(ctx, `UPDATE placements SET position_x = ?, position_y = ?, updated_at = ? WHERE view_id = ? AND element_id = ?`, pos.X, pos.Y, nowString(), viewID, elementID); err != nil {
			return err
		}
	}
	return nil
}

func (m *materializer) viewPlacementNodes(ctx context.Context, viewID int64) ([]watchPlacementNode, error) {
	rows, err := m.store.rowsRaw(ctx, `SELECT element_id, position_x, position_y FROM placements WHERE view_id = ? ORDER BY id`, viewID)
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
	rows, err := m.store.rowsRaw(ctx, `SELECT source_element_id, target_element_id FROM connectors WHERE view_id = ? ORDER BY id`, viewID)
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

type viewPairReference struct {
	Key             string
	OwnerKey        string
	ViewID          int64
	SourceElementID int64
	TargetElementID int64
	Forward         bool
	Backward        bool
	Ref             Reference
	Count           int
}

func (p *viewPairReference) addDirection(sourceElementID, targetElementID int64) {
	if p == nil {
		return
	}
	if sourceElementID == p.SourceElementID && targetElementID == p.TargetElementID {
		p.Forward = true
		return
	}
	if sourceElementID == p.TargetElementID && targetElementID == p.SourceElementID {
		p.Backward = true
	}
}

func (p viewPairReference) direction() string {
	switch {
	case p.Forward && p.Backward:
		return "both"
	case p.Backward:
		return "backward"
	default:
		return "forward"
	}
}

func (m *materializer) materializeFacts(ctx context.Context, facts []Fact, symbols map[int64]Symbol, structuralView int64, fileElements map[string]int64, fileViews map[string]int64, symbolElements map[int64]int64, symbolViews map[int64]int64, symbolPositions map[int64]layoutPoint, occupied map[int64]map[string]struct{}, filtered filterResult) error {
	if len(facts) == 0 {
		return nil
	}
	symbolIDByStable := map[string]int64{}
	for id, sym := range symbols {
		symbolIDByStable[sym.StableKey] = id
	}
	nodeFactsByFile := map[string][]Fact{}
	summaryFactsByFile := map[string][]Fact{}
	dependencyImportFactsByFile := map[string][]Fact{}
	connectionFactsByFile := map[string][]Fact{}
	metadataFactsByFile := map[string][]Fact{}
	for _, fact := range facts {
		if strings.TrimSpace(fact.FilePath) == "" || fileViews[fact.FilePath] == 0 {
			continue
		}
		if technologyMetadataFact(fact) {
			metadataFactsByFile[fact.FilePath] = append(metadataFactsByFile[fact.FilePath], fact)
			continue
		}
		if dependencyImportFact(fact) {
			dependencyImportFactsByFile[fact.FilePath] = append(dependencyImportFactsByFile[fact.FilePath], fact)
			continue
		}
		if runtimeConnectionFact(fact) {
			connectionFactsByFile[fact.FilePath] = append(connectionFactsByFile[fact.FilePath], fact)
			continue
		}
		highSignal := highSignalFact(fact)
		if highSignal && standaloneFactNode(fact) {
			nodeFactsByFile[fact.FilePath] = append(nodeFactsByFile[fact.FilePath], fact)
		} else if !highSignal {
			summaryFactsByFile[fact.FilePath] = append(summaryFactsByFile[fact.FilePath], fact)
		}
	}
	fileSet := map[string]struct{}{}
	for file := range nodeFactsByFile {
		fileSet[file] = struct{}{}
	}
	for file := range summaryFactsByFile {
		fileSet[file] = struct{}{}
	}
	for file := range dependencyImportFactsByFile {
		fileSet[file] = struct{}{}
	}
	for file := range connectionFactsByFile {
		fileSet[file] = struct{}{}
	}
	changedFactLines := changedLineSetForFacts(m.repo.RepoRoot, filtered.ChangedFiles)
	componentFactsByFile := runtimeComponentFactsByFile(facts)
	componentElementsByName, err := m.materializeRuntimeComponents(ctx, facts, structuralView, fileViews, symbols, occupied)
	if err != nil {
		return err
	}
	componentElementsByFile := map[string]map[string]int64{}
	for file, factsByName := range componentFactsByFile {
		componentElementsByFile[file] = map[string]int64{}
		for name := range factsByName {
			if elem := componentElementsByName[name]; elem != 0 {
				componentElementsByFile[file][name] = elem
			}
		}
	}
	volumeFactsByFile := map[string][]Fact{}
	volumeElementsByFile := map[string]map[string]int64{}
	endpointFactsByFile := map[string][]Fact{}
	endpointElementsByFile := map[string]map[string]int64{}
	for _, file := range sortedKeys(fileSet) {
		items := nodeFactsByFile[file]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Type == items[j].Type {
				return factOwnerKey(items[i]) < factOwnerKey(items[j])
			}
			return items[i].Type < items[j].Type
		})
		limit := min(factNodeLimitForFile(m.thresholds, filtered.Visibility, filtered.ContextExpansions.fileTier(file)), len(items))
		subjectFactCounts := map[int64]int{}
		for i, fact := range items[:limit] {
			if runtimeComponentFact(fact) {
				continue
			}
			elem, err := m.upsertElement(ctx, "fact", factOwnerKey(fact), elementInput{
				Name:        m.factNodeName(fact),
				Kind:        factNodeKind(fact),
				Description: factNodeDescription(fact),
				Technology:  factTechnology(fact),
				Repo:        repoIdentity(m.repo),
				Branch:      nullStringValue(m.repo.Branch),
				FilePath:    fact.FilePath,
				Language:    m.languageForFile(fact.FilePath),
				Tags:        m.tagPlan.tagsFor("fact", factOwnerKey(fact)),
			})
			if err != nil {
				return err
			}
			if storageVolumeFact(fact) {
				volumeFactsByFile[file] = append(volumeFactsByFile[file], fact)
				if volumeElementsByFile[file] == nil {
					volumeElementsByFile[file] = map[string]int64{}
				}
				volumeElementsByFile[file][factOwnerKey(fact)] = elem
			}
			if runtimeEndpointFact(fact) {
				endpointFactsByFile[file] = append(endpointFactsByFile[file], fact)
				if endpointElementsByFile[file] == nil {
					endpointElementsByFile[file] = map[string]int64{}
				}
				endpointElementsByFile[file][factOwnerKey(fact)] = elem
			}
			viewID := fileViews[file]
			var subjectID int64
			if fact.SubjectKind == "symbol" {
				subjectID = symbolIDByStable[fact.SubjectStableKey]
			}
			placement := nextFactPlacement(viewID, subjectID, subjectFactCounts[subjectID], symbolViews, symbolPositions, occupied, i)
			if subjectID != 0 {
				subjectFactCounts[subjectID]++
			}
			if err := m.upsertPlacement(ctx, viewID, elem, placement.Point.X, placement.Point.Y); err != nil {
				return err
			}
			markOccupied(occupied, viewID, placement.Point)
			if fact.SubjectKind == "symbol" {
				if symID := subjectID; symID != 0 && symbolElements[symID] != 0 && symbolViews[symID] == viewID {
					ownerKey := factOwnerKey(fact) + ":subject"
					label := firstNonEmpty(fact.Relationship, "declares")
					if err := m.upsertConnectorDetailed(ctx, "fact-reference", ownerKey, viewID, symbolElements[symID], elem, label, label, ""); err != nil {
						return err
					}
				}
			}
		}
		summaryFacts := append([]Fact(nil), summaryFactsByFile[file]...)
		if limit < len(items) {
			summaryFacts = append(summaryFacts, items[limit:]...)
		}
		var extractedFacts []Fact
		summaryFacts, extractedFacts = extractStandaloneSummaryFacts(summaryFacts, filtered.ChangedFiles, changedFactLines)
		for i, fact := range extractedFacts {
			if runtimeComponentFact(fact) || technologyMetadataFact(fact) {
				continue
			}
			elem, err := m.upsertElement(ctx, "fact", factOwnerKey(fact), elementInput{
				Name:        m.factNodeName(fact),
				Kind:        factNodeKind(fact),
				Description: factNodeDescription(fact),
				Technology:  factTechnology(fact),
				Repo:        repoIdentity(m.repo),
				Branch:      nullStringValue(m.repo.Branch),
				FilePath:    fact.FilePath,
				Language:    m.languageForFile(fact.FilePath),
				Tags:        m.tagPlan.tagsFor("fact", factOwnerKey(fact)),
			})
			if err != nil {
				return err
			}
			point := nextOpenGridPoint(fileViews[file], occupied, 500+i)
			if err := m.upsertPlacement(ctx, fileViews[file], elem, point.X, point.Y); err != nil {
				return err
			}
			markOccupied(occupied, fileViews[file], point)
		}
		if len(summaryFacts) > 0 {
			if err := m.materializeFactSummaries(ctx, file, fileViews[file], summaryFacts, occupied); err != nil {
				return err
			}
		}
	}
	if filtered.Dependencies.Enabled {
		if err := m.materializeDependencyImports(ctx, dependencyImportFactsByFile, structuralView, fileElements, occupied, filtered, changedFactLines); err != nil {
			return err
		}
	}
	if err := m.applyTechnologyMetadataFacts(ctx, metadataFactsByFile, fileElements, symbolElements, symbolIDByStable); err != nil {
		return err
	}
	if err := m.materializeRuntimeFactConnectors(ctx, connectionFactsByFile, componentFactsByFile, componentElementsByFile, fileViews); err != nil {
		return err
	}
	if err := m.materializeStorageVolumeConnectors(ctx, volumeFactsByFile, componentFactsByFile, componentElementsByFile, volumeElementsByFile, fileViews); err != nil {
		return err
	}
	if err := m.materializeRuntimeEndpointConnectors(ctx, endpointFactsByFile, componentFactsByFile, componentElementsByFile, endpointElementsByFile, fileViews); err != nil {
		return err
	}
	return nil
}

func (m *materializer) materializeDependencyImports(ctx context.Context, factsByFile map[string][]Fact, structuralView int64, fileElements map[string]int64, occupied map[int64]map[string]struct{}, filtered filterResult, changedLines map[string]map[int]struct{}) error {
	if len(factsByFile) == 0 || structuralView == 0 {
		return nil
	}
	repoModule := goModulePath(m.repo.RepoRoot)
	groupElements := map[string]int64{}
	groupIndex := 0
	moduleElements := map[string]int64{}
	moduleIndex := 0
	for _, file := range sortedKeysFromFactGroups(factsByFile) {
		facts := append([]Fact(nil), factsByFile[file]...)
		sort.SliceStable(facts, func(i, j int) bool {
			leftName, rightName := dependencyImportName(facts[i]), dependencyImportName(facts[j])
			if leftName == rightName {
				return factOwnerKey(facts[i]) < factOwnerKey(facts[j])
			}
			return leftName < rightName
		})
		connectedGroups := map[string]struct{}{}
		for _, fact := range facts {
			module := dependencyImportName(fact)
			if strings.TrimSpace(module) == "" {
				continue
			}
			fileElementID := fileElements[file]
			if fileElementID == 0 {
				continue
			}
			group := classifyDependencyImport(fact, repoModule)
			groupOwnerKey := dependencyGroupOwnerKey(group.Key)
			moduleOwnerKey := dependencyModuleOwnerKey(module)
			groupHidden := m.contextPolicyHidden("dependency-group", groupOwnerKey)
			groupExpanded := filtered.ContextExpansions.ownerTier("dependency-group", groupOwnerKey) > 0
			exact := factAffectedByDiff(fact, filtered.ChangedFiles, changedLines) ||
				groupExpanded ||
				m.contextPolicyShown("dependency-module", moduleOwnerKey)
			if groupHidden && !m.contextPolicyShown("dependency-module", moduleOwnerKey) {
				continue
			}
			if !groupHidden {
				elem := groupElements[group.Key]
				if elem == 0 {
					groupFacts := dependencyGroupFacts(factsByFile, repoModule, group.Key)
					var err error
					elem, err = m.upsertElement(ctx, "dependency-group", groupOwnerKey, elementInput{
						Name:        group.Name,
						Kind:        "dependency-group",
						Description: dependencyGroupDescription(group, len(groupFacts)),
						Technology:  group.Technology,
						Repo:        repoIdentity(m.repo),
						Branch:      nullStringValue(m.repo.Branch),
						Language:    m.languageForFile(fact.FilePath),
						Tags:        dependencyGroupTags(group),
					})
					if err != nil {
						return err
					}
					point := nextOpenGridPoint(structuralView, occupied, 650+groupIndex)
					groupIndex++
					if err := m.upsertPlacement(ctx, structuralView, elem, point.X, point.Y); err != nil {
						return err
					}
					markOccupied(occupied, structuralView, point)
					groupElements[group.Key] = elem
				}
			}
			if exact && !m.contextPolicyHidden("dependency-module", moduleOwnerKey) {
				elem := moduleElements[module]
				if elem == 0 {
					var err error
					elem, err = m.upsertElement(ctx, "dependency-module", moduleOwnerKey, elementInput{
						Name:        module,
						Kind:        "dependency",
						Description: "Imported dependency module",
						Technology:  dependencyImportTechnology(fact),
						Repo:        repoIdentity(m.repo),
						Branch:      nullStringValue(m.repo.Branch),
						Language:    m.languageForFile(fact.FilePath),
						Tags:        m.tagPlan.tagsFor("dependency-module", moduleOwnerKey),
					})
					if err != nil {
						return err
					}
					point := nextOpenGridPoint(structuralView, occupied, 700+moduleIndex)
					moduleIndex++
					if err := m.upsertPlacement(ctx, structuralView, elem, point.X, point.Y); err != nil {
						return err
					}
					markOccupied(occupied, structuralView, point)
					moduleElements[module] = elem
				}
				if err := m.upsertConnectorDetailed(ctx, "fact-import-connector", factOwnerKey(fact)+":module", structuralView, fileElementID, elem, "imports", firstNonEmpty(fact.Relationship, "imports"), factNodeDescription(fact)); err != nil {
					return err
				}
				continue
			}
			if groupHidden {
				continue
			}
			if _, ok := connectedGroups[group.Key]; ok {
				continue
			}
			elem := groupElements[group.Key]
			fileGroupFacts := dependencyGroupFactsForFile(facts, repoModule, group.Key, groupExpanded, filtered.ChangedFiles, changedLines, m)
			if len(fileGroupFacts) == 0 {
				continue
			}
			label := fmt.Sprintf("%d imports", len(fileGroupFacts))
			if len(fileGroupFacts) == 1 {
				label = "1 import"
			}
			connectorOwnerKey := dependencyGroupConnectorOwnerKey(file, group.Key)
			if err := m.upsertConnectorDetailed(ctx, "dependency-group-connector", connectorOwnerKey, structuralView, fileElementID, elem, label, "imports", dependencyGroupConnectorDescription(fileGroupFacts)); err != nil {
				return err
			}
			connectedGroups[group.Key] = struct{}{}
		}
	}
	return nil
}

func dependencyModuleOwnerKey(module string) string {
	return "dependency.module:" + strings.TrimSpace(module)
}

type dependencyImportGroup struct {
	Key        string
	Name       string
	Technology string
	Tag        string
}

func classifyDependencyImport(fact Fact, repoModule string) dependencyImportGroup {
	module := dependencyImportName(fact)
	if strings.HasPrefix(module, "./") || strings.HasPrefix(module, "../") {
		return dependencyImportGroup{Key: "relative", Name: "Relative imports", Technology: "Dependency", Tag: "dependency:relative"}
	}
	repoModule = strings.TrimSpace(repoModule)
	if repoModule != "" && (module == repoModule || strings.HasPrefix(module, repoModule+"/")) {
		return dependencyImportGroup{Key: "internal", Name: "Internal packages", Technology: "Dependency", Tag: "dependency:internal"}
	}
	firstSegment := module
	if before, _, ok := strings.Cut(module, "/"); ok {
		firstSegment = before
	}
	if m := strings.TrimSpace(module); m != "" && !strings.Contains(firstSegment, ".") && !strings.HasPrefix(module, "@") && strings.HasSuffix(filepathToSlash(fact.FilePath), ".go") {
		return dependencyImportGroup{Key: "go-stdlib", Name: "Go standard library", Technology: "Go", Tag: "dependency:go-stdlib"}
	}
	return dependencyImportGroup{Key: "external", Name: "External packages", Technology: "Dependency", Tag: "dependency:external"}
}

func dependencyGroupOwnerKey(key string) string {
	return "dependency.group:" + strings.TrimSpace(key)
}

func dependencyGroupConnectorOwnerKey(file, groupKey string) string {
	return "dependency.group.connector:" + filepathToSlash(file) + ":" + strings.TrimSpace(groupKey)
}

func dependencyGroupTags(group dependencyImportGroup) []string {
	return []string{"role:dependency", "dependency:group", group.Tag}
}

func dependencyGroupDescription(group dependencyImportGroup, count int) string {
	label := "imports"
	if count == 1 {
		label = "import"
	}
	return fmt.Sprintf("%d %s in %s", count, label, group.Name)
}

func dependencyGroupConnectorDescription(facts []Fact) string {
	modules := map[string]struct{}{}
	for _, fact := range facts {
		module := strings.TrimSpace(dependencyImportName(fact))
		if module == "" {
			continue
		}
		modules[module] = struct{}{}
	}
	names := sortedKeys(modules)
	if len(names) > 6 {
		names = append(names[:6], fmt.Sprintf("+%d more", len(modules)-6))
	}
	return strings.Join(names, ", ")
}

func dependencyGroupFacts(factsByFile map[string][]Fact, repoModule, groupKey string) []Fact {
	out := []Fact{}
	for _, facts := range factsByFile {
		for _, fact := range facts {
			if classifyDependencyImport(fact, repoModule).Key == groupKey {
				out = append(out, fact)
			}
		}
	}
	return out
}

func dependencyGroupFactsForFile(facts []Fact, repoModule, groupKey string, groupExpanded bool, changedFiles map[string]struct{}, changedLines map[string]map[int]struct{}, m *materializer) []Fact {
	out := []Fact{}
	for _, fact := range facts {
		if classifyDependencyImport(fact, repoModule).Key != groupKey {
			continue
		}
		moduleOwnerKey := dependencyModuleOwnerKey(dependencyImportName(fact))
		exact := factAffectedByDiff(fact, changedFiles, changedLines) ||
			groupExpanded ||
			m.contextPolicyShown("dependency-module", moduleOwnerKey)
		if exact && !m.contextPolicyHidden("dependency-module", moduleOwnerKey) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func goModulePath(repoRoot string) string {
	source, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func (m *materializer) materializeRuntimeComponents(ctx context.Context, facts []Fact, structuralView int64, fileViews map[string]int64, symbols map[int64]Symbol, occupied map[int64]map[string]struct{}) (map[string]int64, error) {
	grouped := runtimeComponentFactsByName(facts)
	out := map[string]int64{}
	index := 0
	for _, name := range sortedKeys(grouped) {
		items := grouped[name]
		if len(items) == 0 {
			continue
		}
		primary := runtimePrimaryFact(items)
		ownerKey := runtimeComponentOwnerKey(name)
		elem, err := m.upsertElement(ctx, "runtime-component", ownerKey, elementInput{
			Name:        name,
			Kind:        factNodeKind(primary),
			Description: runtimeComponentDescription(items),
			Technology:  factTechnology(primary),
			Repo:        repoIdentity(m.repo),
			Branch:      nullStringValue(m.repo.Branch),
			FilePath:    primary.FilePath,
			Language:    m.languageForFile(primary.FilePath),
			Tags:        m.tagPlan.tagsFor("runtime-component", ownerKey),
		})
		if err != nil {
			return nil, err
		}
		point := nextOpenGridPoint(structuralView, occupied, 900+index)
		index++
		if err := m.upsertPlacement(ctx, structuralView, elem, point.X, point.Y); err != nil {
			return nil, err
		}
		markOccupied(occupied, structuralView, point)
		for _, fact := range items {
			viewID := fileViews[fact.FilePath]
			if viewID == 0 {
				continue
			}
			point := nextOpenGridPoint(viewID, occupied, 900+index)
			if err := m.upsertPlacement(ctx, viewID, elem, point.X, point.Y); err != nil {
				return nil, err
			}
			markOccupied(occupied, viewID, point)
		}
		out[name] = elem
	}
	return out, nil
}

func runtimeComponentFactsByName(facts []Fact) map[string][]Fact {
	out := map[string][]Fact{}
	for _, fact := range facts {
		if !runtimeComponentFact(fact) {
			continue
		}
		name := runtimeComponentName(fact)
		if name == "" {
			continue
		}
		out[name] = append(out[name], fact)
	}
	return out
}

func runtimePrimaryFact(facts []Fact) Fact {
	sort.SliceStable(facts, func(i, j int) bool {
		return factOwnerKey(facts[i]) < factOwnerKey(facts[j])
	})
	return facts[0]
}

func runtimeComponentName(fact Fact) string {
	attrs := factAttributes(fact)
	return normalizeFactEndpoint(firstNonEmpty(attrs["name"], fact.Name, fact.ObjectName))
}

func runtimeComponentOwnerKey(name string) string {
	return "runtime.component:" + strings.TrimSpace(name)
}

func runtimeComponentDescription(facts []Fact) string {
	var evidence []string
	for _, fact := range facts {
		if desc := factNodeDescription(fact); desc != "" {
			evidence = append(evidence, desc)
		}
	}
	sort.Strings(evidence)
	return strings.Join(evidence, "\n")
}

func (m *materializer) applyTechnologyMetadataFacts(ctx context.Context, factsByFile map[string][]Fact, fileElements map[string]int64, symbolElements map[int64]int64, symbolIDByStable map[string]int64) error {
	for _, file := range sortedKeysFromFactGroups(factsByFile) {
		for _, fact := range factsByFile[file] {
			targetID := int64(0)
			if fact.SubjectKind == "symbol" {
				targetID = symbolElements[symbolIDByStable[fact.SubjectStableKey]]
			}
			if targetID == 0 {
				targetID = fileElements[file]
			}
			if targetID == 0 {
				continue
			}
			if err := m.mergeElementTechnology(ctx, targetID, technologyMetadataLabel(fact)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *materializer) mergeElementTechnology(ctx context.Context, elementID int64, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	var current, language sql.NullString
	if err := m.store.rowRaw(ctx, `SELECT technology, language FROM elements WHERE id = ?`, elementID).Scan(&current, &language); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	technology := mergeTechnologyLabel(current.String, label)
	techLinks, _ := json.Marshal(technologyLinksForElement(technology, language.String))
	_, err := m.store.execRaw(ctx, `
		UPDATE elements
		SET technology = ?, technology_connectors = ?, updated_at = ?
		WHERE id = ?`, nullString(technology), string(techLinks), nowString(), elementID)
	return err
}

func mergeTechnologyLabel(current, next string) string {
	parts := technologyLabelParts(current)
	for _, part := range technologyLabelParts(next) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(part))
	}
	return strings.Join(out, " / ")
}

func technologyMetadataLabel(fact Fact) string {
	attrs := factAttributes(fact)
	switch fact.Type {
	case "orm.query":
		return firstNonEmpty(attrs["orm"], attrs["dependency"], fact.Name)
	case "cache.key", "auth.issuer":
		return firstNonEmpty(attrs["dependency"], fact.Name)
	default:
		return factTechnology(fact)
	}
}

func changedLineSetForFacts(repoRoot string, changedFiles map[string]struct{}) map[string]map[int]struct{} {
	if len(changedFiles) == 0 || strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	hunks, err := tldgit.LineHunksAgainstHead(repoRoot)
	if err != nil {
		return nil
	}
	out := map[string]map[int]struct{}{}
	for file := range changedFiles {
		for _, hunk := range hunks[filepathToSlash(file)] {
			for _, line := range hunk.AddedLines {
				if out[file] == nil {
					out[file] = map[int]struct{}{}
				}
				out[file][line] = struct{}{}
			}
		}
	}
	return out
}

func extractStandaloneSummaryFacts(facts []Fact, changedFiles map[string]struct{}, changedLines map[string]map[int]struct{}) ([]Fact, []Fact) {
	if len(facts) <= 1 {
		return nil, facts
	}
	remaining := make([]Fact, 0, len(facts))
	extracted := make([]Fact, 0)
	for _, fact := range facts {
		if factAffectedByDiff(fact, changedFiles, changedLines) {
			extracted = append(extracted, fact)
			continue
		}
		remaining = append(remaining, fact)
	}
	return remaining, extracted
}

func factAffectedByDiff(fact Fact, changedFiles map[string]struct{}, changedLines map[string]map[int]struct{}) bool {
	file := filepathToSlash(fact.FilePath)
	if file == "" {
		return false
	}
	if _, ok := changedFiles[file]; !ok {
		return false
	}
	if changedLines == nil {
		return true
	}
	lines, ok := changedLines[file]
	if !ok {
		return true
	}
	start := fact.StartLine
	if start <= 0 {
		return true
	}
	end := start
	if fact.EndLine != nil && *fact.EndLine >= start {
		end = *fact.EndLine
	}
	for line := start; line <= end; line++ {
		if _, ok := lines[line]; ok {
			return true
		}
	}
	return false
}

func (m *materializer) materializeRuntimeFactConnectors(ctx context.Context, connectionFactsByFile map[string][]Fact, componentFactsByFile map[string]map[string]Fact, componentElementsByFile map[string]map[string]int64, fileViews map[string]int64) error {
	for _, file := range sortedKeysFromRuntimeConnectionGroups(connectionFactsByFile) {
		for _, fact := range connectionFactsByFile[file] {
			attrs := factAttributes(fact)
			source := normalizeFactEndpoint(firstNonEmpty(attrs["source"], componentSourceForFact(fact, attrs)))
			target := normalizeFactEndpoint(firstNonEmpty(attrs["target"], fact.ObjectName))
			if source == "" || target == "" || source == target {
				continue
			}
			if componentElementsByFile[file] == nil {
				componentElementsByFile[file] = map[string]int64{}
			}
			sourceID := componentElementsByFile[file][source]
			targetID := componentElementsByFile[file][target]
			if sourceID == 0 {
				var err error
				sourceID, err = m.ensureRuntimeConnectorEndpoint(ctx, componentFactsByFile[file][source], fileViews[file])
				if err != nil {
					return err
				}
				if sourceID != 0 {
					componentElementsByFile[file][source] = sourceID
				}
			}
			if targetID == 0 {
				var err error
				targetID, err = m.ensureRuntimeConnectorEndpoint(ctx, componentFactsByFile[file][target], fileViews[file])
				if err != nil {
					return err
				}
				if targetID != 0 {
					componentElementsByFile[file][target] = targetID
				}
			}
			if sourceID == 0 || targetID == 0 {
				continue
			}
			label := firstNonEmpty(attrs["label"], fact.Relationship, "uses")
			relationship := firstNonEmpty(fact.Relationship, "runtime-dependency")
			if err := m.upsertConnectorDetailed(ctx, "fact-runtime-connection", factOwnerKey(fact), fileViews[file], sourceID, targetID, label, relationship, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *materializer) materializeStorageVolumeConnectors(ctx context.Context, volumeFactsByFile map[string][]Fact, componentFactsByFile map[string]map[string]Fact, componentElementsByFile map[string]map[string]int64, volumeElementsByFile map[string]map[string]int64, fileViews map[string]int64) error {
	for _, file := range sortedKeysFromFactGroups(volumeFactsByFile) {
		for _, fact := range volumeFactsByFile[file] {
			attrs := factAttributes(fact)
			service := normalizeFactEndpoint(attrs["service"])
			if service == "" {
				continue
			}
			if componentElementsByFile[file] == nil {
				componentElementsByFile[file] = map[string]int64{}
			}
			sourceID := componentElementsByFile[file][service]
			if sourceID == 0 {
				var err error
				sourceID, err = m.ensureRuntimeConnectorEndpoint(ctx, componentFactsByFile[file][service], fileViews[file])
				if err != nil {
					return err
				}
				if sourceID != 0 {
					componentElementsByFile[file][service] = sourceID
				}
			}
			targetID := volumeElementsByFile[file][factOwnerKey(fact)]
			if sourceID == 0 || targetID == 0 {
				continue
			}
			description := ""
			if target := strings.TrimSpace(attrs["target"]); target != "" {
				description = "Mounted at " + target
			}
			if err := m.upsertConnectorDetailed(ctx, "fact-storage-volume", factOwnerKey(fact), fileViews[file], sourceID, targetID, "mounts", firstNonEmpty(fact.Relationship, "uses"), description); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *materializer) materializeRuntimeEndpointConnectors(ctx context.Context, endpointFactsByFile map[string][]Fact, componentFactsByFile map[string]map[string]Fact, componentElementsByFile map[string]map[string]int64, endpointElementsByFile map[string]map[string]int64, fileViews map[string]int64) error {
	for _, file := range sortedKeysFromFactGroups(endpointFactsByFile) {
		for _, fact := range endpointFactsByFile[file] {
			attrs := factAttributes(fact)
			service := normalizeFactEndpoint(attrs["service"])
			if service == "" {
				continue
			}
			if componentElementsByFile[file] == nil {
				componentElementsByFile[file] = map[string]int64{}
			}
			sourceID := componentElementsByFile[file][service]
			if sourceID == 0 {
				var err error
				sourceID, err = m.ensureRuntimeConnectorEndpoint(ctx, componentFactsByFile[file][service], fileViews[file])
				if err != nil {
					return err
				}
				if sourceID != 0 {
					componentElementsByFile[file][service] = sourceID
				}
			}
			targetID := endpointElementsByFile[file][factOwnerKey(fact)]
			if sourceID == 0 || targetID == 0 {
				continue
			}
			if err := m.upsertConnectorDetailed(ctx, "fact-runtime-endpoint", factOwnerKey(fact), fileViews[file], targetID, sourceID, endpointConnectorLabel(fact), firstNonEmpty(fact.Relationship, "exposes"), ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *materializer) ensureRuntimeConnectorEndpoint(ctx context.Context, fact Fact, viewID int64) (int64, error) {
	if fact.StableKey == "" || viewID == 0 {
		return 0, nil
	}
	state, ok, err := m.store.MappingState(ctx, m.repo.ID, "fact", factOwnerKey(fact), "element")
	if err != nil {
		return 0, err
	}
	if !ok || !elementExists(ctx, m.store.db, state.ResourceID) {
		return 0, nil
	}
	if _, err := m.store.execRaw(ctx, `
		INSERT INTO placements(view_id, element_id, x, y, created_at, updated_at)
		SELECT ?, ?, 0, 0, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM placements WHERE view_id = ? AND element_id = ?)`,
		viewID, state.ResourceID, nowString(), nowString(), viewID, state.ResourceID); err != nil {
		return 0, err
	}
	return state.ResourceID, nil
}

func runtimeComponentFactsByFile(facts []Fact) map[string]map[string]Fact {
	out := map[string]map[string]Fact{}
	for _, fact := range facts {
		if !runtimeComponentFact(fact) {
			continue
		}
		name := runtimeComponentName(fact)
		if name == "" {
			continue
		}
		if out[fact.FilePath] == nil {
			out[fact.FilePath] = map[string]Fact{}
		}
		out[fact.FilePath][name] = fact
	}
	return out
}

func runtimeComponentFact(fact Fact) bool {
	return fact.Type == "runtime.component"
}

func runtimeConnectionFact(fact Fact) bool {
	return fact.Type == "runtime.connection"
}

func storageVolumeFact(fact Fact) bool {
	return fact.Type == "storage.volume"
}

func dependencyImportFact(fact Fact) bool {
	return fact.Type == "dependency.import"
}

func technologyMetadataFact(fact Fact) bool {
	switch fact.Type {
	case "orm.query", "cache.key", "auth.issuer":
		return true
	default:
		return false
	}
}

func runtimeEndpointFact(fact Fact) bool {
	return fact.Type == "runtime.endpoint"
}

func standaloneFactNode(fact Fact) bool {
	switch fact.Type {
	case "http.route", "frontend.route", "runtime.endpoint", "storage.volume":
		return concreteFactObject(fact)
	default:
		return false
	}
}

func concreteFactObject(fact Fact) bool {
	return strings.TrimSpace(firstNonEmpty(fact.ObjectStableKey, fact.ObjectName, fact.Name)) != ""
}

func componentSourceForFact(fact Fact, attrs map[string]string) string {
	if source := strings.TrimSpace(attrs["source"]); source != "" {
		return source
	}
	return inferredComponentFromFact(fact, attrs)
}

func sortedKeysFromRuntimeConnectionGroups(groups map[string][]Fact) []string {
	return sortedKeysFromFactGroups(groups)
}

func sortedKeysFromFactGroups(groups map[string][]Fact) []string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func factNodeLimitForFile(thresholds Thresholds, visibility VisibilityConfig, tier int) int {
	limit := effectiveMaxElementsPerView(thresholds, visibility, tier) / 3
	if limit < 3 {
		return 3
	}
	return limit
}

func (m *materializer) materializeFactSummaries(ctx context.Context, file string, viewID int64, facts []Fact, occupied map[int64]map[string]struct{}) error {
	byType := map[string][]Fact{}
	for _, fact := range facts {
		byType[fact.Type] = append(byType[fact.Type], fact)
	}
	i := 0
	for _, factType := range sortedKeysFromFactSummaryGroups(byType) {
		items := byType[factType]
		ownerKey := "fact-summary:" + file + ":" + factType
		elem, err := m.upsertElement(ctx, "fact-summary", ownerKey, elementInput{
			Name:        fmt.Sprintf("%d %s", len(items), factSummaryLabel(factType, len(items))),
			Kind:        "summary",
			Description: fmt.Sprintf("%d omitted %s facts in %s", len(items), factType, file),
			Technology:  "Runtime",
			Repo:        repoIdentity(m.repo),
			Branch:      nullStringValue(m.repo.Branch),
			FilePath:    file,
			Tags:        summaryTagsForFacts(items),
		})
		if err != nil {
			return err
		}
		point := nextOpenGridPoint(viewID, occupied, 1000+i)
		if err := m.upsertPlacement(ctx, viewID, elem, point.X, point.Y); err != nil {
			return err
		}
		markOccupied(occupied, viewID, point)
		i++
	}
	return nil
}

func nextFactPlacement(viewID, subjectID int64, subjectIndex int, symbolViews map[int64]int64, symbolPositions map[int64]layoutPoint, occupied map[int64]map[string]struct{}, fallbackIndex int) factPlacement {
	if subjectID == 0 || symbolViews[subjectID] != viewID {
		point := nextOpenGridPoint(viewID, occupied, fallbackIndex)
		return factPlacement{Point: point, SourceHandle: "right", TargetHandle: "left"}
	}
	origin := symbolPositions[subjectID]
	candidates := factPlacementCandidates(origin, subjectIndex)
	for _, candidate := range candidates {
		if !isOccupied(occupied, viewID, candidate.Point) {
			return candidate
		}
	}
	point := nextOpenGridPoint(viewID, occupied, fallbackIndex)
	return factPlacement{Point: point, SourceHandle: "right", TargetHandle: "left"}
}

func factPlacementCandidates(origin layoutPoint, subjectIndex int) []factPlacement {
	ring := subjectIndex/8 + 1
	spread := float64((subjectIndex%3)-1) * 90
	dx := float64(ring) * watchLayoutGapX
	dy := float64(ring) * watchLayoutGapY
	return []factPlacement{
		{Point: layoutPoint{X: origin.X + dx, Y: origin.Y + spread}, SourceHandle: "right", TargetHandle: "left"},
		{Point: layoutPoint{X: origin.X, Y: origin.Y + dy + spread}, SourceHandle: "bottom", TargetHandle: "top"},
		{Point: layoutPoint{X: origin.X - dx, Y: origin.Y + spread}, SourceHandle: "left", TargetHandle: "right"},
		{Point: layoutPoint{X: origin.X, Y: origin.Y - dy + spread}, SourceHandle: "top", TargetHandle: "bottom"},
		{Point: layoutPoint{X: origin.X + dx, Y: origin.Y + dy}, SourceHandle: "right", TargetHandle: "left"},
		{Point: layoutPoint{X: origin.X - dx, Y: origin.Y + dy}, SourceHandle: "left", TargetHandle: "right"},
		{Point: layoutPoint{X: origin.X + dx, Y: origin.Y - dy}, SourceHandle: "right", TargetHandle: "left"},
		{Point: layoutPoint{X: origin.X - dx, Y: origin.Y - dy}, SourceHandle: "left", TargetHandle: "right"},
	}
}

func nextOpenGridPoint(viewID int64, occupied map[int64]map[string]struct{}, startIndex int) layoutPoint {
	for i := startIndex; ; i++ {
		x, y := gridPosition(i)
		point := layoutPoint{X: x, Y: y}
		if !isOccupied(occupied, viewID, point) {
			return point
		}
	}
}

func markOccupied(occupied map[int64]map[string]struct{}, viewID int64, point layoutPoint) {
	if occupied[viewID] == nil {
		occupied[viewID] = map[string]struct{}{}
	}
	occupied[viewID][layoutPointKey(point)] = struct{}{}
}

func isOccupied(occupied map[int64]map[string]struct{}, viewID int64, point layoutPoint) bool {
	if occupied[viewID] == nil {
		return false
	}
	_, ok := occupied[viewID][layoutPointKey(point)]
	return ok
}

func layoutPointKey(point layoutPoint) string {
	return fmt.Sprintf("%.0f:%.0f", point.X, point.Y)
}

func sortedKeysFromFactSummaryGroups(groups map[string][]Fact) []string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func factSummaryLabel(factType string, count int) string {
	label := factType
	switch factType {
	case "http.route", "frontend.route":
		label = "routes"
	case "dependency.import":
		label = "imports"
	case "orm.query":
		label = "data access facts"
	}
	if count == 1 {
		return strings.TrimSuffix(label, "s")
	}
	return label
}

func summaryTagsForFacts(facts []Fact) []string {
	set := map[string]struct{}{}
	for _, fact := range facts {
		for _, tag := range fact.Tags {
			tag = strings.TrimSpace(tag)
			if strings.HasPrefix(tag, "role:") {
				set[tag] = struct{}{}
			}
		}
	}
	return sortedKeys(set)
}

func (m *materializer) factNodeName(fact Fact) string {
	if dependencyImportFact(fact) {
		return dependencyImportName(fact)
	}
	if storageVolumeFact(fact) {
		return m.storageVolumeFactName(fact)
	}
	if runtimeEndpointFact(fact) {
		return endpointFactName(fact)
	}
	return firstNonEmpty(fact.Name, fact.ObjectName, fact.Type)
}

func (m *materializer) storageVolumeFactName(fact Fact) string {
	attrs := factAttributes(fact)
	name := firstNonEmpty(attrs["source"], fact.ObjectName, fact.Name, fact.Type)
	if strings.Contains(name, " -> ") {
		_, name, _ = strings.Cut(name, " -> ")
	}
	return m.relativeFactPath(name)
}

func endpointFactName(fact Fact) string {
	attrs := factAttributes(fact)
	port := strings.TrimSpace(attrs["port"])
	protocol := strings.TrimSpace(attrs["protocol"])
	if protocol == "" {
		protocol = "tcp"
	}
	if port != "" {
		return port + "/" + protocol
	}
	name := firstNonEmpty(fact.ObjectName, fact.Name, fact.Type)
	if strings.Contains(name, ":") {
		name = name[strings.LastIndex(name, ":")+1:]
	}
	return strings.TrimSpace(name)
}

func endpointConnectorLabel(fact Fact) string {
	attrs := factAttributes(fact)
	if published := strings.TrimSpace(attrs["published"]); published != "" {
		return ":" + published
	}
	if port := strings.TrimSpace(attrs["port"]); port != "" {
		return ":" + port
	}
	return "exposes"
}

func factNodeKind(fact Fact) string {
	switch fact.Type {
	case "dependency.import":
		return "dependency"
	case "http.route", "frontend.route":
		return "route"
	case "orm.query":
		return "data-access"
	case "runtime.component":
		attrs := map[string]string{}
		_ = json.Unmarshal([]byte(fact.AttributesJSON), &attrs)
		if kind := strings.TrimSpace(attrs["kind"]); kind != "" {
			return kind
		}
		return "service"
	case "runtime.connection":
		return "connection"
	case "storage.volume":
		return "volume"
	case "runtime.endpoint":
		return "endpoint"
	default:
		return "fact"
	}
}

func factTechnology(fact Fact) string {
	if dependencyImportFact(fact) {
		return dependencyImportTechnology(fact)
	}
	if storageVolumeFact(fact) {
		return "Folder"
	}
	if runtimeEndpointFact(fact) {
		return "Endpoint"
	}
	attrs := map[string]string{}
	_ = json.Unmarshal([]byte(fact.AttributesJSON), &attrs)
	if framework := strings.TrimSpace(attrs["framework"]); framework != "" {
		return framework
	}
	if orm := strings.TrimSpace(attrs["orm"]); orm != "" {
		return orm
	}
	if technology := strings.TrimSpace(attrs["technology"]); technology != "" {
		return technology
	}
	return "Runtime"
}

func dependencyImportName(fact Fact) string {
	attrs := factAttributes(fact)
	return firstNonEmpty(attrs["module"], fact.ObjectName, fact.Name, fact.Type)
}

func dependencyImportTechnology(fact Fact) string {
	attrs := factAttributes(fact)
	return firstNonEmpty(attrs["ecosystem"], attrs["language"], "Dependency")
}

func extractTechnologyFromTags(currentTechnology string, tags []string) (string, []string) {
	var filtered []string
	var extracted string
	for _, tag := range tags {
		if extracted == "" && strings.HasPrefix(tag, "technology:") {
			extracted = strings.TrimPrefix(tag, "technology:")
			continue
		}
		filtered = append(filtered, tag)
	}
	if extracted == "" {
		return currentTechnology, filtered
	}
	if currentTechnology == "" || currentTechnology == "Runtime" || currentTechnology == "Source" {
		return extracted, filtered
	}
	return currentTechnology, filtered
}

func (m *materializer) relativeFactPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.TrimSpace(m.repo.RepoRoot) == "" || !filepath.IsAbs(value) {
		return filepath.ToSlash(value)
	}
	if rel, err := filepath.Rel(m.repo.RepoRoot, value); err == nil && rel != "." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(value)
}

func factNodeDescription(fact Fact) string {
	parts := []string{fact.Type}
	if fact.Relationship != "" {
		parts = append(parts, fact.Relationship)
	}
	if fact.FilePath != "" && fact.StartLine > 0 {
		parts = append(parts, fmt.Sprintf("%s:%d", fact.FilePath, fact.StartLine))
	}
	return strings.Join(parts, " - ")
}

func (m *materializer) materializeConnectors(ctx context.Context, refs []Reference, symbols map[int64]Symbol, folderElements map[string]int64, folderViews map[string]int64, fileElements map[string]int64, symbolElements map[int64]int64, symbolViews map[int64]int64, repoView int64) error {
	filePairs := map[string]viewPairReference{}
	provenanceFilePairs := map[string]viewPairReference{}
	symbolPairs := map[string]viewPairReference{}
	for _, ref := range refs {
		source := symbols[ref.SourceSymbolID]
		target := symbols[ref.TargetSymbolID]
		if source.FilePath != "" && target.FilePath != "" && source.FilePath != target.FilePath {
			sourceFileElementID := fileElements[source.FilePath]
			targetFileElementID := fileElements[target.FilePath]
			if sourceFileElementID != 0 && targetFileElementID != 0 {
				provenanceKey := canonicalViewPairKey(repoView, sourceFileElementID, targetFileElementID, "file")
				provenancePair := provenanceFilePairs[provenanceKey]
				if provenancePair.Count == 0 {
					provenancePair = viewPairReference{Key: provenanceKey, OwnerKey: "file:" + source.FilePath + "->" + target.FilePath, ViewID: repoView, SourceElementID: sourceFileElementID, TargetElementID: targetFileElementID, Ref: ref}
				}
				provenancePair.addDirection(sourceFileElementID, targetFileElementID)
				provenancePair.Count++
				provenanceFilePairs[provenanceKey] = provenancePair
			}
			viewID, sourceElementID, targetElementID, ownerKey, ok := connectorViewEndpoints(source.FilePath, target.FilePath, folderElements, folderViews, fileElements, repoView)
			if !ok {
				continue
			}
			key := canonicalViewPairKey(viewID, sourceElementID, targetElementID, "file")
			pair := filePairs[key]
			if pair.Count == 0 {
				pair = viewPairReference{Key: key, OwnerKey: ownerKey, ViewID: viewID, SourceElementID: sourceElementID, TargetElementID: targetElementID, Ref: ref}
			}
			pair.addDirection(sourceElementID, targetElementID)
			pair.Count++
			filePairs[key] = pair
			continue
		}
		viewID := symbolViews[ref.SourceSymbolID]
		if viewID == 0 || viewID != symbolViews[ref.TargetSymbolID] {
			continue
		}
		sourceElementID := symbolElements[ref.SourceSymbolID]
		targetElementID := symbolElements[ref.TargetSymbolID]
		ownerKey := fmt.Sprintf("symbol:%s:%s:%s", symbolOwnerKey(source, m.identityKeys), symbolOwnerKey(target, m.identityKeys), ref.Kind)
		key := canonicalViewPairKey(viewID, sourceElementID, targetElementID, ref.Kind)
		pair := symbolPairs[key]
		if pair.Count == 0 {
			pair = viewPairReference{Key: key, OwnerKey: ownerKey, ViewID: viewID, SourceElementID: sourceElementID, TargetElementID: targetElementID, Ref: ref}
		}
		pair.addDirection(sourceElementID, targetElementID)
		pair.Count++
		symbolPairs[key] = pair
	}

	symbolConnectorCount := map[int64]int{}
	for _, key := range sortedKeys(symbolPairs) {
		pair := symbolPairs[key]
		if symbolConnectorCount[pair.ViewID] >= m.thresholds.MaxConnectorsPerView {
			continue
		}
		if m.contextPolicyHidden("reference", pair.OwnerKey) {
			continue
		}
		if err := m.upsertConnectorDetailedWithDirection(ctx, "reference", pair.OwnerKey, pair.ViewID, pair.SourceElementID, pair.TargetElementID, "calls", "calls", pair.direction(), ""); err != nil {
			return err
		}
		symbolConnectorCount[pair.ViewID]++
	}

	provenanceGroups := map[string][]viewPairReference{}
	for _, key := range sortedKeys(provenanceFilePairs) {
		pair := provenanceFilePairs[key]
		provenanceGroups[pair.OwnerKey] = append(provenanceGroups[pair.OwnerKey], pair)
	}

	fileConnectorCountByView := map[int64]int{}
	for _, groupKey := range sortedFileGroupKeys(provenanceGroups) {
		group := provenanceGroups[groupKey]
		if len(group) == 0 {
			continue
		}
		viewID := group[0].ViewID
		if fileConnectorCountByView[viewID] >= m.thresholds.MaxConnectorsPerView {
			continue
		}
		rawReferenceCount := filePairReferenceCount(group)
		if strings.HasPrefix(groupKey, "folder:") && rawReferenceCount > m.thresholds.MaxExpandedConnectorsPerGroup {
			if m.contextPolicyHidden("folder-reference", groupKey) {
				continue
			}
			first := group[0]
			if err := m.upsertConnectorDetailedWithDirection(ctx, "folder-reference", groupKey, first.ViewID, first.SourceElementID, first.TargetElementID, fmt.Sprintf("%d references", rawReferenceCount), fmt.Sprintf("%d references", rawReferenceCount), viewPairDirection(group), ""); err != nil {
				return err
			}
			fileConnectorCountByView[viewID]++
			continue
		}
		for _, item := range group {
			if fileConnectorCountByView[item.ViewID] >= m.thresholds.MaxConnectorsPerView {
				break
			}
			if m.contextPolicyHidden("file-reference", item.OwnerKey) {
				continue
			}
			if err := m.upsertConnectorDetailedWithDirection(ctx, "file-reference", item.OwnerKey, item.ViewID, item.SourceElementID, item.TargetElementID, "", "", viewPairDirection([]viewPairReference{item}), ""); err != nil {
				return err
			}
			fileConnectorCountByView[item.ViewID]++
		}
	}

	localFileGroups := map[string][]viewPairReference{}
	for _, key := range sortedKeys(filePairs) {
		pair := filePairs[key]
		localFileGroups[pair.OwnerKey] = append(localFileGroups[pair.OwnerKey], pair)
	}

	localConnectorCountByView := map[int64]int{}
	for _, groupKey := range sortedFileGroupKeys(localFileGroups) {
		group := localFileGroups[groupKey]
		if len(group) == 0 {
			continue
		}
		viewID := group[0].ViewID
		if localConnectorCountByView[viewID] >= m.thresholds.MaxConnectorsPerView {
			continue
		}
		rawReferenceCount := filePairReferenceCount(group)
		if strings.HasPrefix(groupKey, "folder:") && rawReferenceCount > m.thresholds.MaxExpandedConnectorsPerGroup {
			if m.contextPolicyHidden("view-folder-reference", groupKey) {
				continue
			}
			first := group[0]
			if err := m.upsertConnectorDetailedWithDirection(ctx, "view-folder-reference", groupKey, first.ViewID, first.SourceElementID, first.TargetElementID, fmt.Sprintf("%d references", rawReferenceCount), fmt.Sprintf("%d references", rawReferenceCount), viewPairDirection(group), ""); err != nil {
				return err
			}
			localConnectorCountByView[viewID]++
			continue
		}
		for _, item := range group {
			if localConnectorCountByView[item.ViewID] >= m.thresholds.MaxConnectorsPerView {
				break
			}
			if m.contextPolicyHidden("view-file-reference", item.OwnerKey) {
				continue
			}
			if err := m.upsertConnectorDetailedWithDirection(ctx, "view-file-reference", item.OwnerKey, item.ViewID, item.SourceElementID, item.TargetElementID, "", "", viewPairDirection([]viewPairReference{item}), ""); err != nil {
				return err
			}
			localConnectorCountByView[item.ViewID]++
		}
	}
	return nil
}

func (m *materializer) contextPolicyHidden(ownerType, ownerKey string) bool {
	_, hidden := m.contextPolicies.Hide[ownerMapKey(ownerType, ownerKey)]
	return hidden
}

func (m *materializer) contextPolicyShown(ownerType, ownerKey string) bool {
	_, shown := m.contextPolicies.Show[ownerMapKey(ownerType, ownerKey)]
	return shown
}

func filePairReferenceCount(group []viewPairReference) int {
	count := 0
	for _, item := range group {
		count += item.Count
	}
	return count
}

func viewPairDirection(group []viewPairReference) string {
	var forward, backward bool
	for _, item := range group {
		switch item.direction() {
		case "both":
			forward = true
			backward = true
		case "backward":
			backward = true
		default:
			forward = true
		}
	}
	switch {
	case forward && backward:
		return "both"
	case backward:
		return "backward"
	default:
		return "forward"
	}
}

func sortedFileGroupKeys(groups map[string][]viewPairReference) []string {
	keys := sortedKeys(groups)
	sort.SliceStable(keys, func(i, j int) bool {
		left := keys[i]
		right := keys[j]
		leftCross := strings.HasPrefix(left, "folder:")
		rightCross := strings.HasPrefix(right, "folder:")
		if leftCross != rightCross {
			return leftCross
		}
		leftCount := filePairReferenceCount(groups[left])
		rightCount := filePairReferenceCount(groups[right])
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return left < right
	})
	return keys
}

func connectorViewEndpoints(sourcePath, targetPath string, folderElements, folderViews map[string]int64, fileElements map[string]int64, repoView int64) (int64, int64, int64, string, bool) {
	common := commonConnectorDir(sourcePath, targetPath)
	viewID := repoView
	if common != "" {
		viewID = folderViews[common]
	}
	if viewID == 0 {
		return 0, 0, 0, "", false
	}
	sourceElementID, sourceKey := connectorChildElement(sourcePath, common, folderElements, fileElements)
	targetElementID, targetKey := connectorChildElement(targetPath, common, folderElements, fileElements)
	if sourceElementID == 0 || targetElementID == 0 || sourceElementID == targetElementID {
		return 0, 0, 0, "", false
	}
	ownerPrefix := "file"
	if sourceKey != sourcePath || targetKey != targetPath {
		ownerPrefix = "folder"
	}
	return viewID, sourceElementID, targetElementID, ownerPrefix + ":" + sourceKey + "->" + targetKey, true
}

func canonicalViewPairKey(viewID, sourceElementID, targetElementID int64, scope string) string {
	if sourceElementID > targetElementID {
		sourceElementID, targetElementID = targetElementID, sourceElementID
	}
	return fmt.Sprintf("%d:%d:%d:%s", viewID, sourceElementID, targetElementID, scope)
}

func connectorChildElement(filePath, commonDir string, folderElements map[string]int64, fileElements map[string]int64) (int64, string) {
	dir := path.Dir(filePath)
	if dir == "." {
		dir = ""
	}
	if dir == commonDir {
		return fileElements[filePath], filePath
	}
	child := firstPathChild(filePath, commonDir)
	if child == "" {
		return 0, ""
	}
	return folderElements[child], child
}

func firstPathChild(filePath, commonDir string) string {
	trimmed := filePath
	if commonDir != "" {
		prefix := strings.TrimSuffix(commonDir, "/") + "/"
		if !strings.HasPrefix(filePath, prefix) {
			return ""
		}
		trimmed = strings.TrimPrefix(filePath, prefix)
	}
	first, _, _ := strings.Cut(trimmed, "/")
	if first == "" || first == trimmed {
		return ""
	}
	if commonDir == "" {
		return first
	}
	return commonDir + "/" + first
}

func commonConnectorDir(leftPath, rightPath string) string {
	leftDir := path.Dir(leftPath)
	rightDir := path.Dir(rightPath)
	if leftDir == "." {
		leftDir = ""
	}
	if rightDir == "." {
		rightDir = ""
	}
	leftParts := splitPathParts(leftDir)
	rightParts := splitPathParts(rightDir)
	limit := len(leftParts)
	if len(rightParts) < limit {
		limit = len(rightParts)
	}
	var common []string
	for i := 0; i < limit; i++ {
		if leftParts[i] != rightParts[i] {
			break
		}
		common = append(common, leftParts[i])
	}
	return strings.Join(common, "/")
}

func splitPathParts(value string) []string {
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return nil
	}
	return strings.Split(value, "/")
}

func (m *materializer) upsertConnectorDetailed(ctx context.Context, ownerType, ownerKey string, viewID, sourceElementID, targetElementID int64, label, relationship, description string) error {
	return m.upsertConnectorDetailedWithDirection(ctx, ownerType, ownerKey, viewID, sourceElementID, targetElementID, label, relationship, "forward", description)
}

func (m *materializer) upsertConnectorDetailedWithDirection(ctx context.Context, ownerType, ownerKey string, viewID, sourceElementID, targetElementID int64, label, relationship, direction, description string) error {
	if sourceElementID == 0 || targetElementID == 0 || sourceElementID == targetElementID {
		return nil
	}
	if strings.TrimSpace(relationship) == "" {
		relationship = label
	}
	direction = normalizedArchitectureConnectorDirection(direction)
	sourceHandle, targetHandle := "", ""
	if ownerType == "fact-reference" || ownerType == "fact-import-connector" {
		sourceHandle = "right"
		targetHandle = "left"
	}
	if state, ok, err := m.store.MappingState(ctx, m.repo.ID, ownerType, ownerKey, "connector"); err != nil {
		return err
	} else if ok && connectorExists(ctx, m.store.db, state.ResourceID) {
		dirty, err := m.mappingDirty(ctx, ownerType, ownerKey, "connector", state)
		if err != nil {
			return err
		}
		if dirty {
			m.stats.ConnectorsPreserved++
			return m.saveMapping(ctx, ownerType, ownerKey, "connector", state.ResourceID)
		}
		_, err = m.store.execRaw(ctx, `
			UPDATE connectors
			SET view_id = ?, source_element_id = ?, target_element_id = ?, label = ?, description = ?, relationship = ?, direction = ?, style = 'bezier', source_handle = ?, target_handle = ?, updated_at = ?
			WHERE id = ?`, viewID, sourceElementID, targetElementID, label, nullString(description), relationship, direction, nullString(sourceHandle), nullString(targetHandle), nowString(), state.ResourceID)
		if err != nil {
			return err
		}
		if err := m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "connector", state.ResourceID); err != nil {
			return err
		}
		m.stats.ConnectorsUpdated++
		return nil
	}
	now := nowString()
	id, err := m.store.insertReturningID(ctx, `
		INSERT INTO connectors(view_id, source_element_id, target_element_id, label, description, relationship, direction, style, source_handle, target_handle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'bezier', ?, ?, ?, ?)`, viewID, sourceElementID, targetElementID, label, nullString(description), relationship, direction, nullString(sourceHandle), nullString(targetHandle), now, now)
	if err != nil {
		return err
	}
	if err := m.saveMappingWithCurrentHash(ctx, ownerType, ownerKey, "connector", id); err != nil {
		return err
	}
	m.stats.ConnectorsCreated++
	return nil
}

func (m *materializer) saveMapping(ctx context.Context, ownerType, ownerKey, resourceType string, resourceID int64) error {
	return m.store.SaveMappingAt(ctx, m.repo.ID, ownerType, ownerKey, resourceType, resourceID, m.runMarker)
}

func (m *materializer) saveMappingWithCurrentHash(ctx context.Context, ownerType, ownerKey, resourceType string, resourceID int64) error {
	resourceHash, exists, err := m.store.WatchResourceHash(ctx, resourceType, resourceID)
	if err != nil {
		return err
	}
	if !exists {
		return m.saveMapping(ctx, ownerType, ownerKey, resourceType, resourceID)
	}
	return m.store.SaveMappingHashAt(ctx, m.repo.ID, ownerType, ownerKey, resourceType, resourceID, resourceHash, m.runMarker)
}

func (m *materializer) mappingDirty(ctx context.Context, ownerType, ownerKey, resourceType string, state materializationState) (bool, error) {
	if state.Dirty {
		return true, nil
	}
	if state.LastWatchHash == nil {
		return false, nil
	}
	currentHash, exists, err := m.store.WatchResourceHash(ctx, resourceType, state.ResourceID)
	if err != nil {
		return false, err
	}
	if !exists || currentHash == *state.LastWatchHash {
		return false, nil
	}
	if err := m.store.MarkMappingDirty(ctx, m.repo.ID, ownerType, ownerKey, resourceType, state.ResourceID); err != nil {
		return false, err
	}
	return true, nil
}

func (m *materializer) pruneStaleResources(ctx context.Context) error {
	preserved, err := m.store.PruneStaleMaterializedResources(ctx, m.repo.ID, m.runMarker)
	if err != nil {
		return err
	}
	m.stats.DeletesPreserved += preserved
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
	err := NewStore(db).rowRaw(ctx, query, id).Scan(&one)
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

type symbolLanguageIndex struct {
	repoLanguage string
	byFile       map[string]string
}

func newSymbolLanguageIndex(symbols map[int64]Symbol) symbolLanguageIndex {
	repoCounts := map[string]int{}
	fileCounts := map[string]map[string]int{}
	for _, sym := range symbols {
		language := languageFromStableKey(sym.StableKey)
		if language != "" {
			repoCounts[language]++
			if fileCounts[sym.FilePath] == nil {
				fileCounts[sym.FilePath] = map[string]int{}
			}
			fileCounts[sym.FilePath][language]++
		}
	}
	index := symbolLanguageIndex{
		repoLanguage: bestLanguage(repoCounts, "source"),
		byFile:       make(map[string]string, len(fileCounts)),
	}
	for file, counts := range fileCounts {
		index.byFile[file] = bestLanguage(counts, "")
	}
	return index
}

func bestLanguage(counts map[string]int, fallback string) string {
	best := fallback
	bestCount := 0
	for language, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || language < best)) {
			best = language
			bestCount = count
		}
	}
	return best
}

func (i symbolLanguageIndex) languageForFile(file string) string {
	if i.byFile == nil {
		return ""
	}
	return i.byFile[file]
}

func (m *materializer) languageForFile(file string) string {
	if m == nil {
		return ""
	}
	return m.symbolLanguages.languageForFile(file)
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
		return ""
	}
}

func technologyLinksForLanguage(language string) []materializedTechnologyLink {
	label := technologyLabel(language)
	slug := technologyCatalogSlug(language)
	if slug == "" {
		if label == "" {
			return []materializedTechnologyLink{}
		}
		return []materializedTechnologyLink{{Type: "custom", Label: label}}
	}
	return []materializedTechnologyLink{{
		Type:          "catalog",
		Slug:          slug,
		Label:         label,
		IsPrimaryIcon: true,
	}}
}

func technologyLinksForElement(technology, language string) []materializedTechnologyLink {
	links := technologyCatalogLinksForLabel(technology)
	if len(links) > 0 {
		return links
	}
	if strings.TrimSpace(technology) != "" {
		return nil
	}
	if langLinks := technologyLinksForLanguage(language); len(langLinks) > 0 {
		return langLinks
	}
	return nil
}

func technologyCatalogLinksForLabel(label string) []materializedTechnologyLink {
	var links []materializedTechnologyLink
	seen := map[string]struct{}{}
	for _, part := range technologyLabelParts(label) {
		slug, displayLabel := technologyCatalogMatchForLabel(part)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		links = append(links, materializedTechnologyLink{
			Type:          "catalog",
			Slug:          slug,
			Label:         displayLabel,
			IsPrimaryIcon: len(links) == 0,
		})
		if len(links) == 3 {
			break
		}
	}
	return links
}

func technologyLabelParts(label string) []string {
	parts := strings.FieldsFunc(label, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 && strings.TrimSpace(label) != "" {
		return []string{strings.TrimSpace(label)}
	}
	return out
}

func technologyCatalogMatchForLabel(label string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "container":
		return "docker", "Container"
	default:
		slug, name, ok := tech.LookupCatalogFuzzy(label)
		if !ok {
			return "", ""
		}
		return slug, name
	}
}

func technologyCatalogSlug(language string) string {
	switch language {
	case "go":
		return "go"
	case "typescript":
		return "typescript"
	case "javascript":
		return "javascript"
	case "python":
		return "python"
	case "java":
		return "java"
	case "cpp":
		return "cplusplus"
	case "c":
		return "c"
	case "json":
		return "json"
	default:
		return ""
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
		end := min(start+size, len(symbols))
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
	for _, file := range sortedKeys(filtered.ChangedFiles) {
		parts = append(parts, "f:"+file)
	}
	for _, file := range sortedKeys(filtered.VisibleFiles) {
		parts = append(parts, "vf:"+file)
	}
	for _, sym := range sortedSymbols(filtered.VisibleSymbols) {
		parts = append(parts, "s:"+sym.StableKey)
	}
	facts := append([]Fact(nil), filtered.VisibleFacts...)
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].Enricher == facts[j].Enricher {
			return facts[i].StableKey < facts[j].StableKey
		}
		return facts[i].Enricher < facts[j].Enricher
	})
	for _, fact := range facts {
		parts = append(parts, "fact:"+fact.Enricher+":"+fact.StableKey+":"+fact.FactHash)
	}
	var expansionKeys []string
	for key := range filtered.ContextExpansions.Tiers {
		expansionKeys = append(expansionKeys, key)
	}
	sort.Strings(expansionKeys)
	for _, key := range expansionKeys {
		parts = append(parts, fmt.Sprintf("x:%s:%d", key, filtered.ContextExpansions.Tiers[key]))
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
