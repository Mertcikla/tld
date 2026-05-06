package watch

import (
	"database/sql"
	"time"
)

const SettingsHash = ""

type Repository struct {
	ID             int64          `json:"id"`
	RemoteURL      sql.NullString `json:"-"`
	RepoRoot       string         `json:"repo_root"`
	DisplayName    string         `json:"display_name"`
	Branch         sql.NullString `json:"-"`
	HeadCommit     sql.NullString `json:"-"`
	IdentityStatus string         `json:"identity_status"`
	SettingsHash   string         `json:"settings_hash"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type RepositoryJSON struct {
	ID             int64   `json:"id"`
	RemoteURL      *string `json:"remote_url"`
	RepoRoot       string  `json:"repo_root"`
	DisplayName    string  `json:"display_name"`
	Branch         *string `json:"branch"`
	HeadCommit     *string `json:"head_commit"`
	IdentityStatus string  `json:"identity_status"`
	SettingsHash   string  `json:"settings_hash"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type File struct {
	ID           int64          `json:"id"`
	RepositoryID int64          `json:"repository_id"`
	Path         string         `json:"path"`
	Language     string         `json:"language"`
	GitBlobHash  sql.NullString `json:"-"`
	WorktreeHash string         `json:"worktree_hash"`
	SizeBytes    int64          `json:"size_bytes"`
	MtimeUnix    int64          `json:"mtime_unix"`
	ScanStatus   string         `json:"scan_status"`
	ScanError    sql.NullString `json:"-"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type Symbol struct {
	ID            int64  `json:"id"`
	RepositoryID  int64  `json:"repository_id"`
	FileID        int64  `json:"file_id"`
	FilePath      string `json:"file_path,omitempty"`
	StableKey     string `json:"stable_key"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	Kind          string `json:"kind"`
	StartLine     int    `json:"start_line"`
	EndLine       *int   `json:"end_line"`
	SignatureHash string `json:"signature_hash"`
	ContentHash   string `json:"content_hash"`
	RawJSON       string `json:"raw_json"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type Reference struct {
	ID             int64  `json:"id"`
	RepositoryID   int64  `json:"repository_id"`
	SourceSymbolID int64  `json:"source_symbol_id"`
	TargetSymbolID int64  `json:"target_symbol_id"`
	SourceFileID   int64  `json:"source_file_id"`
	Kind           string `json:"kind"`
	Line           int    `json:"line"`
	Column         int    `json:"column"`
	EvidenceHash   string `json:"evidence_hash"`
	RawJSON        string `json:"raw_json"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type Fact struct {
	ID               int64    `json:"id"`
	RepositoryID     int64    `json:"repository_id"`
	FileID           int64    `json:"file_id"`
	FilePath         string   `json:"file_path"`
	StableKey        string   `json:"stable_key"`
	Type             string   `json:"type"`
	Enricher         string   `json:"enricher"`
	SubjectKind      string   `json:"subject_kind"`
	SubjectStableKey string   `json:"subject_stable_key"`
	StartLine        int      `json:"start_line"`
	EndLine          *int     `json:"end_line,omitempty"`
	Confidence       float64  `json:"confidence"`
	Name             string   `json:"name"`
	Tags             []string `json:"tags"`
	AttributesJSON   string   `json:"attributes_json"`
	FactHash         string   `json:"fact_hash"`
	RawJSON          string   `json:"raw_json"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type Summary struct {
	RepositoryID     int64  `json:"repository_id"`
	Files            int    `json:"files"`
	Symbols          int    `json:"symbols"`
	References       int    `json:"references"`
	LastScanStatus   string `json:"last_scan_status,omitempty"`
	LastScanStarted  string `json:"last_scan_started_at,omitempty"`
	LastScanFinished string `json:"last_scan_finished_at,omitempty"`
}

type ScanResult struct {
	RepositoryID   int64    `json:"repository_id"`
	ScanRunID      int64    `json:"scan_run_id"`
	FilesSeen      int      `json:"files_seen"`
	FilesParsed    int      `json:"files_parsed"`
	FilesSkipped   int      `json:"files_skipped"`
	SymbolsSeen    int      `json:"symbols_seen"`
	ReferencesSeen int      `json:"references_seen"`
	Warning        string   `json:"warning,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

type EmbeddingConfig struct {
	Provider        string  `json:"provider" yaml:"provider"`
	Endpoint        string  `json:"endpoint,omitempty" yaml:"endpoint"`
	Model           string  `json:"model" yaml:"model"`
	Dimension       int     `json:"dimension" yaml:"dimension"`
	HealthThreshold float64 `json:"health_threshold,omitempty" yaml:"health_threshold"`
}

type Thresholds struct {
	MaxElementsPerView            int `json:"max_elements_per_view"`
	MaxConnectorsPerView          int `json:"max_connectors_per_view"`
	MaxIncomingPerElement         int `json:"max_incoming_per_element"`
	MaxOutgoingPerElement         int `json:"max_outgoing_per_element"`
	MaxExpandedConnectorsPerGroup int `json:"max_expanded_connectors_per_group"`
}

type Settings struct {
	Languages    []string      `json:"languages"`
	Watcher      string        `json:"watcher"`
	PollInterval time.Duration `json:"poll_interval"`
	Debounce     time.Duration `json:"debounce"`
	Thresholds   Thresholds    `json:"thresholds"`
}

type RepresentRequest struct {
	Embedding  EmbeddingConfig `json:"embedding"`
	Thresholds Thresholds      `json:"thresholds"`
	Progress   ProgressSink    `json:"-"`
}

type RepresentResult struct {
	RepositoryID        int64  `json:"repository_id"`
	RepresentationRun   int64  `json:"representation_run_id"`
	FilterRunID         int64  `json:"filter_run_id"`
	RawGraphHash        string `json:"raw_graph_hash"`
	SettingsHash        string `json:"settings_hash"`
	RepresentationHash  string `json:"representation_hash"`
	ElementsCreated     int    `json:"elements_created"`
	ElementsUpdated     int    `json:"elements_updated"`
	ConnectorsCreated   int    `json:"connectors_created"`
	ConnectorsUpdated   int    `json:"connectors_updated"`
	ViewsCreated        int    `json:"views_created"`
	ElementsPreserved   int    `json:"elements_preserved"`
	ConnectorsPreserved int    `json:"connectors_preserved"`
	ViewsPreserved      int    `json:"views_preserved"`
	DeletesPreserved    int    `json:"deletes_preserved"`
	EmbeddingCacheHits  int    `json:"embedding_cache_hits"`
	EmbeddingsCreated   int    `json:"embeddings_created"`
}

type ProgressSink interface {
	Start(label string, total int)
	Advance(label string)
	Finish()
}

type RepresentationSummary struct {
	RepositoryID       int64                `json:"repository_id"`
	RawGraphHash       string               `json:"raw_graph_hash,omitempty"`
	SettingsHash       string               `json:"filter_settings_hash,omitempty"`
	RepresentationHash string               `json:"representation_hash,omitempty"`
	LastStatus         string               `json:"last_status,omitempty"`
	LastStartedAt      string               `json:"last_started_at,omitempty"`
	LastFinishedAt     *string              `json:"last_finished_at,omitempty"`
	ElementsCreated    int                  `json:"elements_created"`
	ElementsUpdated    int                  `json:"elements_updated"`
	ConnectorsCreated  int                  `json:"connectors_created"`
	ConnectorsUpdated  int                  `json:"connectors_updated"`
	ViewsCreated       int                  `json:"views_created"`
	VisibleSymbols     int                  `json:"visible_symbols"`
	HiddenSymbols      int                  `json:"hidden_symbols"`
	VisibleReferences  int                  `json:"visible_references"`
	HiddenReferences   int                  `json:"hidden_references"`
	Diffs              []RepresentationDiff `json:"diffs,omitempty"`
}

type FilterDecision struct {
	ID          int64    `json:"id"`
	FilterRunID int64    `json:"filter_run_id"`
	OwnerType   string   `json:"owner_type"`
	OwnerID     int64    `json:"owner_id"`
	Decision    string   `json:"decision"`
	Reason      string   `json:"reason"`
	Score       *float64 `json:"score,omitempty"`
}

type Cluster struct {
	ID              int64  `json:"id"`
	RepositoryID    int64  `json:"repository_id"`
	StableKey       string `json:"stable_key"`
	ParentClusterID *int64 `json:"parent_cluster_id,omitempty"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Algorithm       string `json:"algorithm"`
	SettingsHash    string `json:"settings_hash"`
	MemberCount     int    `json:"member_count"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type MaterializationMapping struct {
	ID              int64   `json:"id"`
	RepositoryID    int64   `json:"repository_id"`
	OwnerType       string  `json:"owner_type"`
	OwnerKey        string  `json:"owner_key"`
	ResourceType    string  `json:"resource_type"`
	ResourceID      int64   `json:"resource_id"`
	LastWatchHash   *string `json:"last_watch_hash,omitempty"`
	Dirty           bool    `json:"dirty"`
	DirtyDetectedAt *string `json:"dirty_detected_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ContextPolicy struct {
	ID           int64  `json:"id"`
	RepositoryID int64  `json:"repository_id"`
	OwnerType    string `json:"owner_type"`
	OwnerKey     string `json:"owner_key"`
	Action       string `json:"action"`
	Scope        string `json:"scope"`
	Active       bool   `json:"active"`
	Reason       string `json:"reason"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ContextResourceRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
}

type ContextActionResult struct {
	RepositoryID        int64                 `json:"repository_id"`
	Action              string                `json:"action"`
	PoliciesCreated     int                   `json:"policies_created"`
	PoliciesUpdated     int                   `json:"policies_updated"`
	PoliciesDeactivated int                   `json:"policies_deactivated"`
	OwnersAffected      int                   `json:"owners_affected"`
	ElementsRemoved     int                   `json:"elements_removed"`
	ConnectorsRemoved   int                   `json:"connectors_removed"`
	ViewsRemoved        int                   `json:"views_removed"`
	Representation      RepresentResult       `json:"representation"`
	Summary             RepresentationSummary `json:"summary"`
}

type Lock struct {
	ID           int64  `json:"id"`
	RepositoryID int64  `json:"repository_id"`
	PID          int    `json:"pid"`
	Token        string `json:"token,omitempty"`
	StartedAt    string `json:"started_at"`
	HeartbeatAt  string `json:"heartbeat_at"`
	Status       string `json:"status"`
}

type GitStatus struct {
	Branch      string   `json:"branch"`
	HeadCommit  string   `json:"head_commit"`
	HeadMessage string   `json:"head_message,omitempty"`
	RemoteURL   string   `json:"remote_url"`
	Staged      []string `json:"staged"`
	Unstaged    []string `json:"unstaged"`
	Untracked   []string `json:"untracked"`
	Deleted     []string `json:"deleted"`
}

type GitTagUpdateResult struct {
	TagsAdded   int `json:"tags_added"`
	TagsRemoved int `json:"tags_removed"`
}

type SourceFileChange struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"`
	Language   string `json:"language,omitempty"`
}

type SourceFileChangeResult struct {
	Change                SourceFileChange   `json:"change"`
	RepresentationChanged bool               `json:"representation_changed"`
	Representation        RepresentResult    `json:"representation"`
	GitTags               GitTagUpdateResult `json:"git_tags"`
}

type ChangeCounter struct {
	TotalChangesProcessed    int `json:"total_changes_processed"`
	IntervalChangesProcessed int `json:"interval_changes_processed"`
}

type Event struct {
	Type         string   `json:"type"`
	RepositoryID int64    `json:"repository_id,omitempty"`
	Message      string   `json:"message,omitempty"`
	At           string   `json:"at"`
	Data         any      `json:"data,omitempty"`
	Phase        string   `json:"phase,omitempty"`
	WatcherMode  string   `json:"watcher_mode,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	ChangedFiles int      `json:"changed_files,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

type Version struct {
	ID                 int64  `json:"id"`
	RepositoryID       int64  `json:"repository_id"`
	CommitHash         string `json:"commit_hash"`
	CommitMessage      string `json:"commit_message,omitempty"`
	ParentCommitHash   string `json:"parent_commit_hash,omitempty"`
	Branch             string `json:"branch,omitempty"`
	RepresentationHash string `json:"representation_hash"`
	WorkspaceVersionID *int64 `json:"workspace_version_id,omitempty"`
	CreatedAt          string `json:"created_at"`
}

type RepresentationDiff struct {
	ID           int64   `json:"id"`
	VersionID    int64   `json:"version_id"`
	OwnerType    string  `json:"owner_type"`
	OwnerKey     string  `json:"owner_key"`
	ChangeType   string  `json:"change_type"`
	BeforeHash   *string `json:"before_hash,omitempty"`
	AfterHash    *string `json:"after_hash,omitempty"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID   *int64  `json:"resource_id,omitempty"`
	Language     *string `json:"language,omitempty"`
	Summary      *string `json:"summary,omitempty"`
	AddedLines   int     `json:"added_lines,omitempty"`
	RemovedLines int     `json:"removed_lines,omitempty"`
}

func (r Repository) JSON() RepositoryJSON {
	return RepositoryJSON{
		ID:             r.ID,
		RemoteURL:      nullStringPtr(r.RemoteURL),
		RepoRoot:       r.RepoRoot,
		DisplayName:    r.DisplayName,
		Branch:         nullStringPtr(r.Branch),
		HeadCommit:     nullStringPtr(r.HeadCommit),
		IdentityStatus: r.IdentityStatus,
		SettingsHash:   r.SettingsHash,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
