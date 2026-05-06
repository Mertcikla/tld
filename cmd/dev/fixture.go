package dev

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	assets "github.com/mertcikla/tld"
	"github.com/mertcikla/tld/internal/localserver"
	storepkg "github.com/mertcikla/tld/internal/store"
	"github.com/mertcikla/tld/internal/term"
	watchpkg "github.com/mertcikla/tld/internal/watch"
	"github.com/spf13/cobra"
)

const fixtureSchemaVersion = 1

type fixtureOptions struct {
	Name             string
	CorpusDir        string
	Approve          bool
	Reject           bool
	JSON             bool
	NoCopy           bool
	Notes            []string
	LanguageFlags    []string
	MaxElements      int
	MaxConnectors    int
	MaxIncoming      int
	MaxOutgoing      int
	MaxExpandedGroup int
	Language         string
	Domain           string
	Framework        string
	Type             string
	ReviewStatus     string
	Accuracy         string
	ReviewComments   []string
	ReviewedAt       *time.Time
}

type fixtureManifest struct {
	SchemaVersion  int        `json:"schema_version"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Language       string     `json:"language,omitempty"`
	Domain         string     `json:"domain,omitempty"`
	Framework      string     `json:"framework,omitempty"`
	Type           string     `json:"type,omitempty"`
	Notes          []string   `json:"notes,omitempty"`
	ReviewStatus   string     `json:"review_status,omitempty"`
	Accuracy       string     `json:"accuracy,omitempty"`
	ReviewComments []string   `json:"review_comments,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	SourcePath     string     `json:"source_path,omitempty"`
	RepoPath       string     `json:"repo_path,omitempty"`
	SnapshotPath   string     `json:"snapshot_path"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type fixtureSnapshot struct {
	SchemaVersion  int                   `json:"schema_version"`
	Name           string                `json:"name"`
	Repository     fixtureRepository     `json:"repository"`
	Representation fixtureRepresentation `json:"representation"`
	Counts         map[string]int        `json:"counts"`
	Elements       []fixtureElement      `json:"elements"`
	Connectors     []fixtureConnector    `json:"connectors"`
	Views          []fixtureView         `json:"views"`
	Facts          []fixtureFact         `json:"facts"`
	Decisions      []fixtureDecision     `json:"filter_decisions"`
}

type fixtureRepository struct {
	Files      int `json:"files"`
	Symbols    int `json:"symbols"`
	References int `json:"references"`
	Facts      int `json:"facts"`
}

type fixtureRepresentation struct {
	RawGraphHash       string `json:"raw_graph_hash"`
	SettingsHash       string `json:"settings_hash"`
	RepresentationHash string `json:"representation_hash"`
	ElementsCreated    int    `json:"elements_created"`
	ElementsUpdated    int    `json:"elements_updated"`
	ConnectorsCreated  int    `json:"connectors_created"`
	ConnectorsUpdated  int    `json:"connectors_updated"`
	ViewsCreated       int    `json:"views_created"`
}

type fixtureElement struct {
	OwnerType  string   `json:"owner_type"`
	OwnerKey   string   `json:"owner_key"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind,omitempty"`
	Technology string   `json:"technology,omitempty"`
	FilePath   string   `json:"file_path,omitempty"`
	Language   string   `json:"language,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type fixtureConnector struct {
	OwnerType string `json:"owner_type"`
	OwnerKey  string `json:"owner_key"`
	Label     string `json:"label,omitempty"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	View      string `json:"view"`
}

type fixtureView struct {
	OwnerType string `json:"owner_type"`
	OwnerKey  string `json:"owner_key"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
}

type fixtureFact struct {
	Type             string   `json:"type"`
	Enricher         string   `json:"enricher"`
	StableKey        string   `json:"stable_key"`
	SubjectKind      string   `json:"subject_kind"`
	SubjectStableKey string   `json:"subject_stable_key"`
	ObjectKind       string   `json:"object_kind,omitempty"`
	ObjectStableKey  string   `json:"object_stable_key,omitempty"`
	Relationship     string   `json:"relationship,omitempty"`
	FilePath         string   `json:"file_path"`
	Name             string   `json:"name,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

type fixtureDecision struct {
	OwnerType string   `json:"owner_type"`
	OwnerKey  string   `json:"owner_key"`
	Decision  string   `json:"decision"`
	Reason    string   `json:"reason,omitempty"`
	Score     *float64 `json:"score,omitempty"`
	Tier      int      `json:"tier,omitempty"`
	Signals   []string `json:"signals,omitempty"`
}

func newFixtureCmd() *cobra.Command {
	opts := fixtureOptions{CorpusDir: filepath.Join("internal", "watch", "testdata", "corpus")}
	c := &cobra.Command{
		Use:   "fixture <repo-path>",
		Short: "Preview and approve watch golden corpus fixtures",
		Long: `Runs the watch pipeline against a small repository, prints a canonical preview,
then records a fixture manifest, notes, source copy, and stable JSON snapshot for golden tests.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Approve && opts.Reject {
				return fmt.Errorf("--approve and --reject are mutually exclusive")
			}
			return runFixtureBuilder(cmd, args[0], opts)
		},
	}
	c.Flags().StringVar(&opts.Name, "name", "", "fixture name; defaults to the repository directory name")
	c.Flags().StringVar(&opts.CorpusDir, "corpus-dir", opts.CorpusDir, "directory that stores fixture cases")
	c.Flags().StringVar(&opts.Language, "fixture-language", "", "fixture taxonomy language, for example go or typescript")
	c.Flags().StringVar(&opts.Domain, "fixture-domain", "", "fixture taxonomy domain, for example http, frontend, orm, or dependency")
	c.Flags().StringVar(&opts.Framework, "fixture-framework", "", "fixture taxonomy framework or library, for example gin, express, or prisma")
	c.Flags().StringVar(&opts.Type, "fixture-type", "", "fixture taxonomy type, for example basic_route or dependency_manifest")
	c.Flags().BoolVar(&opts.Approve, "approve", false, "approve without prompting")
	c.Flags().BoolVar(&opts.Reject, "reject", false, "reject without prompting")
	c.Flags().BoolVar(&opts.JSON, "json", false, "print the canonical snapshot JSON and do not write fixture files")
	c.Flags().BoolVar(&opts.NoCopy, "no-copy", false, "do not copy source files into the fixture repo directory")
	c.Flags().StringArrayVar(&opts.Notes, "note", nil, "review note to attach to the fixture; repeatable")
	c.Flags().StringSliceVar(&opts.LanguageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().IntVar(&opts.MaxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&opts.MaxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&opts.MaxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&opts.MaxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&opts.MaxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	return c
}

func runFixtureBuilder(cmd *cobra.Command, repoPath string, opts fixtureOptions) error {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	name := fixtureName(opts.Name, absRepo)
	settings := resolveFixtureWatchSettings(opts.LanguageFlags, opts.MaxElements, opts.MaxConnectors, opts.MaxIncoming, opts.MaxOutgoing, opts.MaxExpandedGroup)
	tmpDir, err := os.MkdirTemp("", "tld-watch-fixture-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	sqliteStore, err := storepkg.Open(localserver.DatabasePath(tmpDir), assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sqliteStore.DB().Close() }()
	watchStore := watchpkg.NewStore(sqliteStore.DB())
	once, err := watchpkg.NewRunner(watchStore).RunOnce(cmd.Context(), watchpkg.OneShotOptions{
		Path:      absRepo,
		Rescan:    true,
		Embedding: watchpkg.EmbeddingConfig{Provider: "none"},
		Settings:  settings,
	})
	if err != nil {
		return err
	}
	snapshot, err := buildFixtureSnapshot(cmd.Context(), sqliteStore.DB(), watchStore, name, once)
	if err != nil {
		return err
	}
	if opts.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(snapshot)
	}
	printFixturePreview(cmd.OutOrStdout(), snapshot)
	status := ""
	if opts.Approve {
		status = "approved"
	} else if opts.Reject {
		status = "rejected"
	} else {
		status, opts, err = runFixtureCandidateReviewTUI(cmd.Context(), cmd.OutOrStdout(), snapshot, opts)
		if err != nil && errors.Is(err, errFixtureReviewUnavailable) {
			status, opts.Notes, err = promptFixtureReview(cmd.InOrStdin(), cmd.OutOrStdout(), opts.Notes)
		}
		if err != nil {
			return err
		}
	}
	if status == "skip" {
		term.Hint(cmd.OutOrStdout(), "Skipped writing fixture files.")
		return nil
	}
	sourceRepo := once.Repository.RepoRoot
	if strings.TrimSpace(sourceRepo) == "" {
		sourceRepo = absRepo
	}
	if err := writeFixtureFiles(sourceRepo, name, opts, status, snapshot); err != nil {
		return err
	}
	term.Successf(cmd.OutOrStdout(), "fixture %s: %s", status, filepath.Join(opts.CorpusDir, name))
	return nil
}

func fixtureName(name, repoPath string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(repoPath)
	}
	name = strings.ToLower(name)
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('_')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func resolveFixtureWatchSettings(languages []string, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int) watchpkg.Settings {
	settings := watchpkg.DefaultSettings()
	if len(languages) > 0 {
		settings.Languages = languages
	}
	if maxElements > 0 {
		settings.Thresholds.MaxElementsPerView = maxElements
	}
	if maxConnectors > 0 {
		settings.Thresholds.MaxConnectorsPerView = maxConnectors
	}
	if maxIncoming > 0 {
		settings.Thresholds.MaxIncomingPerElement = maxIncoming
	}
	if maxOutgoing > 0 {
		settings.Thresholds.MaxOutgoingPerElement = maxOutgoing
	}
	if maxExpandedGroup > 0 {
		settings.Thresholds.MaxExpandedConnectorsPerGroup = maxExpandedGroup
	}
	return watchpkg.NormalizeSettings(settings)
}

func buildFixtureSnapshot(ctx context.Context, db *sql.DB, store *watchpkg.Store, name string, once watchpkg.OneShotResult) (fixtureSnapshot, error) {
	repoSummary, err := store.Summary(ctx, once.Scan.RepositoryID)
	if err != nil {
		return fixtureSnapshot{}, err
	}
	factCount, err := fixtureCount(ctx, db, `SELECT COUNT(*) FROM watch_facts WHERE repository_id = ?`, once.Scan.RepositoryID)
	if err != nil {
		return fixtureSnapshot{}, err
	}
	snapshot := fixtureSnapshot{
		SchemaVersion: fixtureSchemaVersion,
		Name:          name,
		Repository: fixtureRepository{
			Files:      repoSummary.Files,
			Symbols:    repoSummary.Symbols,
			References: repoSummary.References,
			Facts:      factCount,
		},
		Representation: fixtureRepresentation{
			RawGraphHash:       once.Representation.RawGraphHash,
			SettingsHash:       once.Representation.SettingsHash,
			RepresentationHash: once.Representation.RepresentationHash,
			ElementsCreated:    once.Representation.ElementsCreated,
			ElementsUpdated:    once.Representation.ElementsUpdated,
			ConnectorsCreated:  once.Representation.ConnectorsCreated,
			ConnectorsUpdated:  once.Representation.ConnectorsUpdated,
			ViewsCreated:       once.Representation.ViewsCreated,
		},
		Counts: map[string]int{},
	}
	if snapshot.Elements, err = fixtureElements(ctx, db, once.Scan.RepositoryID); err != nil {
		return fixtureSnapshot{}, err
	}
	if snapshot.Views, err = fixtureViews(ctx, db, once.Scan.RepositoryID); err != nil {
		return fixtureSnapshot{}, err
	}
	if snapshot.Connectors, err = fixtureConnectors(ctx, db, once.Scan.RepositoryID); err != nil {
		return fixtureSnapshot{}, err
	}
	if snapshot.Facts, err = fixtureFacts(ctx, store, once.Scan.RepositoryID); err != nil {
		return fixtureSnapshot{}, err
	}
	if snapshot.Decisions, err = fixtureDecisions(ctx, store, once.Scan.RepositoryID); err != nil {
		return fixtureSnapshot{}, err
	}
	snapshot.Counts["elements"] = len(snapshot.Elements)
	snapshot.Counts["connectors"] = len(snapshot.Connectors)
	snapshot.Counts["views"] = len(snapshot.Views)
	snapshot.Counts["facts"] = len(snapshot.Facts)
	snapshot.Counts["filter_decisions"] = len(snapshot.Decisions)
	normalizeFixtureSnapshot(&snapshot)
	return snapshot, nil
}

func normalizeFixtureSnapshot(snapshot *fixtureSnapshot) {
	if snapshot == nil {
		return
	}
	for i := range snapshot.Elements {
		snapshot.Elements[i].OwnerKey = normalizeFixtureOwnerKey(snapshot.Elements[i].OwnerType, snapshot.Elements[i].OwnerKey)
		if snapshot.Elements[i].OwnerType == "repository" {
			snapshot.Elements[i].Name = snapshot.Name
		}
	}
	for i := range snapshot.Views {
		snapshot.Views[i].OwnerKey = normalizeFixtureOwnerKey(snapshot.Views[i].OwnerType, snapshot.Views[i].OwnerKey)
		if snapshot.Views[i].OwnerType == "repository" {
			snapshot.Views[i].Name = snapshot.Name
		}
	}
	for i := range snapshot.Connectors {
		snapshot.Connectors[i].OwnerKey = normalizeFixtureOwnerKey(snapshot.Connectors[i].OwnerType, snapshot.Connectors[i].OwnerKey)
		snapshot.Connectors[i].Source = normalizeFixtureOwnerRef(snapshot.Connectors[i].Source)
		snapshot.Connectors[i].Target = normalizeFixtureOwnerRef(snapshot.Connectors[i].Target)
		snapshot.Connectors[i].View = normalizeFixtureOwnerRef(snapshot.Connectors[i].View)
	}
	for i := range snapshot.Decisions {
		snapshot.Decisions[i].OwnerKey = normalizeFixtureOwnerKey(snapshot.Decisions[i].OwnerType, snapshot.Decisions[i].OwnerKey)
	}
}

func normalizeFixtureOwnerRef(ref string) string {
	ownerType, ownerKey, ok := strings.Cut(ref, ":")
	if !ok {
		return ref
	}
	return ownerType + ":" + normalizeFixtureOwnerKey(ownerType, ownerKey)
}

func normalizeFixtureOwnerKey(ownerType, ownerKey string) string {
	if ownerType == "repository" {
		return "repository"
	}
	return ownerKey
}

func fixtureElements(ctx context.Context, db *sql.DB, repositoryID int64) ([]fixtureElement, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key, e.name, COALESCE(e.kind, ''), COALESCE(e.technology, ''),
		       COALESCE(e.file_path, ''), COALESCE(e.language, ''), e.tags
		FROM watch_materialization wm
		JOIN elements e ON wm.resource_type = 'element' AND wm.resource_id = e.id
		WHERE wm.repository_id = ?
		ORDER BY wm.owner_type, wm.owner_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []fixtureElement
	for rows.Next() {
		var item fixtureElement
		var rawTags string
		if err := rows.Scan(&item.OwnerType, &item.OwnerKey, &item.Name, &item.Kind, &item.Technology, &item.FilePath, &item.Language, &rawTags); err != nil {
			return nil, err
		}
		item.Tags = sortedJSONStrings(rawTags)
		out = append(out, item)
	}
	return out, rows.Err()
}

func fixtureCount(ctx context.Context, db *sql.DB, query string, args ...any) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func fixtureViews(ctx context.Context, db *sql.DB, repositoryID int64) ([]fixtureView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key, v.name, v.level
		FROM watch_materialization wm
		JOIN views v ON wm.resource_type = 'view' AND wm.resource_id = v.id
		WHERE wm.repository_id = ?
		ORDER BY wm.owner_type, wm.owner_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []fixtureView
	for rows.Next() {
		var item fixtureView
		if err := rows.Scan(&item.OwnerType, &item.OwnerKey, &item.Name, &item.Level); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func fixtureConnectors(ctx context.Context, db *sql.DB, repositoryID int64) ([]fixtureConnector, error) {
	elementOwners, err := fixtureElementOwnerMap(ctx, db, repositoryID)
	if err != nil {
		return nil, err
	}
	viewOwners, err := fixtureViewOwnerMap(ctx, db, repositoryID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key, COALESCE(c.label, ''), c.source_element_id, c.target_element_id, c.view_id
		FROM watch_materialization wm
		JOIN connectors c ON wm.resource_type = 'connector' AND wm.resource_id = c.id
		WHERE wm.repository_id = ?
		ORDER BY wm.owner_type, wm.owner_key`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []fixtureConnector
	for rows.Next() {
		var item fixtureConnector
		var sourceID, targetID, viewID int64
		if err := rows.Scan(&item.OwnerType, &item.OwnerKey, &item.Label, &sourceID, &targetID, &viewID); err != nil {
			return nil, err
		}
		item.Source = elementOwners[sourceID]
		item.Target = elementOwners[targetID]
		item.View = viewOwners[viewID]
		out = append(out, item)
	}
	return out, rows.Err()
}

func fixtureElementOwnerMap(ctx context.Context, db *sql.DB, repositoryID int64) (map[int64]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT wm.resource_id, wm.owner_type, wm.owner_key
		FROM watch_materialization wm
		WHERE wm.repository_id = ? AND wm.resource_type = 'element'`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var ownerType, ownerKey string
		if err := rows.Scan(&id, &ownerType, &ownerKey); err != nil {
			return nil, err
		}
		out[id] = ownerType + ":" + ownerKey
	}
	return out, rows.Err()
}

func fixtureViewOwnerMap(ctx context.Context, db *sql.DB, repositoryID int64) (map[int64]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT wm.resource_id, wm.owner_type, wm.owner_key
		FROM watch_materialization wm
		WHERE wm.repository_id = ? AND wm.resource_type = 'view'`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var ownerType, ownerKey string
		if err := rows.Scan(&id, &ownerType, &ownerKey); err != nil {
			return nil, err
		}
		out[id] = ownerType + ":" + ownerKey
	}
	return out, rows.Err()
}

func fixtureFacts(ctx context.Context, store *watchpkg.Store, repositoryID int64) ([]fixtureFact, error) {
	facts, err := store.FactsForRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]fixtureFact, 0, len(facts))
	for _, fact := range facts {
		if fact.Type == "watch.enrichment.version" {
			continue
		}
		out = append(out, fixtureFact{
			Type:             fact.Type,
			Enricher:         fact.Enricher,
			StableKey:        fact.StableKey,
			SubjectKind:      fact.SubjectKind,
			SubjectStableKey: fact.SubjectStableKey,
			ObjectKind:       fact.ObjectKind,
			ObjectStableKey:  fact.ObjectStableKey,
			Relationship:     fact.Relationship,
			FilePath:         fact.FilePath,
			Name:             fact.Name,
			Tags:             sortedStrings(fact.Tags),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].StableKey < out[j].StableKey
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func fixtureDecisions(ctx context.Context, store *watchpkg.Store, repositoryID int64) ([]fixtureDecision, error) {
	decisions, err := store.FilterDecisions(ctx, repositoryID, watchpkg.FilterDecisionQuery{Limit: -1})
	if err != nil {
		return nil, err
	}
	out := make([]fixtureDecision, 0, len(decisions))
	for _, decision := range decisions {
		out = append(out, fixtureDecision{
			OwnerType: decision.OwnerType,
			OwnerKey:  decision.OwnerKey,
			Decision:  decision.Decision,
			Reason:    decision.Reason,
			Score:     decision.Score,
			Tier:      decision.Tier,
			Signals:   fixtureSignalNames(decision.SignalsJSON),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerType == out[j].OwnerType {
			return out[i].OwnerKey < out[j].OwnerKey
		}
		return out[i].OwnerType < out[j].OwnerType
	})
	return out, nil
}

func fixtureSignalNames(raw string) []string {
	var signals []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal([]byte(raw), &signals)
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		if strings.TrimSpace(signal.Name) != "" {
			names = append(names, signal.Name)
		}
	}
	return sortedStrings(names)
}

func printFixturePreview(w io.Writer, snapshot fixtureSnapshot) {
	term.Separator(w)
	term.Label(w, 18, "Fixture", snapshot.Name)
	term.Label(w, 18, "Raw graph", snapshot.Representation.RawGraphHash)
	term.Label(w, 18, "Representation", snapshot.Representation.RepresentationHash)
	term.Label(w, 18, "Repository", fmt.Sprintf("%d files, %d symbols, %d references, %d facts", snapshot.Repository.Files, snapshot.Repository.Symbols, snapshot.Repository.References, snapshot.Repository.Facts))
	term.Label(w, 18, "Materialized", fmt.Sprintf("%d elements, %d connectors, %d views", len(snapshot.Elements), len(snapshot.Connectors), len(snapshot.Views)))
	term.Separator(w)
	printFixtureItems(w, "Elements", fixtureElementLines(snapshot.Elements, 12))
	printFixtureItems(w, "Facts", fixtureFactLines(snapshot.Facts, 10))
	printFixtureItems(w, "Visible decisions", fixtureDecisionLines(snapshot.Decisions, "visible", 10))
	printFixtureItems(w, "Hidden decisions", fixtureDecisionLines(snapshot.Decisions, "hidden", 6))
	term.Separator(w)
}

func printFixtureItems(w io.Writer, title string, lines []string) {
	_, _ = fmt.Fprintf(w, "%s\n", title)
	if len(lines) == 0 {
		_, _ = fmt.Fprintln(w, "  none")
		return
	}
	for _, line := range lines {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}

func fixtureElementLines(elements []fixtureElement, limit int) []string {
	var lines []string
	for i, item := range elements {
		if i >= limit {
			lines = append(lines, fmt.Sprintf("... %d more", len(elements)-limit))
			break
		}
		lines = append(lines, fmt.Sprintf("%s:%s -> %s (%s) %s", item.OwnerType, item.OwnerKey, item.Name, item.Kind, strings.Join(item.Tags, ",")))
	}
	return lines
}

func fixtureFactLines(facts []fixtureFact, limit int) []string {
	var lines []string
	for i, fact := range facts {
		if i >= limit {
			lines = append(lines, fmt.Sprintf("... %d more", len(facts)-limit))
			break
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", fact.Type, fact.FilePath, strings.Join(fact.Tags, ",")))
	}
	return lines
}

func fixtureDecisionLines(decisions []fixtureDecision, decision string, limit int) []string {
	var lines []string
	for _, item := range decisions {
		if item.Decision != decision {
			continue
		}
		if len(lines) >= limit {
			lines = append(lines, "...")
			break
		}
		lines = append(lines, fmt.Sprintf("%s:%s %s", item.OwnerType, item.OwnerKey, strings.Join(item.Signals, ",")))
	}
	return lines
}

func promptFixtureReview(r io.Reader, w io.Writer, notes []string) (string, []string, error) {
	reader := bufio.NewReader(r)
	_, _ = fmt.Fprint(w, "Decision [a]pprove/[r]eject/[s]kip: ")
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", notes, err
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "a", "approve", "approved":
		text = "approved"
	case "r", "reject", "rejected":
		text = "rejected"
	case "s", "skip", "":
		return "skip", notes, nil
	default:
		return "", notes, fmt.Errorf("unknown fixture decision %q", strings.TrimSpace(text))
	}
	_, _ = fmt.Fprint(w, "Notes (optional, single line): ")
	note, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", notes, err
	}
	if note = strings.TrimSpace(note); note != "" {
		notes = append(notes, note)
	}
	return text, notes, nil
}

func writeFixtureFiles(sourceRepo, name string, opts fixtureOptions, status string, snapshot fixtureSnapshot) error {
	fixtureDir := fixtureOutputDir(opts.CorpusDir, name, opts)
	goldenDir := filepath.Join(fixtureDir, "golden")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		return err
	}
	if !opts.NoCopy {
		dstRepo := filepath.Join(fixtureDir, "repo")
		if !samePath(sourceRepo, dstRepo) {
			if err := os.RemoveAll(dstRepo); err != nil {
				return err
			}
			if err := copyFixtureRepo(sourceRepo, dstRepo); err != nil {
				return err
			}
		}
	}
	snapshotPath := filepath.Join(goldenDir, "snapshot.json")
	if err := writePrettyJSON(snapshotPath, snapshot); err != nil {
		return err
	}
	manifest := fixtureManifest{
		SchemaVersion:  fixtureSchemaVersion,
		Name:           name,
		Status:         status,
		Language:       taxonomyValue(opts.Language),
		Domain:         taxonomyValue(opts.Domain),
		Framework:      taxonomyValue(opts.Framework),
		Type:           taxonomyValue(opts.Type),
		Notes:          fixtureManifestNotes(filepath.Join(fixtureDir, "fixture.json"), opts.Notes),
		ReviewStatus:   opts.ReviewStatus,
		Accuracy:       opts.Accuracy,
		ReviewComments: sortedReviewComments(opts.ReviewComments),
		ReviewedAt:     opts.ReviewedAt,
		SourcePath:     sourceRepo,
		RepoPath:       "repo",
		SnapshotPath:   filepath.ToSlash(filepath.Join("golden", "snapshot.json")),
		UpdatedAt:      time.Now().UTC(),
	}
	if opts.NoCopy {
		manifest.RepoPath = ""
	}
	inferFixtureTaxonomy(&manifest, opts.CorpusDir, fixtureDir)
	return writePrettyJSON(filepath.Join(fixtureDir, "fixture.json"), manifest)
}

func fixtureOutputDir(corpusDir, name string, opts fixtureOptions) string {
	parts := []string{taxonomyValue(opts.Language), taxonomyValue(opts.Domain), taxonomyValue(opts.Framework), taxonomyValue(opts.Type)}
	for _, part := range parts {
		if part == "" {
			return filepath.Join(corpusDir, name)
		}
	}
	return filepath.Join(append([]string{corpusDir}, parts...)...)
}

func taxonomyValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return fixtureName(value, value)
}

func inferFixtureTaxonomy(manifest *fixtureManifest, corpusDir, fixtureDir string) {
	if manifest == nil {
		return
	}
	rel, err := filepath.Rel(corpusDir, fixtureDir)
	if err != nil {
		return
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 4 {
		return
	}
	if manifest.Language == "" {
		manifest.Language = parts[0]
	}
	if manifest.Domain == "" {
		manifest.Domain = parts[1]
	}
	if manifest.Framework == "" {
		manifest.Framework = parts[2]
	}
	if manifest.Type == "" {
		manifest.Type = parts[3]
	}
}

func fixtureManifestNotes(path string, next []string) []string {
	var existing fixtureManifest
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	out := append([]string{}, existing.Notes...)
	for _, note := range next {
		note = strings.TrimSpace(note)
		if note != "" {
			out = append(out, note)
		}
	}
	return out
}

func writePrettyJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func copyFixtureRepo(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "node_modules" || name == ".next" || name == "dist" || name == "build") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func sortedJSONStrings(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return sortedStrings(values)
}

func sortedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
