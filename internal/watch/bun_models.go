package watch

import (
	"database/sql"
	"strings"

	"github.com/uptrace/bun"
)

type repositoryModel struct {
	bun.BaseModel `bun:"table:watch_repositories"`

	ID             int64   `bun:"id,pk,autoincrement"`
	RemoteURL      *string `bun:"remote_url"`
	RepoRoot       string  `bun:"repo_root"`
	DisplayName    string  `bun:"display_name"`
	Branch         *string `bun:"branch"`
	HeadCommit     *string `bun:"head_commit"`
	IdentityStatus string  `bun:"identity_status"`
	SettingsHash   string  `bun:"settings_hash"`
	RootElementID  *int64  `bun:"root_element_id"`
	CreatedAt      string  `bun:"created_at"`
	UpdatedAt      string  `bun:"updated_at"`
}

type filterRunModel struct {
	bun.BaseModel `bun:"table:watch_filter_runs"`

	ID                int64  `bun:"id,pk,autoincrement"`
	RepositoryID      int64  `bun:"repository_id"`
	SettingsHash      string `bun:"settings_hash"`
	RawGraphHash      string `bun:"raw_graph_hash"`
	StartedAt         string `bun:"started_at"`
	FinishedAt        string `bun:"finished_at,nullzero"`
	Status            string `bun:"status"`
	VisibleSymbols    int    `bun:"visible_symbols"`
	HiddenSymbols     int    `bun:"hidden_symbols"`
	VisibleReferences int    `bun:"visible_references"`
	HiddenReferences  int    `bun:"hidden_references"`
}

type impactRunModel struct {
	bun.BaseModel `bun:"table:impact_runs"`

	ID           int64  `bun:"id,pk,autoincrement"`
	RepoRef      string `bun:"repo_ref"`
	RepoRoot     string `bun:"repo_root"`
	RemoteURL    string `bun:"remote_url"`
	Base         string `bun:"base"`
	Head         string `bun:"head"`
	ViewID       int32  `bun:"view_id"`
	ArchRevision string `bun:"arch_revision"`
	ChangedFiles string `bun:"changed_files"`
	Changed      string `bun:"changed"`
	Candidates   string `bun:"candidates"`
	Related      string `bun:"related"`
	Edges        string `bun:"edges"`
	Unmapped     string `bun:"unmapped"`
	Coverage     string `bun:"coverage"`
	CreatedAt    string `bun:"created_at"`
}

type filterDecisionModel struct {
	bun.BaseModel `bun:"table:watch_filter_decisions"`

	ID          int64    `bun:"id,pk,autoincrement"`
	FilterRunID int64    `bun:"filter_run_id"`
	OwnerType   string   `bun:"owner_type"`
	OwnerID     int64    `bun:"owner_id"`
	OwnerKey    string   `bun:"owner_key"`
	Decision    string   `bun:"decision"`
	Reason      string   `bun:"reason"`
	Score       *float64 `bun:"score"`
	Tier        int      `bun:"tier"`
	SignalsJSON string   `bun:"signals_json"`
}

func repositoryFromModel(row repositoryModel) Repository {
	return Repository{
		ID:             row.ID,
		RemoteURL:      sqlNullStringFromPtr(row.RemoteURL),
		RepoRoot:       row.RepoRoot,
		DisplayName:    row.DisplayName,
		Branch:         sqlNullStringFromPtr(row.Branch),
		HeadCommit:     sqlNullStringFromPtr(row.HeadCommit),
		IdentityStatus: row.IdentityStatus,
		SettingsHash:   row.SettingsHash,
		RootElementID:  sqlNullInt64FromPtr(row.RootElementID),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func sqlNullInt64FromPtr(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func sqlNullStringFromPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func stringPtrOrNil(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
