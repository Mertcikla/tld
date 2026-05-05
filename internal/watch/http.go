package watch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	Store       *Store
	Representer *Representer
}

func NewHandler(store *Store) *Handler {
	return &Handler{Store: store, Representer: NewRepresenter(store)}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/watch/ws", h.watchWebSocket)
	mux.HandleFunc("GET /api/watch/status", h.status)
	mux.HandleFunc("GET /api/watch/repositories", h.listRepositories)
	mux.HandleFunc("GET /api/watch/repositories/{id}/raw-graph/summary", h.rawGraphSummary)
	mux.HandleFunc("GET /api/watch/repositories/{id}/raw-graph/symbols", h.rawGraphSymbols)
	mux.HandleFunc("GET /api/watch/repositories/{id}/raw-graph/references", h.rawGraphReferences)
	mux.HandleFunc("POST /api/watch/repositories/{id}/reassociate", h.reassociateRepository)
	mux.HandleFunc("POST /api/watch/repositories/{id}/represent", h.representRepository)
	mux.HandleFunc("GET /api/watch/repositories/{id}/representation/summary", h.representationSummary)
	mux.HandleFunc("GET /api/watch/repositories/{id}/filter-decisions", h.filterDecisions)
	mux.HandleFunc("GET /api/watch/repositories/{id}/clusters", h.clusters)
	mux.HandleFunc("GET /api/watch/repositories/{id}/materialization", h.materialization)
	mux.HandleFunc("GET /api/watch/repositories/{id}/versions", h.versions)
	mux.HandleFunc("GET /api/watch/versions/{id}/diffs", h.versionDiffs)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	lock, live, err := h.Store.ActiveLiveLock(r.Context(), LockHeartbeatTimeout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load watch status")
		return
	}
	if !live {
		writeJSON(w, http.StatusOK, map[string]any{
			"active":            false,
			"connected_clients": WatchWebSocketClientCount(),
		})
		return
	}
	repo, err := h.Store.Repository(r.Context(), lock.RepositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load watch repository")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":            true,
		"repository":        repo.JSON(),
		"lock":              lock,
		"connected_clients": WatchWebSocketClientCount(),
	})
}

func (h *Handler) listRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := h.Store.Repositories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list repositories")
		return
	}
	out := make([]RepositoryJSON, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repo.JSON())
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) rawGraphSummary(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	summary, err := h.Store.Summary(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load raw graph summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) rawGraphSymbols(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	query := r.URL.Query()
	symbols, err := h.Store.QuerySymbols(r.Context(), repositoryID, SymbolQuery{
		Search: query.Get("search"),
		File:   query.Get("file"),
		Kind:   query.Get("kind"),
		Limit:  parseInt(query.Get("limit"), 100),
		Offset: parseInt(query.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list raw graph symbols")
		return
	}
	writeJSON(w, http.StatusOK, symbols)
}

func (h *Handler) rawGraphReferences(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	query := r.URL.Query()
	refs, err := h.Store.QueryReferences(r.Context(), repositoryID, ReferenceQuery{
		SymbolID: int64(parseInt(query.Get("symbol_id"), 0)),
		Limit:    parseInt(query.Get("limit"), 100),
		Offset:   parseInt(query.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list raw graph references")
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

func (h *Handler) reassociateRepository(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		RemoteURL string `json:"remote_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	repo, err := h.Store.ReassociateRepository(r.Context(), repositoryID, body.RemoteURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repo.JSON())
}

func (h *Handler) representRepository(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	var body RepresentRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}
	representer := h.Representer
	if representer == nil {
		representer = NewRepresenter(h.Store)
	}
	result, err := representer.Represent(r.Context(), repositoryID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) representationSummary(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	summary, err := h.Store.RepresentationSummary(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load representation summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) filterDecisions(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	query := r.URL.Query()
	decisions, err := h.Store.FilterDecisions(r.Context(), repositoryID, FilterDecisionQuery{
		OwnerType: query.Get("owner_type"),
		Decision:  query.Get("decision"),
		Limit:     parseInt(query.Get("limit"), 100),
		Offset:    parseInt(query.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list filter decisions")
		return
	}
	writeJSON(w, http.StatusOK, decisions)
}

func (h *Handler) clusters(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	clusters, err := h.Store.Clusters(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list clusters")
		return
	}
	writeJSON(w, http.StatusOK, clusters)
}

func (h *Handler) materialization(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	mappings, err := h.Store.Materialization(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list materialization mappings")
		return
	}
	writeJSON(w, http.StatusOK, mappings)
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	versions, err := h.Store.WatchVersions(r.Context(), repositoryID, parseInt(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list watch versions")
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (h *Handler) versionDiffs(w http.ResponseWriter, r *http.Request) {
	versionID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	query := r.URL.Query()
	diffs, err := h.Store.WatchDiffs(r.Context(), versionID, query.Get("owner_type"), query.Get("change_type"), query.Get("resource_type"), query.Get("language"), parseInt(query.Get("limit"), 200))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list watch diffs")
		return
	}
	writeJSON(w, http.StatusOK, diffs)
}

func parseIDPath(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
