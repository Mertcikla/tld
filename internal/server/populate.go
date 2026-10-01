package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/embed"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

type populateElementResult struct {
	ID                   int64           `json:"id"`
	Name                 string          `json:"name"`
	Kind                 *string         `json:"kind"`
	Description          *string         `json:"description"`
	Technology           *string         `json:"technology"`
	URL                  *string         `json:"url"`
	LogoURL              *string         `json:"logo_url"`
	TechnologyConnectors json.RawMessage `json:"technology_connectors"`
	Tags                 json.RawMessage `json:"tags"`
	Repo                 *string         `json:"repo,omitempty"`
	Branch               *string         `json:"branch,omitempty"`
	FilePath             *string         `json:"file_path,omitempty"`
	Language             *string         `json:"language,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	SimilarityScore      float64         `json:"similarity_score"`
	MatchKind            string          `json:"match_kind,omitempty"`
	MatchReason          string          `json:"match_reason,omitempty"`
	RelatedTo            string          `json:"related_to,omitempty"`
	ViaKind              string          `json:"via_kind,omitempty"`
	Placed               bool            `json:"placed"`
}

// maxNeighbors caps the one-hop expansion so a query near a hub node cannot
// pull the whole graph into the result set.
const maxNeighbors = 40

var (
	errPopulateViewNotFound        = errors.New("view not found")
	errPopulateNoRepository        = errors.New("no indexed repositories found; run 'tld index <path>' first")
	errPopulateAmbiguousRepository = errors.New("this view is not linked to a repository and several are indexed; add an indexed element to the view or open an indexed view")
	errPopulateNoSnapshot          = errors.New("the linked repository has no published snapshot")
	errEmbeddingsUnavailable       = errors.New("embedding server unavailable")
	errEmbeddingsNotConfigured     = errors.New("embeddings are not configured; start a local server with 'make embed-server'")
)

type populateDeps struct {
	db    *sql.DB
	ws    core.Store
	idx   *cstore.Store
	embed embed.Client
}

// registerPopulateHandlers serves the "populate view" search. Populate is a
// scoped projection of the indexed code graph: the query is embedded, matched
// against the view's repository snapshot by vector similarity, and the matched
// facts are materialized into the view as candidate elements and connectors.
func registerPopulateHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore, cfg *workspace.Config) {
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	deps := &populateDeps{
		db:    sqliteStore.DB(),
		ws:    sqliteStore,
		idx:   idx,
		embed: embed.Client{Config: configbridge.FromGlobal(cfg), Store: idx},
	}

	mux.HandleFunc("GET /api/debug/populate-reranker-metrics", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"enabled": false, "metrics": map[string]any{}})
	})

	mux.HandleFunc("GET /api/views/{id}/populate-query", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		query, err := buildPopulateQuery(r.Context(), deps.db, viewID, "")
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "view not found")
			} else {
				writeJSONError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		writeJSON(w, map[string]string{"query": query, "enriched_query": query})
	})

	mux.HandleFunc("GET /api/views/{id}/populate", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			writeJSON(w, map[string]any{"results": []any{}})
			return
		}
		limit := 5
		if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 {
			limit = parsed
		}
		if limit > 50 {
			limit = 50
		}
		results, err := deps.search(r.Context(), viewID, query, limit)
		if err != nil {
			switch {
			case errors.Is(err, errPopulateViewNotFound):
				writeJSONError(w, http.StatusNotFound, err.Error())
			case errors.Is(err, errPopulateNoRepository), errors.Is(err, errPopulateAmbiguousRepository), errors.Is(err, errPopulateNoSnapshot):
				writeJSONError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, errEmbeddingsNotConfigured), errors.Is(err, errEmbeddingsUnavailable):
				writeJSONError(w, http.StatusServiceUnavailable, err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "failed to populate view: "+err.Error())
			}
			return
		}
		writeJSON(w, map[string]any{"results": results})
	})
}

func buildPopulateQuery(ctx context.Context, db *sql.DB, viewID int64, userQuery string) (string, error) {
	var viewName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM views WHERE id = ?`, viewID).Scan(&viewName); err != nil {
		return "", err
	}
	if base := strings.TrimSpace(userQuery); base != "" {
		return base, nil
	}
	return strings.TrimSpace(viewName), nil
}

// resolveRepository determines which indexed repository a populate request is
// scoped to. It prefers an explicit view mapping (recorded when the indexer
// materializes into a view), then the repositories of the view's placed
// elements, then a single globally indexed repository.
func (d *populateDeps) resolveRepository(ctx context.Context, viewID int64) (string, error) {
	if mapping, ok, err := d.idx.MappingByResource(ctx, cstore.MappingView, viewID); err != nil {
		return "", err
	} else if ok && mapping.RepositoryID != "" {
		return mapping.RepositoryID, nil
	}

	if placements, err := d.ws.Placements(ctx, viewID); err == nil {
		repositories := map[string]struct{}{}
		for _, placement := range placements {
			if mapping, ok, err := d.idx.MappingByResource(ctx, cstore.MappingElement, placement.ElementID); err == nil && ok && mapping.RepositoryID != "" {
				repositories[mapping.RepositoryID] = struct{}{}
			}
		}
		switch {
		case len(repositories) == 1:
			for id := range repositories {
				return id, nil
			}
		case len(repositories) > 1:
			return "", errPopulateAmbiguousRepository
		}
	}

	repositories, err := d.idx.ListRepositories(ctx)
	if err != nil {
		return "", err
	}
	indexed := make([]string, 0, len(repositories))
	for _, repo := range repositories {
		if repo.LatestSnapshotId != "" {
			indexed = append(indexed, repo.Id)
		}
	}
	switch len(indexed) {
	case 0:
		return "", errPopulateNoRepository
	case 1:
		return indexed[0], nil
	default:
		return "", errPopulateAmbiguousRepository
	}
}

// canonicalize keeps exactly one candidate per logical key (the same symbol)
// and merges their edges. Refs are stable cross-snapshot identity, so this
// removes genuine duplicates without collapsing distinct declarations that
// merely share a name. It also de-dupes parallel edges by (from, to, kind).
func canonicalize(p project.Result) project.Result {
	identity := func(element project.Element) string {
		return element.Ref
	}

	primary := map[string]string{}
	remap := map[string]string{}
	out := project.Result{SnapshotID: p.SnapshotID}
	for _, element := range p.Elements {
		key := identity(element)
		if kept, ok := primary[key]; ok {
			remap[element.Ref] = kept
			continue
		}
		primary[key] = element.Ref
		remap[element.Ref] = element.Ref
		out.Elements = append(out.Elements, element)
	}

	seen := map[string]bool{}
	for _, connector := range p.Connectors {
		from := remap[connector.FromRef]
		if from == "" {
			from = connector.FromRef
		}
		to := remap[connector.ToRef]
		if to == "" {
			to = connector.ToRef
		}
		if from == to {
			continue
		}
		key := fmt.Sprintf("%s|%s|%d", from, to, int(connector.Kind))
		if seen[key] {
			continue
		}
		seen[key] = true
		connector.FromRef = from
		connector.ToRef = to
		out.Connectors = append(out.Connectors, connector)
	}
	return out
}

func (d *populateDeps) search(ctx context.Context, viewID int64, query string, limit int) ([]populateElementResult, error) {
	if _, err := d.ws.ViewByID(ctx, viewID); err != nil {
		return nil, errPopulateViewNotFound
	}

	repositoryID, err := d.resolveRepository(ctx, viewID)
	if err != nil {
		return nil, err
	}
	snapshotID, err := d.idx.Latest(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if snapshotID == "" {
		return nil, errPopulateNoSnapshot
	}
	if !d.embed.Enabled() {
		return nil, errEmbeddingsNotConfigured
	}
	vector, err := d.embed.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errEmbeddingsUnavailable, err)
	}
	profile := d.embed.Profile()

	hits, err := d.idx.SimilarFacts(ctx, snapshotID, profile, vector, limit)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return []populateElementResult{}, nil
	}
	factIDs := make([]string, 0, len(hits))
	seenFactID := map[string]bool{}
	for _, hit := range hits {
		if seenFactID[hit.FactID] {
			continue
		}
		seenFactID[hit.FactID] = true
		factIDs = append(factIDs, hit.FactID)
	}
	similarity, err := d.idx.FactSimilarities(ctx, snapshotID, profile, vector, factIDs)
	if err != nil {
		return nil, err
	}

	snapshot, err := d.idx.Snapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	graph, err := d.idx.LoadGraph(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	projection := canonicalize(project.Project(snapshot, graph))

	nameByRef := map[string]string{}
	for _, element := range projection.Elements {
		nameByRef[element.Ref] = element.Name
	}
	seedScore := map[string]float64{}
	for _, id := range factIDs {
		fact := graph.Facts[id]
		if fact == nil {
			continue
		}
		ref := fact.LogicalKey
		if ref == "" {
			ref = fact.Id
		}
		seedScore[ref] = float64(similarity[id])
	}

	// Expand the scope by one hop: include each seed's immediate neighbors and
	// the edges that connect the subgraph, so populate materializes a connected
	// projection rather than isolated seed nodes.
	type linkInfo struct{ seedRef, kind string }
	linkedBy := map[string]linkInfo{}
	included := map[string]bool{}
	for ref := range seedScore {
		included[ref] = true
	}
	neighbors := 0
	for _, connector := range projection.Connectors {
		_, fromSeed := seedScore[connector.FromRef]
		_, toSeed := seedScore[connector.ToRef]
		var neighborRef, seedRef string
		switch {
		case fromSeed && !toSeed:
			neighborRef, seedRef = connector.ToRef, connector.FromRef
		case toSeed && !fromSeed:
			neighborRef, seedRef = connector.FromRef, connector.ToRef
		default:
			continue
		}
		if included[neighborRef] {
			continue
		}
		if neighbors >= maxNeighbors {
			break
		}
		included[neighborRef] = true
		linkedBy[neighborRef] = linkInfo{seedRef: seedRef, kind: strings.ToLower(project.KindLabelEdge(connector.Kind))}
		neighbors++
	}

	scoped := project.Result{SnapshotID: snapshotID}
	for _, element := range projection.Elements {
		if included[element.Ref] {
			scoped.Elements = append(scoped.Elements, element)
		}
	}
	for _, connector := range projection.Connectors {
		if included[connector.FromRef] && included[connector.ToRef] {
			scoped.Connectors = append(scoped.Connectors, connector)
		}
	}
	if len(scoped.Elements) == 0 {
		return []populateElementResult{}, nil
	}

	repoName := ""
	if repo, repoErr := d.idx.Repository(ctx, repositoryID); repoErr == nil && repo != nil {
		repoName = filepath.Base(repo.Root)
	}
	if _, err := materialize.ApplyScoped(ctx, d.ws, d.idx, scoped, materialize.ScopedOptions{
		RepositoryID:   repositoryID,
		RepositoryName: repoName,
		SnapshotID:     snapshotID,
		ViewID:         viewID,
	}); err != nil {
		return nil, err
	}

	placed := map[int64]bool{}
	if placements, err := d.ws.Placements(ctx, viewID); err == nil {
		for _, placement := range placements {
			placed[placement.ElementID] = true
		}
	}

	type resultMeta struct {
		score     float64
		reason    string
		relatedTo string
		viaKind   string
	}
	ids := make([]int64, 0, len(scoped.Elements))
	metaByID := map[int64]resultMeta{}
	for _, element := range scoped.Elements {
		mapping, ok, err := d.idx.MappingByLogicalKey(ctx, element.Ref)
		if err != nil || !ok || mapping.Kind != cstore.MappingElement {
			continue
		}
		score, isSeed := seedScore[element.Ref]
		meta := resultMeta{score: score, reason: "vector"}
		if !isSeed {
			link := linkedBy[element.Ref]
			meta = resultMeta{reason: "neighbor", relatedTo: nameByRef[link.seedRef], viaKind: link.kind}
		}
		// Merge duplicates: a direct match always wins over a neighbor, and the
		// first id is kept so the result list stays unique.
		if existing, ok := metaByID[mapping.ResourceID]; ok {
			if existing.reason == "vector" || !isSeed {
				continue
			}
			metaByID[mapping.ResourceID] = meta
			continue
		}
		metaByID[mapping.ResourceID] = meta
		ids = append(ids, mapping.ResourceID)
	}
	if len(ids) == 0 {
		return []populateElementResult{}, nil
	}

	results, err := d.elementsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range results {
		meta := metaByID[results[i].ID]
		results[i].SimilarityScore = meta.score
		results[i].MatchReason = meta.reason
		results[i].RelatedTo = meta.relatedTo
		results[i].ViaKind = meta.viaKind
		results[i].Placed = placed[results[i].ID]
		if results[i].Kind != nil {
			results[i].MatchKind = *results[i].Kind
		}
	}
	results = dedupeResults(results)
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if (left.MatchReason == "vector") != (right.MatchReason == "vector") {
			return left.MatchReason == "vector"
		}
		if left.SimilarityScore != right.SimilarityScore {
			return left.SimilarityScore > right.SimilarityScore
		}
		return left.Name < right.Name
	})
	return results, nil
}

// dedupeResults keeps one entry per element, merging duplicate ids and
// preferring a direct match's metadata when an element matched both ways.
func dedupeResults(results []populateElementResult) []populateElementResult {
	out := make([]populateElementResult, 0, len(results))
	index := map[int64]int{}
	for _, result := range results {
		if at, ok := index[result.ID]; ok {
			if out[at].MatchReason == "neighbor" && result.MatchReason == "vector" {
				out[at] = result
			}
			continue
		}
		index[result.ID] = len(out)
		out = append(out, result)
	}
	return out
}

func (d *populateDeps) elementsByIDs(ctx context.Context, ids []int64) ([]populateElementResult, error) {
	if len(ids) == 0 {
		return []populateElementResult{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT el.id, el.name, el.kind, el.description, el.technology, el.url, el.logo_url,
		       el.technology_connectors, el.tags, el.repo, el.branch, el.file_path, el.language,
		       el.created_at, el.updated_at
		FROM elements el
		WHERE el.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]populateElementResult, 0, len(ids))
	for rows.Next() {
		var (
			el       populateElementResult
			techJSON string
			tagsJSON string
		)
		if err := rows.Scan(&el.ID, &el.Name, &el.Kind, &el.Description, &el.Technology, &el.URL, &el.LogoURL,
			&techJSON, &tagsJSON, &el.Repo, &el.Branch, &el.FilePath, &el.Language, &el.CreatedAt, &el.UpdatedAt); err != nil {
			return nil, err
		}
		el.TechnologyConnectors = rawJSON(techJSON, "[]")
		el.Tags = rawJSON(tagsJSON, "[]")
		out = append(out, el)
	}
	return out, rows.Err()
}

func rawJSON(value, fallback string) json.RawMessage {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(trimmed)
}
