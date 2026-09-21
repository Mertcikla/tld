package watch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// SettingsDescriptor merges a config definition with its effective value so the
// UI can render the full watch tuning surface generically.
type SettingsDescriptor struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Source      string `json:"source"`
	Env         string `json:"env,omitempty"`
	Description string `json:"description"`
	Secret      bool   `json:"secret,omitempty"`
}

type repositorySettingsResponse struct {
	RepositoryID int64                 `json:"repository_id"`
	Settings     Settings              `json:"settings"`
	Embedding    EmbeddingConfig       `json:"embedding"`
	Overridden   bool                  `json:"overridden"`
	UpdatedAt    string                `json:"updated_at,omitempty"`
	Descriptors  []SettingsDescriptor  `json:"descriptors"`
}

func (h *Handler) addRepository(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "resolve path: "+err.Error())
		return
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not an existing directory")
		return
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is not inside a git repository", path))
		return
	}
	cfg := h.globalConfig()
	settings := ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	repo, err := h.Store.EnsureRepository(r.Context(), RepositoryInput{
		RemoteURL:    detectString(func() (string, error) { return tldgit.DetectRemoteURL(repoRoot) }),
		RepoRoot:     repoRoot,
		DisplayName:  filepath.Base(repoRoot),
		Branch:       detectString(func() (string, error) { return tldgit.DetectBranch(repoRoot) }),
		HeadCommit:   detectString(func() (string, error) { return tldgit.DetectHeadCommit(repoRoot) }),
		SettingsHash: stableHash(settings),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repo.JSON())
}

func (h *Handler) settingsDescriptors(w http.ResponseWriter, r *http.Request) {
	descriptors, err := watchSettingsDescriptors()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load watch settings descriptors")
		return
	}
	writeJSON(w, http.StatusOK, descriptors)
}

func (h *Handler) repositorySettings(w http.ResponseWriter, r *http.Request) {
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	cfg := h.globalConfig()
	settings := ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	embedding := ResolveEmbeddingConfig(cfg, "", "", "", 0, 0)
	overridden := false
	updatedAt := ""
	if stored, found, err := h.Store.RepositorySettings(r.Context(), repositoryID); err != nil {
		writeError(w, http.StatusInternalServerError, "load repository settings")
		return
	} else if found {
		settings = stored.Settings
		embedding = stored.Embedding
		overridden = true
		updatedAt = stored.UpdatedAt
	}
	descriptors, _ := watchSettingsDescriptors()
	writeJSON(w, http.StatusOK, repositorySettingsResponse{
		RepositoryID: repositoryID,
		Settings:     settings,
		Embedding:    embedding,
		Overridden:   overridden,
		UpdatedAt:    updatedAt,
		Descriptors:  descriptors,
	})
}

func (h *Handler) saveRepositorySettings(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Settings  *Settings        `json:"settings"`
		Embedding *EmbeddingConfig `json:"embedding"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg := h.globalConfig()
	settings := ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	embedding := ResolveEmbeddingConfig(cfg, "", "", "", 0, 0)
	if body.Settings != nil {
		settings = *body.Settings
	}
	if body.Embedding != nil {
		embedding = *body.Embedding
	}
	saved, err := h.Store.SaveRepositorySettings(r.Context(), repositoryID, settings, embedding)
	if err != nil {
		status := http.StatusBadRequest
		var validationErr SettingsValidationErrors
		if !asValidationErrors(err, &validationErr) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err.Error())
		return
	}
	descriptors, _ := watchSettingsDescriptors()
	writeJSON(w, http.StatusOK, repositorySettingsResponse{
		RepositoryID: repositoryID,
		Settings:     saved.Settings,
		Embedding:    saved.Embedding,
		Overridden:   true,
		UpdatedAt:    saved.UpdatedAt,
		Descriptors:  descriptors,
	})
}

func (h *Handler) deleteRepositorySettings(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	if err := h.Store.DeleteRepositorySettings(r.Context(), repositoryID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete repository settings")
		return
	}
	cfg := h.globalConfig()
	settings := ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	embedding := ResolveEmbeddingConfig(cfg, "", "", "", 0, 0)
	descriptors, _ := watchSettingsDescriptors()
	writeJSON(w, http.StatusOK, repositorySettingsResponse{
		RepositoryID: repositoryID,
		Settings:     settings,
		Embedding:    embedding,
		Overridden:   false,
		Descriptors:  descriptors,
	})
}

type healthcheckResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	// LSP is set for language-server healthchecks.
	LSP *LSPStatus `json:"lsp,omitempty"`
	// Embedding is set for embedding healthchecks.
	Embedding *EmbeddingHealth `json:"embedding,omitempty"`
}

type EmbeddingHealth struct {
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	Endpoint   string  `json:"endpoint,omitempty"`
	Dimension  int     `json:"dimension"`
	Similarity float64 `json:"similarity"`
}

func (h *Handler) lspHealthcheck(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	settings, _, err := h.effectiveSettings(r, repositoryID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := InitialLSPStatus(settings)
	ok = lspHealthy(status)
	message := ""
	if !ok {
		reasons := lspWarnings(status)
		message = strings.Join(reasons, "; ")
	}
	writeJSON(w, http.StatusOK, healthcheckResponse{OK: ok, Message: message, LSP: &status})
}

func (h *Handler) embeddingHealthcheck(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	repositoryID, ok := parseIDPath(w, r, "id")
	if !ok {
		return
	}
	_, embedding, err := h.effectiveSettings(r, repositoryID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	displayEndpoint := embedding.Endpoint
	if displayEndpoint == "" && len(embedding.Endpoints) > 0 {
		displayEndpoint = embedding.Endpoints[0]
	}
	if embedding.Provider == "" || embedding.Provider == "none" {
		writeJSON(w, http.StatusOK, healthcheckResponse{
			OK:      false,
			Message: "embedding provider is disabled",
			Embedding: &EmbeddingHealth{
				Provider: embedding.Provider,
				Model:    embedding.Model,
			},
		})
		return
	}
	checked, health, err := CheckEmbeddingHealth(r.Context(), embedding)
	if err != nil {
		writeJSON(w, http.StatusOK, healthcheckResponse{
			OK:      false,
			Message: err.Error(),
			Embedding: &EmbeddingHealth{
				Provider: embedding.Provider,
				Model:    embedding.Model,
				Endpoint: displayEndpoint,
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, healthcheckResponse{
		OK: true,
		Embedding: &EmbeddingHealth{
			Provider:   checked.Provider,
			Model:      checked.Model,
			Endpoint:   displayEndpoint,
			Dimension:  health.Dimension,
			Similarity: health.Similarity,
		},
	})
}

func (h *Handler) effectiveSettings(r *http.Request, repositoryID int64) (Settings, EmbeddingConfig, error) {
	cfg := h.globalConfig()
	baseSettings := ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	baseEmbedding := ResolveEmbeddingConfig(cfg, "", "", "", 0, 0)
	stored, found, err := h.Store.RepositorySettings(r.Context(), repositoryID)
	if err != nil {
		return Settings{}, EmbeddingConfig{}, err
	}
	if found {
		return stored.Settings, stored.Embedding, nil
	}
	return baseSettings, baseEmbedding, nil
}

func (h *Handler) globalConfig() *workspace.Config {
	if h.Config == nil {
		return nil
	}
	return h.Config()
}

func (h *Handler) originAllowed(r *http.Request) bool {
	if h.CheckOrigin == nil {
		return true
	}
	return h.CheckOrigin(r)
}

func asValidationErrors(err error, target *SettingsValidationErrors) bool {
	validationErr, ok := err.(SettingsValidationErrors)
	if !ok {
		return false
	}
	*target = validationErr
	return true
}

func watchSettingsDescriptors() ([]SettingsDescriptor, error) {
	state, err := workspace.LoadGlobalConfigState()
	if err != nil {
		return nil, err
	}
	defs := workspace.ConfigDefinitions()
	out := make([]SettingsDescriptor, 0, len(defs))
	byKey := map[string]workspace.ConfigValue{}
	for _, value := range state.Values {
		byKey[value.Key] = value
	}
	for _, def := range defs {
		if !strings.HasPrefix(def.Key, "watch.") {
			continue
		}
		value, ok := byKey[def.Key]
		descriptor := SettingsDescriptor{
			Key:         def.Key,
			Description: def.Description,
			Secret:      def.Secret,
		}
		if len(def.Env) > 0 {
			descriptor.Env = strings.Join(def.Env, ",")
		}
		if ok {
			descriptor.Value = value.Value
			descriptor.Source = string(value.Source)
		}
		out = append(out, descriptor)
	}
	return out, nil
}

func lspHealthy(status LSPStatus) bool {
	if !status.Enabled {
		return true
	}
	return status.Summary.Unavailable == 0 && status.Summary.Failed == 0 && status.Summary.MemoryLimited == 0
}
