package watch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

// ImpactRun is a persisted, read-only snapshot of an impact analysis. It is a
// trimmed form of ImpactReport: evidence is flattened to display strings, and
// element/connector ids are resolved at persist time so a loaded run can be
// rendered without the live architecture.
type ImpactRun struct {
	ID           int64
	RepoRef      string
	RepoRoot     string
	RemoteURL    string
	Base         string
	Head         string
	ViewID       int32
	ArchRevision string

	ChangedFiles []ChangedFile
	Changed      []ImpactRunElement
	Candidates   []ImpactRunElement
	Related      []ImpactRunElement
	Edges        []ImpactRunEdge
	Unmapped     []string
	Coverage     Coverage

	CreatedAt string
}

// ImpactRunElement is an impacted authored element in a persisted run.
type ImpactRunElement struct {
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Owner     string   `json:"owner,omitempty"`
	ElementID *int32   `json:"element_id,omitempty"`
	Change    string   `json:"change,omitempty"`
	Evidence  []string `json:"evidence,omitempty"`
}

// ImpactRunEdge is a declared or observed relationship in a persisted run.
type ImpactRunEdge struct {
	SourceRef   string `json:"source_ref"`
	TargetRef   string `json:"target_ref"`
	Label       string `json:"label,omitempty"`
	ConnectorID *int32 `json:"connector_id,omitempty"`
	Observed    bool   `json:"observed"`
}

// SaveImpactRun inserts a run snapshot and returns it with its id and
// timestamp populated.
func (s *Store) SaveImpactRun(ctx context.Context, run ImpactRun) (ImpactRun, error) {
	now := nowString()
	row := &impactRunModel{
		RepoRef:      run.RepoRef,
		RepoRoot:     run.RepoRoot,
		RemoteURL:    run.RemoteURL,
		Base:         run.Base,
		Head:         run.Head,
		ViewID:       run.ViewID,
		ArchRevision: run.ArchRevision,
		ChangedFiles: marshalImpactJSON(run.ChangedFiles, "[]"),
		Changed:      marshalImpactJSON(run.Changed, "[]"),
		Candidates:   marshalImpactJSON(run.Candidates, "[]"),
		Related:      marshalImpactJSON(run.Related, "[]"),
		Edges:        marshalImpactJSON(run.Edges, "[]"),
		Unmapped:     marshalImpactJSON(run.Unmapped, "[]"),
		Coverage:     marshalImpactJSON(run.Coverage, "{}"),
		CreatedAt:    now,
	}
	if _, err := s.bun.NewInsert().Model(row).Exec(ctx); err != nil {
		return ImpactRun{}, err
	}
	run.ID = row.ID
	run.CreatedAt = now
	return run, nil
}

// LatestImpactRun returns the most recent run persisted for a repository
// checkout path. The second return value is false when no run exists.
func (s *Store) LatestImpactRun(ctx context.Context, repoRoot string) (ImpactRun, bool, error) {
	var row impactRunModel
	err := s.bun.NewSelect().
		Model(&row).
		Where("repo_root = ?", repoRoot).
		Order("created_at DESC").
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ImpactRun{}, false, nil
	}
	if err != nil {
		return ImpactRun{}, false, err
	}
	return impactRunFromModel(row), true, nil
}

func impactRunFromModel(row impactRunModel) ImpactRun {
	run := ImpactRun{
		ID:           row.ID,
		RepoRef:      row.RepoRef,
		RepoRoot:     row.RepoRoot,
		RemoteURL:    row.RemoteURL,
		Base:         row.Base,
		Head:         row.Head,
		ViewID:       row.ViewID,
		ArchRevision: row.ArchRevision,
		CreatedAt:    row.CreatedAt,
	}
	decodeImpactJSON(row.ChangedFiles, &run.ChangedFiles)
	decodeImpactJSON(row.Changed, &run.Changed)
	decodeImpactJSON(row.Candidates, &run.Candidates)
	decodeImpactJSON(row.Related, &run.Related)
	decodeImpactJSON(row.Edges, &run.Edges)
	decodeImpactJSON(row.Unmapped, &run.Unmapped)
	decodeImpactJSON(row.Coverage, &run.Coverage)
	if run.Unmapped == nil {
		run.Unmapped = []string{}
	}
	return run
}

func marshalImpactJSON(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

func decodeImpactJSON(raw string, dest any) {
	if raw == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), dest)
}

// ArchitectureRevision is a stable hash of the authored architecture a report
// was computed against. It lets a persisted run be marked stale when the
// architecture changes without touching the report itself.
func ArchitectureRevision(elements map[string]*workspace.Element, connectors map[string]*workspace.Connector) string {
	hash := sha256.New()
	refs := make([]string, 0, len(elements))
	for ref := range elements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		element := elements[ref]
		if element == nil {
			continue
		}
		_, _ = fmt.Fprintf(hash, "e|%s|%s|%s|%s|%s|%s|%s\n",
			ref, element.Name, element.Kind, element.Repo, element.Branch, element.FilePath, element.Symbol)
	}
	keys := make([]string, 0, len(connectors))
	for key := range connectors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		connector := connectors[key]
		if connector == nil {
			continue
		}
		_, _ = fmt.Fprintf(hash, "c|%s|%s\n", key, connector.Label)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
