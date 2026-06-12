package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/encoding/protojson"
)

func openAdapterTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	sqliteStore, err := Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteStore.Legacy().Close() })
	return sqliteStore
}

func insertAdapterTestView(t *testing.T, sqliteStore *SQLiteStore, id int32, name string) {
	t.Helper()
	if _, err := sqliteStore.DB().ExecContext(context.Background(), `
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (?, NULL, ?, NULL, 'System', 0, 'now', 'now')
	`, id, name); err != nil {
		t.Fatal(err)
	}
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for git status tests")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func TestElementToProtoPreservesPrimaryIconMetadata(t *testing.T) {
	technology := "JavaScript"
	element := elementToProto(app.LibraryElement{
		ID:         1,
		Name:       "Web",
		Technology: &technology,
		TechnologyConnectors: []app.TechnologyConnector{{
			Type:          "catalog",
			Slug:          "javascript",
			Label:         "JavaScript",
			IsPrimaryIcon: true,
		}},
	}, uuid.Nil)

	data, err := protojson.Marshal(element)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"technology":"JavaScript"`) {
		t.Fatalf("response body = %s, want technology field", body)
	}
	if !strings.Contains(body, `"isPrimaryIcon":true`) {
		t.Fatalf("response body = %s, want primary icon metadata", body)
	}
	if element.GetLogoUrl() != "/icons/javascript.svg" {
		t.Fatalf("logo url = %q, want derived primary technology icon", element.GetLogoUrl())
	}
}

func TestPlacedElementToProtoPreservesPrimaryIconMetadata(t *testing.T) {
	placement := placedElementToProto(app.PlacedElement{
		ID:        1,
		ViewID:    1,
		ElementID: 1,
		Name:      "Web",
		TechnologyConnectors: []app.TechnologyConnector{{
			Type:          "catalog",
			Slug:          "javascript",
			Label:         "JavaScript",
			IsPrimaryIcon: true,
		}},
	})

	data, err := protojson.Marshal(placement)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"isPrimaryIcon":true`) {
		t.Fatalf("response body = %s, want primary icon metadata", body)
	}
	if placement.GetLogoUrl() != "/icons/javascript.svg" {
		t.Fatalf("logo url = %q, want derived primary technology icon", placement.GetLogoUrl())
	}
}

func TestElementToProtoResolvesLegacyCatalogIconSlug(t *testing.T) {
	element := elementToProto(app.LibraryElement{
		ID:   1,
		Name: "API",
		TechnologyConnectors: []app.TechnologyConnector{{
			Type:          "catalog",
			Slug:          "golang",
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
	}, uuid.Nil)

	if element.GetLogoUrl() != "/icons/go.svg" {
		t.Fatalf("logo url = %q, want legacy slug resolved to devicon SVG", element.GetLogoUrl())
	}
}

func TestElementToProtoDoesNotInventRemovedCatalogIcon(t *testing.T) {
	element := elementToProto(app.LibraryElement{
		ID:   1,
		Name: "Cloud",
		TechnologyConnectors: []app.TechnologyConnector{{
			Type:          "catalog",
			Slug:          "aws-amazon-ec2-instances",
			Label:         "EC2 Instances",
			IsPrimaryIcon: true,
		}},
	}, uuid.Nil)

	if element.LogoUrl != nil {
		t.Fatalf("logo url = %q, want no derived icon for removed catalog slug", element.GetLogoUrl())
	}
}

func TestElementToProtoPreservesExplicitLogoClear(t *testing.T) {
	emptyLogo := ""
	element := elementToProto(app.LibraryElement{
		ID:      1,
		Name:    "Web",
		LogoURL: &emptyLogo,
		TechnologyConnectors: []app.TechnologyConnector{{
			Type:          "catalog",
			Slug:          "javascript",
			Label:         "JavaScript",
			IsPrimaryIcon: true,
		}},
	}, uuid.Nil)

	if element.LogoUrl == nil || element.GetLogoUrl() != "" {
		t.Fatalf("logo url = %v, want explicit empty clear preserved", element.LogoUrl)
	}
}

func TestGetWorkspaceResourceCountsUsesTableCounts(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)

	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(name, tags, technology_connectors, created_at, updated_at)
		VALUES
			('A', '[]', '[]', 'now', 'now'),
			('B', '[]', '[]', 'now', 'now');
		INSERT INTO views(owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (1, 'A view', NULL, 'Service', 2, 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (1, 1, 0, 0, 'now', 'now'), (2, 2, 10, 10, 'now', 'now');
		INSERT INTO connectors(view_id, source_element_id, target_element_id, direction, style, created_at, updated_at)
		VALUES (1, 1, 2, 'forward', 'bezier', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	views, elements, connectors, err := NewAPIAdapter(sqliteStore).GetWorkspaceResourceCounts(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if views != 2 || elements != 2 || connectors != 1 {
		t.Fatalf("counts = views:%d elements:%d connectors:%d, want 2/2/1", views, elements, connectors)
	}
}

func TestGetViewsFiltersDirectChildrenByParentViewID(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)

	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES
			(10, 'Service', '[]', '[]', 'now', 'now'),
			(11, 'Component', '[]', '[]', 'now', 'now');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES
			(20, 10, 'Service view', NULL, 'Service', 2, 'now', 'now'),
			(21, 11, 'Component view', NULL, 'Component', 3, 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(1, 10, 0, 0, 'now', 'now'),
			(20, 11, 10, 10, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	parentID := int32(1)
	children, total, err := NewAPIAdapter(sqliteStore).GetViews(context.Background(), uuid.Nil, &parentID, nil, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(children) != 1 || children[0].GetId() != 20 {
		t.Fatalf("root children = total:%d views:%v, want only view 20", total, children)
	}

	parentID = 20
	children, total, err = NewAPIAdapter(sqliteStore).GetViews(context.Background(), uuid.Nil, &parentID, nil, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(children) != 1 || children[0].GetId() != 21 {
		t.Fatalf("nested children = total:%d views:%v, want only view 21", total, children)
	}
}

func TestApplyPlanAutoLayoutsUnpositionedPlacements(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)

	resp, err := NewAPIAdapter(sqliteStore).ApplyPlan(context.Background(), uuid.Nil, &diagv1.ApplyPlanRequest{
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
			{Ref: "db", Name: "DB", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
		},
		Connectors: []*diagv1.PlanConnector{{
			Ref:              "api-db",
			ViewRef:          "root",
			SourceElementRef: "api",
			TargetElementRef: "db",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetCreatedPlacements()) != 2 {
		t.Fatalf("created placements = %d, want 2", len(resp.GetCreatedPlacements()))
	}
	first := resp.GetCreatedPlacements()[0]
	second := resp.GetCreatedPlacements()[1]
	if first.GetPositionX() == second.GetPositionX() && first.GetPositionY() == second.GetPositionY() {
		t.Fatalf("placements overlapped at (%v, %v)", first.GetPositionX(), first.GetPositionY())
	}

	rows, err := sqliteStore.DB().QueryContext(context.Background(), `SELECT position_x, position_y FROM placements ORDER BY element_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var positions [][2]float64
	for rows.Next() {
		var x, y float64
		if err := rows.Scan(&x, &y); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, [2]float64{x, y})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 || positions[0] == positions[1] {
		t.Fatalf("stored positions = %v, want distinct layout positions", positions)
	}
}

func TestApplyPlanCreatesMissingConnectorWithPlannedID(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	connectorID := int32(77)

	resp, err := NewAPIAdapter(sqliteStore).ApplyPlan(context.Background(), uuid.Nil, &diagv1.ApplyPlanRequest{
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
			{Ref: "db", Name: "DB", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
		},
		Connectors: []*diagv1.PlanConnector{{
			Id:               &connectorID,
			Ref:              "api-db",
			ViewRef:          "root",
			SourceElementRef: "api",
			TargetElementRef: "db",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetConnectorResults()[0].GetId(); got != 77 {
		t.Fatalf("connector result id = %d, want 77", got)
	}
	if _, err := NewAPIAdapter(sqliteStore).GetConnector(context.Background(), 77, uuid.Nil); err != nil {
		t.Fatalf("connector 77 was not created: %v", err)
	}
}

func TestApplyPlanPreservesExplicitPlacementCoordinates(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	x, y := 42.0, 84.0

	resp, err := NewAPIAdapter(sqliteStore).ApplyPlan(context.Background(), uuid.Nil, &diagv1.ApplyPlanRequest{
		Elements: []*diagv1.PlanElement{{
			Ref:  "api",
			Name: "API",
			Placements: []*diagv1.PlanViewPlacement{{
				ParentRef: "root",
				PositionX: &x,
				PositionY: &y,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetCreatedPlacements()) != 1 {
		t.Fatalf("created placements = %d, want 1", len(resp.GetCreatedPlacements()))
	}
	placement := resp.GetCreatedPlacements()[0]
	if placement.GetPositionX() != x || placement.GetPositionY() != y {
		t.Fatalf("placement = (%v, %v), want (%v, %v)", placement.GetPositionX(), placement.GetPositionY(), x, y)
	}
}

func TestApplyPlanDefaultsBypassNoiseGateTrueAndPreservesExplicitFalse(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	explicitFalse := false

	resp, err := NewAPIAdapter(sqliteStore).ApplyPlan(context.Background(), uuid.Nil, &diagv1.ApplyPlanRequest{
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API"},
			{Ref: "manual", Name: "Manual", BypassNoiseGate: &explicitFalse},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetCreatedElements()) != 2 {
		t.Fatalf("created elements = %d, want 2", len(resp.GetCreatedElements()))
	}

	items, _, err := NewAPIAdapter(sqliteStore).ListElements(context.Background(), uuid.Nil, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.GetName()] = item.GetBypassNoiseGate()
	}
	if !got["API"] {
		t.Fatalf("CLI/apply element bypass = %v, want true by default", got["API"])
	}
	if got["Manual"] {
		t.Fatalf("explicit false bypass = %v, want false preserved", got["Manual"])
	}
}

func TestListElementsMapsSearchPaginationAndViewMetadata(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, kind, description, tags, technology_connectors, created_at, updated_at)
		VALUES
			(10, 'API', 'service', 'Public runtime API', '["runtime"]', '[]', 'now', '2026-01-02T00:00:00Z'),
			(11, 'Worker', 'service', 'Background for API jobs', '["runtime"]', '[]', 'now', '2026-01-03T00:00:00Z');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (20, 10, 'API view', NULL, 'Service', 2, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	items, total, err := NewAPIAdapter(sqliteStore).ListElements(context.Background(), uuid.Nil, 1, 0, "API")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].GetId() != 10 {
		t.Fatalf("filtered elements = total:%d items:%+v, want only API", total, items)
	}
	if !items[0].GetHasView() || items[0].GetViewLabel() != "Service" {
		t.Fatalf("view metadata = has:%v label:%q, want Service child view", items[0].GetHasView(), items[0].GetViewLabel())
	}

	items, total, err = NewAPIAdapter(sqliteStore).ListElements(context.Background(), uuid.Nil, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 1 || items[0].GetId() != 11 {
		t.Fatalf("paginated elements = total:%d items:%+v, want Worker after API in name order", total, items)
	}
}

func TestViewMarkdownManagedLifecycle(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir)
	ctx := context.Background()

	if _, err := sqliteStore.DB().ExecContext(ctx, `
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (20, NULL, 'System Context', NULL, 'System', 0, 'now', 'now')
	`); err != nil {
		t.Fatal(err)
	}

	initialContent := "# System Context\n\nInitial notes.\n"
	if _, err := adapter.CreateViewMarkdown(ctx, 20, uuid.Nil, nil, &initialContent, "", nil); err != nil {
		t.Fatal(err)
	}

	markdown, content, err := adapter.GetViewMarkdown(ctx, 20, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if !markdown.GetIsManaged() {
		t.Fatalf("managed = %v, want true", markdown.GetIsManaged())
	}
	if content != initialContent {
		t.Fatalf("content = %q, want %q", content, initialContent)
	}
	managedPath := filepath.Join(dataDir, markdown.GetPath())
	raw, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != initialContent {
		t.Fatalf("file content = %q, want %q", string(raw), initialContent)
	}

	updatedContent := "# Updated\n\nSaved notes.\n"
	updatedMarkdown, err := adapter.SaveViewMarkdown(ctx, 20, uuid.Nil, updatedContent, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if updatedMarkdown.GetPath() != markdown.GetPath() {
		t.Fatalf("path = %q, want %q", updatedMarkdown.GetPath(), markdown.GetPath())
	}
	raw, err = os.ReadFile(managedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != updatedContent {
		t.Fatalf("saved file content = %q, want %q", string(raw), updatedContent)
	}

	if _, err := adapter.UnlinkViewMarkdown(ctx, 20, uuid.Nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed file still exists after unlink: %v", err)
	}
	if _, _, err := adapter.GetViewMarkdown(ctx, 20, uuid.Nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after unlink err = %v, want sql.ErrNoRows", err)
	}
}

func TestViewMarkdownLinkReadsRelativeFilesFromDataDir(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir)
	ctx := context.Background()

	if _, err := sqliteStore.DB().ExecContext(ctx, `
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (30, NULL, 'Deployment', NULL, 'System', 0, 'now', 'now')
	`); err != nil {
		t.Fatal(err)
	}

	linkedDir := filepath.Join(dataDir, "docs")
	if err := os.MkdirAll(linkedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	relPath := filepath.Join("docs", "deployment.md")
	absPath := filepath.Join(dataDir, relPath)
	linkedContent := "# Deployment\n\nLinked file.\n"
	if err := os.WriteFile(absPath, []byte(linkedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := adapter.LinkViewMarkdown(ctx, 30, uuid.Nil, relPath); err != nil {
		t.Fatal(err)
	}
	markdown, content, err := adapter.GetViewMarkdown(ctx, 30, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetIsManaged() {
		t.Fatalf("managed = %v, want false", markdown.GetIsManaged())
	}
	if markdown.GetPath() != relPath {
		t.Fatalf("path = %q, want %q", markdown.GetPath(), relPath)
	}
	if content != linkedContent {
		t.Fatalf("content = %q, want %q", content, linkedContent)
	}
}

func TestViewMarkdownLinkReadsPrivateWorkspacePathFromWorkspaceNotes(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	workspaceDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir, workspaceDir)
	ctx := context.Background()

	insertAdapterTestView(t, sqliteStore, 31, "Linked Notes")

	relPath := filepath.Join(managedViewMarkdownDir, "source-notes.md")
	absPath := filepath.Join(workspaceDir, ".tld", relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	linkedContent := "# Source notes\n\nLinked private note.\n"
	if err := os.WriteFile(absPath, []byte(linkedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := adapter.LinkViewMarkdown(ctx, 31, uuid.Nil, relPath); err != nil {
		t.Fatal(err)
	}
	markdown, content, err := adapter.GetViewMarkdown(ctx, 31, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetPath() != relPath {
		t.Fatalf("path = %q, want %q", markdown.GetPath(), relPath)
	}
	if markdown.GetSourceKind() != markdownSourceAttached {
		t.Fatalf("source kind = %s, want %s", markdown.GetSourceKind(), markdownSourceAttached)
	}
	if content != linkedContent {
		t.Fatalf("content = %q, want %q", content, linkedContent)
	}
}

func TestViewMarkdownPrivateWorkspaceCreatesUnderTLDAndReportsMissing(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	workspaceDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir, workspaceDir)
	ctx := context.Background()
	insertAdapterTestView(t, sqliteStore, 40, "System Context")

	initialContent := "# System Context\n\nPrivate notes.\n"
	if _, err := adapter.CreateViewMarkdown(ctx, 40, uuid.Nil, nil, &initialContent, "PRIVATE_WORKSPACE", nil); err != nil {
		t.Fatal(err)
	}

	markdown, content, err := adapter.GetViewMarkdown(ctx, 40, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetSourceKind() != "PRIVATE_WORKSPACE" {
		t.Fatalf("source kind = %q, want PRIVATE_WORKSPACE", markdown.GetSourceKind())
	}
	if markdown.GetGitState() != "outside_repo" {
		t.Fatalf("git state = %q, want outside_repo for private note outside a git repo", markdown.GetGitState())
	}
	if !markdown.GetExists() || !markdown.GetCanEdit() || !markdown.GetWritable() {
		t.Fatalf("metadata exists/editable/writable = %v/%v/%v, want true/true/true", markdown.GetExists(), markdown.GetCanEdit(), markdown.GetWritable())
	}
	if content != initialContent {
		t.Fatalf("content = %q, want %q", content, initialContent)
	}
	managedPath := filepath.Join(workspaceDir, ".tld", markdown.GetPath())
	if _, err := os.Stat(managedPath); err != nil {
		t.Fatalf("private workspace file was not created under .tld: %v", err)
	}

	if err := os.Remove(managedPath); err != nil {
		t.Fatal(err)
	}
	markdown, content, err = adapter.GetViewMarkdown(ctx, 40, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetExists() || markdown.GetCanEdit() || markdown.GetWritable() || content != "" {
		t.Fatalf("missing metadata/content = exists:%v can_edit:%v writable:%v content:%q, want false/false/false/empty",
			markdown.GetExists(), markdown.GetCanEdit(), markdown.GetWritable(), content)
	}
}

func TestViewMarkdownRepoCreationReportsGitState(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	workspaceDir := t.TempDir()
	runGitCommand(t, workspaceDir, "init")
	adapter := NewAPIAdapter(sqliteStore, dataDir, workspaceDir)
	ctx := context.Background()
	insertAdapterTestView(t, sqliteStore, 41, "Checkout Flow")
	insertAdapterTestView(t, sqliteStore, 42, "Ignored Flow")

	repoPath := "docs/diagrams/checkout-flow.md"
	if _, err := adapter.CreateViewMarkdown(ctx, 41, uuid.Nil, nil, nil, "REPO", &repoPath); err != nil {
		t.Fatal(err)
	}
	markdown, _, err := adapter.GetViewMarkdown(ctx, 41, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetSourceKind() != "REPO" {
		t.Fatalf("source kind = %q, want REPO", markdown.GetSourceKind())
	}
	if markdown.GetRepoRelativePath() != repoPath {
		t.Fatalf("repo relative path = %q, want %q", markdown.GetRepoRelativePath(), repoPath)
	}
	if markdown.GetGitState() != "untracked" {
		t.Fatalf("git state = %q, want untracked", markdown.GetGitState())
	}

	if err := os.WriteFile(filepath.Join(workspaceDir, ".gitignore"), []byte("docs/diagrams/ignored.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignoredPath := "docs/diagrams/ignored.md"
	if _, err := adapter.CreateViewMarkdown(ctx, 42, uuid.Nil, nil, nil, "REPO", &ignoredPath); err != nil {
		t.Fatal(err)
	}
	markdown, _, err = adapter.GetViewMarkdown(ctx, 42, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetGitState() != "ignored" {
		t.Fatalf("ignored git state = %q, want ignored", markdown.GetGitState())
	}
}

func TestViewMarkdownSaveDetectsFileVersionConflict(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir)
	ctx := context.Background()
	insertAdapterTestView(t, sqliteStore, 44, "Conflict Notes")

	initialContent := "# Conflict\n\nInitial.\n"
	if _, err := adapter.CreateViewMarkdown(ctx, 44, uuid.Nil, nil, &initialContent, "PRIVATE_APP", nil); err != nil {
		t.Fatal(err)
	}
	markdown, _, err := adapter.GetViewMarkdown(ctx, 44, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(dataDir, markdown.GetPath())
	if err := os.WriteFile(managedPath, []byte("# Conflict\n\nChanged externally with more bytes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.SaveViewMarkdown(ctx, 44, uuid.Nil, "# Conflict\n\nEditor save.\n", &markdown.FileVersion, false); !errors.Is(err, api.ErrMarkdownFileChanged) {
		t.Fatalf("save err = %v, want ErrMarkdownFileChanged", err)
	}
	if _, err := adapter.SaveViewMarkdown(ctx, 44, uuid.Nil, "# Conflict\n\nForced save.\n", &markdown.FileVersion, true); err != nil {
		t.Fatal(err)
	}
}

func TestViewMarkdownLegacyRelativePathResolvesFromDataDir(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	workspaceDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir, workspaceDir)
	ctx := context.Background()
	insertAdapterTestView(t, sqliteStore, 45, "Legacy Notes")

	relPath := filepath.Join("docs", "legacy.md")
	dataDirContent := "# Legacy\n\nData dir file.\n"
	workspaceContent := "# Legacy\n\nWorkspace file.\n"
	if err := os.MkdirAll(filepath.Join(dataDir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, relPath), []byte(dataDirContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceDir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, relPath), []byte(workspaceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.Legacy().UpsertViewMarkdown(ctx, 45, relPath, false, nowString()); err != nil {
		t.Fatal(err)
	}

	markdown, content, err := adapter.GetViewMarkdown(ctx, 45, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetSourceKind() != "LEGACY" {
		t.Fatalf("source kind = %q, want LEGACY", markdown.GetSourceKind())
	}
	if content != dataDirContent {
		t.Fatalf("content = %q, want legacy data dir content %q", content, dataDirContent)
	}
}

func TestViewMarkdownReadOnlyAttachedFileIsNotEditable(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	dataDir := t.TempDir()
	adapter := NewAPIAdapter(sqliteStore, dataDir)
	ctx := context.Background()
	insertAdapterTestView(t, sqliteStore, 46, "Read Only Notes")

	relPath := filepath.Join("docs", "read-only.md")
	absPath := filepath.Join(dataDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte("# Read only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(absPath, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(absPath, 0o644) })

	if _, err := adapter.LinkViewMarkdown(ctx, 46, uuid.Nil, relPath); err != nil {
		t.Fatal(err)
	}
	markdown, _, err := adapter.GetViewMarkdown(ctx, 46, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.GetWritable() || markdown.GetCanEdit() {
		t.Fatalf("read-only metadata writable/can_edit = %v/%v, want false/false", markdown.GetWritable(), markdown.GetCanEdit())
	}
}
func TestConnectorAdapterPreservesHandlesDefaultsAndViewFiltering(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES
			(10, 'API', '[]', '[]', 'now', 'now'),
			(11, 'DB', '[]', '[]', 'now', 'now');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (20, 10, 'API view', NULL, 'Service', 2, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}
	label := "reads"
	sourceHandle := "right"
	targetHandle := "left"
	connector, err := NewAPIAdapter(sqliteStore).CreateConnector(context.Background(), uuid.Nil, api.ConnectorInput{
		ViewID:       20,
		SourceID:     10,
		TargetID:     11,
		Label:        &label,
		Style:        "bezier",
		SourceHandle: &sourceHandle,
		TargetHandle: &targetHandle,
		Tags:         []string{"runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if connector.GetDirection() != "forward" || connector.GetStyle() != "bezier" {
		t.Fatalf("connector defaults = direction:%q style:%q, want forward/bezier", connector.GetDirection(), connector.GetStyle())
	}
	if connector.GetSourceHandle() != "right" || connector.GetTargetHandle() != "left" {
		t.Fatalf("connector handles = %q/%q, want right/left", connector.GetSourceHandle(), connector.GetTargetHandle())
	}
	if got := connector.GetTags(); len(got) != 1 || got[0] != "runtime" {
		t.Fatalf("connector tags = %v, want runtime", got)
	}

	all, err := NewAPIAdapter(sqliteStore).ListAllConnectors(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	inView, err := NewAPIAdapter(sqliteStore).ListConnectors(context.Background(), 20, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || len(inView) != 1 || all[0].GetId() != inView[0].GetId() {
		t.Fatalf("connector list mismatch: all=%+v inView=%+v", all, inView)
	}
	updated, err := NewAPIAdapter(sqliteStore).UpdateConnector(context.Background(), connector.GetId(), uuid.Nil, api.ConnectorInput{
		ViewID:   20,
		SourceID: 10,
		TargetID: 11,
		Style:    "bezier",
		Tags:     []string{"data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.GetTags(); len(got) != 1 || got[0] != "data" {
		t.Fatalf("updated connector tags = %v, want data", got)
	}
}

func TestViewAdapterPreservesAndUpdatesTags(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES (10, 'API', '[]', '[]', 'now', 'now');
		INSERT INTO views(id, owner_element_id, name, description, level_label, tags, level, created_at, updated_at)
		VALUES (20, 10, 'API view', 'Desc', 'Service', '["runtime"]', 2, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}
	adapter := NewAPIAdapter(sqliteStore)
	view, err := adapter.GetView(context.Background(), 20, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.GetTags(); len(got) != 1 || got[0] != "runtime" {
		t.Fatalf("view tags = %v, want runtime", got)
	}
	description := "Updated"
	label := "Container"
	updated, err := adapter.UpdateView(context.Background(), 20, uuid.Nil, "API internals", &description, &label, []string{"platform"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetDescription() != "Updated" || updated.GetLevelLabel() != "Container" {
		t.Fatalf("updated view = desc:%q label:%q", updated.GetDescription(), updated.GetLevelLabel())
	}
	if got := updated.GetTags(); len(got) != 1 || got[0] != "platform" {
		t.Fatalf("updated view tags = %v, want platform", got)
	}
}

func TestListAllViewLayersBatchesAndPreservesTreeOrder(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES (120, 'Service', '[]', '[]', 'now', 'now');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES
			(120, NULL, 'System', NULL, 'System', 1, 'now', 'now'),
			(121, 120, 'Service detail', NULL, 'Service', 2, 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (120, 120, 0, 0, 'now', 'now');
		INSERT INTO view_layers(id, view_id, name, tags, color, created_at, updated_at)
		VALUES
			(120, 120, 'Root A', '["api"]', '#111111', 'now', 'now'),
			(121, 120, 'Root B', '["db"]', '#222222', 'now', 'now'),
			(122, 121, 'Child A', '["worker"]', '#333333', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	layers, err := NewAPIAdapter(sqliteStore).ListAllViewLayers(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, layer := range layers {
		switch layer.GetId() {
		case 120, 121, 122:
			names = append(names, layer.GetName())
		}
	}
	if strings.Join(names, ",") != "Root A,Root B,Child A" {
		t.Fatalf("layer order = %v, want root layers before child layers", names)
	}
}
