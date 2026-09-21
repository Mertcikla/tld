package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// RepositorySettings is the persisted per-repository override layer for watch
// and analyze. Settings holds the watch tuning values and Embedding holds the
// embedding provider configuration. An absent row means "use global config".
type RepositorySettings struct {
	RepositoryID int64           `json:"repository_id"`
	Settings     Settings        `json:"settings"`
	Embedding    EmbeddingConfig `json:"embedding"`
	UpdatedAt    string          `json:"updated_at,omitempty"`
}

// RepositorySettingsOverride reports the raw override plus whether one exists.
type RepositorySettingsOverride struct {
	Settings  Settings
	Embedding EmbeddingConfig
	Found     bool
}

// RepositorySettings returns the stored override for a repository. When no
// override exists it returns found=false and no error.
func (s *Store) RepositorySettings(ctx context.Context, repositoryID int64) (RepositorySettings, bool, error) {
	if s == nil {
		return RepositorySettings{}, false, fmt.Errorf("watch store is nil")
	}
	var settingsJSON, embeddingJSON, updatedAt string
	err := s.rowRaw(ctx, `
		SELECT settings_json, embedding_json, updated_at
		FROM watch_repository_settings
		WHERE repository_id = ?`, repositoryID).Scan(&settingsJSON, &embeddingJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RepositorySettings{}, false, nil
	}
	if err != nil {
		return RepositorySettings{}, false, err
	}
	settings := DefaultSettings()
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return RepositorySettings{}, false, fmt.Errorf("decode repository settings: %w", err)
	}
	embedding := EmbeddingConfig{}
	if embeddingJSON != "" {
		if err := json.Unmarshal([]byte(embeddingJSON), &embedding); err != nil {
			return RepositorySettings{}, false, fmt.Errorf("decode repository embedding settings: %w", err)
		}
	}
	return RepositorySettings{
		RepositoryID: repositoryID,
		Settings:     NormalizeSettings(settings),
		Embedding:    NormalizeEmbeddingConfig(embedding),
		UpdatedAt:    updatedAt,
	}, true, nil
}

// SaveRepositorySettings validates and persists a per-repository override.
func (s *Store) SaveRepositorySettings(ctx context.Context, repositoryID int64, settings Settings, embedding EmbeddingConfig) (RepositorySettings, error) {
	if s == nil {
		return RepositorySettings{}, fmt.Errorf("watch store is nil")
	}
	if repositoryID <= 0 {
		return RepositorySettings{}, fmt.Errorf("repository id is required")
	}
	if err := ValidateSettings(settings); err != nil {
		return RepositorySettings{}, err
	}
	if err := ValidateEmbeddingConfig(embedding); err != nil {
		return RepositorySettings{}, err
	}
	normalizedSettings := NormalizeSettings(settings)
	normalizedEmbedding := NormalizeEmbeddingConfig(embedding)
	settingsJSON, err := json.Marshal(normalizedSettings)
	if err != nil {
		return RepositorySettings{}, fmt.Errorf("encode repository settings: %w", err)
	}
	embeddingJSON, err := json.Marshal(normalizedEmbedding)
	if err != nil {
		return RepositorySettings{}, fmt.Errorf("encode repository embedding settings: %w", err)
	}
	updatedAt := nowString()
	_, err = s.execRaw(ctx, `
		INSERT INTO watch_repository_settings(repository_id, settings_json, embedding_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
			settings_json = excluded.settings_json,
			embedding_json = excluded.embedding_json,
			updated_at = excluded.updated_at`,
		repositoryID, string(settingsJSON), string(embeddingJSON), updatedAt)
	if err != nil {
		return RepositorySettings{}, err
	}
	return RepositorySettings{
		RepositoryID: repositoryID,
		Settings:     normalizedSettings,
		Embedding:    normalizedEmbedding,
		UpdatedAt:    updatedAt,
	}, nil
}

// DeleteRepositorySettings removes the override so the repository falls back to
// global configuration.
func (s *Store) DeleteRepositorySettings(ctx context.Context, repositoryID int64) error {
	if s == nil {
		return fmt.Errorf("watch store is nil")
	}
	_, err := s.execRaw(ctx, `DELETE FROM watch_repository_settings WHERE repository_id = ?`, repositoryID)
	return err
}
