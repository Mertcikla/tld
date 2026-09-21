package watch

import (
	"net/http"
	"strings"
)

type watcherStatusResponse struct {
	RepositoryID int64           `json:"repository_id"`
	Managed      bool            `json:"managed"`
	Process      *WatcherProcess `json:"process,omitempty"`
	Live         bool            `json:"live"`
	Paused       bool            `json:"paused"`
	Lock         *Lock           `json:"lock,omitempty"`
	WatcherMode  string          `json:"watcher_mode,omitempty"`
}

type scanProgressResponse struct {
	RepositoryID   int64                 `json:"repository_id"`
	Summary        Summary               `json:"summary"`
	Representation RepresentationSummary `json:"representation"`
	Watcher        watcherStatusResponse `json:"watcher"`
}

func (h *Handler) startWatch(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	if h.Supervisor == nil {
		writeError(w, http.StatusNotImplemented, "watch supervisor is not available in this server build")
		return
	}
	h.Supervisor.Reap()
	if status, live, err := h.Store.ActiveLiveLock(r.Context(), LockHeartbeatTimeout); err == nil && live && status.RepositoryID == repositoryID {
		writeError(w, http.StatusConflict, "watch is already running for this repository")
		return
	}
	repo, err := h.Store.Repository(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusNotFound, "repository not found")
		return
	}
	if strings.TrimSpace(repo.RepoRoot) == "" {
		writeError(w, http.StatusBadRequest, "repository has no local path")
		return
	}
	process, err := h.Supervisor.Start(r.Context(), StartOptions{
		RepositoryID: repositoryID,
		RepoRoot:     repo.RepoRoot,
		DataDir:      h.DataDir,
		Args:         h.watchStartArgs(r),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, watcherStatusResponse{
		RepositoryID: repositoryID,
		Managed:      true,
		Process:      &process,
		Live:         true,
	})
}

func (h *Handler) stopWatch(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	if h.Supervisor == nil {
		writeError(w, http.StatusNotImplemented, "watch supervisor is not available in this server build")
		return
	}
	managed, err := h.Supervisor.Stop(repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Also clear any live lock so an externally started watcher can be stopped
	// through the same control surface.
	if err := h.Store.RequestStop(r.Context(), repositoryID); err != nil {
		writeError(w, http.StatusInternalServerError, "stop watch")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repository_id": repositoryID,
		"managed":       managed,
		"stopped":       true,
	})
}

func (h *Handler) scanProgress(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	summary, err := h.Store.Summary(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load scan summary")
		return
	}
	representation, err := h.Store.RepresentationSummary(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load representation summary")
		return
	}
	writeJSON(w, http.StatusOK, scanProgressResponse{
		RepositoryID:   repositoryID,
		Summary:        summary,
		Representation: representation,
		Watcher:        h.watcherStatus(r, repositoryID),
	})
}

func (h *Handler) listProcesses(w http.ResponseWriter, r *http.Request) {
	if h.Supervisor == nil {
		writeJSON(w, http.StatusOK, []WatcherProcess{})
		return
	}
	h.Supervisor.Reap()
	processes := h.Supervisor.List()
	if processes == nil {
		processes = []WatcherProcess{}
	}
	writeJSON(w, http.StatusOK, processes)
}

func (h *Handler) watcherStatus(r *http.Request, repositoryID int64) watcherStatusResponse {
	response := watcherStatusResponse{RepositoryID: repositoryID}
	if h.Supervisor != nil {
		if process, ok := h.Supervisor.Status(repositoryID); ok {
			response.Managed = true
			response.Process = &process
		}
	}
	lock, live, err := h.Store.ActiveLiveLock(r.Context(), LockHeartbeatTimeout)
	if err == nil && live && lock.RepositoryID == repositoryID {
		response.Live = true
		response.Lock = &lock
		response.Paused = lock.Status == "paused"
	}
	return response
}

func (h *Handler) watchStartArgs(r *http.Request) []string {
	query := r.URL.Query()
	var args []string
	if strings.EqualFold(query.Get("rescan"), "true") {
		args = append(args, "--rescan")
	}
	if value := strings.TrimSpace(query.Get("verbose")); strings.EqualFold(value, "true") {
		args = append(args, "--verbose")
	}
	return args
}
