//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	workspacecfg "github.com/mertcikla/tld/v2/internal/workspace"
	coreapi "github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
)

type postgresAPIClient struct {
	workspace diagv1connect.WorkspaceServiceClient
	org       diagv1connect.OrgServiceClient
}

func TestPostgresWorkspaceServiceCriticalPathE2E(t *testing.T) {
	dsn := requirePostgresDSN(t)
	store := openPostgresLocalStore(t, dsn)
	db := store.DB()

	ctx := context.Background()
	orgA := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	orgB := uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
	clientA := newPostgresAPIClient(t, store, orgA)
	clientB := newPostgresAPIClient(t, store, orgB)

	root, err := clientA.workspace.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		Name:       "Critical Path",
		LevelLabel: ptr("System"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	apiElement, err := clientA.workspace.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name:        "API",
		Kind:        ptr("service"),
		Description: ptr("Handles requests"),
		TechnologyLinks: []*diagv1.TechnologyLink{{
			Type:          "catalog",
			Slug:          ptr("go"),
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
		Tags:     []string{"critical", "backend"},
		Repo:     ptr("mertcikla/tld"),
		Branch:   ptr("main"),
		Language: ptr("go"),
		FilePath: ptr("cmd/api/main.go"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	dbElement, err := clientA.workspace.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name: "Database",
		Kind: ptr("database"),
		Tags: []string{"critical", "storage"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	childView, err := clientA.workspace.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		Name:           "API Internals",
		OwnerElementId: ptr(apiElement.Msg.GetElement().GetId()),
		LevelLabel:     ptr("Component"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if childView.Msg.GetView().GetOwnerElementId() != apiElement.Msg.GetElement().GetId() {
		t.Fatalf("child owner = %d, want API element", childView.Msg.GetView().GetOwnerElementId())
	}

	if _, err := clientA.workspace.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: apiElement.Msg.GetElement().GetId(),
		PositionX: 120,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}
	upsertedPlacement, err := clientA.workspace.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: apiElement.Msg.GetElement().GetId(),
		PositionX: 150,
		PositionY: 160,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if upsertedPlacement.Msg.GetPlacement().GetPositionX() != 150 || upsertedPlacement.Msg.GetPlacement().GetPositionY() != 160 {
		t.Fatalf("upserted placement = %+v, want updated coordinates", upsertedPlacement.Msg.GetPlacement())
	}
	assertPlacementCount(t, db, root.Msg.GetView().GetId(), apiElement.Msg.GetElement().GetId(), 1)
	if err := updatePlacement(ctx, clientA.workspace, root.Msg.GetView().GetId(), apiElement.Msg.GetElement().GetId(), 175, 195); err != nil {
		t.Fatal(err)
	}
	if _, err := clientA.workspace.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: dbElement.Msg.GetElement().GetId(),
		PositionX: 460,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}

	connector, err := clientA.workspace.CreateConnector(ctx, connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          root.Msg.GetView().GetId(),
		SourceElementId: apiElement.Msg.GetElement().GetId(),
		TargetElementId: dbElement.Msg.GetElement().GetId(),
		Label:           ptr("reads"),
		Direction:       "forward",
		Style:           "bezier",
		Tags:            []string{"runtime"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	updatedConnector, err := clientA.workspace.UpdateConnector(ctx, connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId:  connector.Msg.GetConnector().GetId(),
		Label:        ptr("writes"),
		Relationship: ptr("SQL"),
		Style:        "smoothstep",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updatedConnector.Msg.GetConnector().GetLabel() != "writes" ||
		updatedConnector.Msg.GetConnector().GetRelationship() != "SQL" ||
		updatedConnector.Msg.GetConnector().GetStyle() != "smoothstep" ||
		updatedConnector.Msg.GetConnector().GetDirection() != "forward" ||
		updatedConnector.Msg.GetConnector().GetSourceElementId() != apiElement.Msg.GetElement().GetId() ||
		updatedConnector.Msg.GetConnector().GetTargetElementId() != dbElement.Msg.GetElement().GetId() {
		t.Fatalf("updated connector = %+v, want partial update with stable endpoints", updatedConnector.Msg.GetConnector())
	}

	renamedElement, err := clientA.workspace.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId: apiElement.Msg.GetElement().GetId(),
		Name:      "API Gateway",
		Tags:      []string{"critical", "backend"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if renamedElement.Msg.GetElement().GetDescription() != "Handles requests" ||
		renamedElement.Msg.GetElement().GetTechnologyLinks()[0].GetSlug() != "go" {
		t.Fatalf("renamed element = %+v, want omitted fields preserved", renamedElement.Msg.GetElement())
	}
	updatedElement, err := clientA.workspace.UpdateElement(ctx, connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId:   apiElement.Msg.GetElement().GetId(),
		Description: ptr("Updated through the shared API"),
		TechnologyLinks: []*diagv1.TechnologyLink{{
			Type:          "catalog",
			Slug:          ptr("go"),
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
		Tags: []string{"critical", "edge"},
		Url:  ptr("https://example.com/runbook"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updatedElement.Msg.GetElement().GetName() != "API Gateway" ||
		updatedElement.Msg.GetElement().GetDescription() != "Updated through the shared API" ||
		updatedElement.Msg.GetElement().GetTags()[1] != "edge" {
		t.Fatalf("updated element = %+v, want renamed element with updated metadata", updatedElement.Msg.GetElement())
	}
	assertRawElementJSON(t, db, apiElement.Msg.GetElement().GetId(), []string{"critical", "edge"}, "go")
	assertRFC3339Timestamps(t, db, "elements", apiElement.Msg.GetElement().GetId())

	search, err := clientA.workspace.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Search: "gateway"}))
	if err != nil {
		t.Fatal(err)
	}
	if search.Msg.GetPagination().GetTotalCount() != 1 || search.Msg.GetElements()[0].GetId() != apiElement.Msg.GetElement().GetId() {
		t.Fatalf("search response = %+v, want only updated API element", search.Msg)
	}

	tagsA, err := clientA.org.ListTagColors(ctx, connect.NewRequest(&diagv1.ListTagColorsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"critical", "backend", "edge", "storage", "runtime"} {
		assertTagPresent(t, tagsA.Msg.GetTags(), tag)
	}

	rootB, err := clientB.workspace.CreateView(ctx, connect.NewRequest(&diagv1.CreateViewRequest{
		Name:       "Critical Path",
		LevelLabel: ptr("System"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	otherTenantElement, err := clientB.workspace.CreateElement(ctx, connect.NewRequest(&diagv1.CreateElementRequest{
		Name: "API Gateway",
		Kind: ptr("service"),
		Tags: []string{"tenant-b"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientB.workspace.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    rootB.Msg.GetView().GetId(),
		ElementId: otherTenantElement.Msg.GetElement().GetId(),
		PositionX: 30,
		PositionY: 40,
	})); err != nil {
		t.Fatal(err)
	}
	tagsB, err := clientB.org.ListTagColors(ctx, connect.NewRequest(&diagv1.ListTagColorsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	assertTagPresent(t, tagsB.Msg.GetTags(), "tenant-b")
	assertTagAbsent(t, tagsB.Msg.GetTags(), "critical")
	assertTagAbsent(t, tagsA.Msg.GetTags(), "tenant-b")

	expectConnectCode(t, connect.CodeNotFound, func() error {
		_, err := clientB.workspace.GetElement(ctx, connect.NewRequest(&diagv1.GetElementRequest{
			ElementId: apiElement.Msg.GetElement().GetId(),
		}))
		return err
	})
	expectConnectCode(t, connect.CodeNotFound, func() error {
		_, err := clientB.workspace.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
			ViewId: root.Msg.GetView().GetId(),
		}))
		return err
	})
	expectConnectCode(t, connect.CodeNotFound, func() error {
		_, err := clientA.workspace.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
			ViewId:    root.Msg.GetView().GetId(),
			ElementId: otherTenantElement.Msg.GetElement().GetId(),
			PositionX: 1,
			PositionY: 2,
		}))
		return err
	})
	expectConnectCode(t, connect.CodeNotFound, func() error {
		_, err := clientA.workspace.CreateConnector(ctx, connect.NewRequest(&diagv1.CreateConnectorRequest{
			ViewId:          root.Msg.GetView().GetId(),
			SourceElementId: apiElement.Msg.GetElement().GetId(),
			TargetElementId: otherTenantElement.Msg.GetElement().GetId(),
			Label:           ptr("cross tenant"),
			Direction:       "forward",
			Style:           "bezier",
		}))
		return err
	})

	tenantSearch, err := clientB.workspace.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Search: "gateway"}))
	if err != nil {
		t.Fatal(err)
	}
	if tenantSearch.Msg.GetPagination().GetTotalCount() != 1 || tenantSearch.Msg.GetElements()[0].GetId() != otherTenantElement.Msg.GetElement().GetId() {
		t.Fatalf("tenant B search = %+v, want only tenant B element", tenantSearch.Msg)
	}

	reloaded, err := clientA.workspace.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
		ViewId:         root.Msg.GetView().GetId(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Msg.GetContent().GetPlacements()) != 2 || len(reloaded.Msg.GetContent().GetConnectors()) != 1 {
		t.Fatalf("reloaded content = %+v, want two placements and one connector", reloaded.Msg.GetContent())
	}
	assertPlacementPosition(t, reloaded.Msg.GetContent().GetPlacements(), apiElement.Msg.GetElement().GetId(), 175, 195)

	workspace, err := clientA.workspace.GetWorkspace(ctx, connect.NewRequest(&diagv1.GetWorkspaceRequest{
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Msg.GetTotalCount() != 2 ||
		!workspaceHasView(workspace.Msg.GetViews(), root.Msg.GetView().GetId()) ||
		!workspaceHasView(workspace.Msg.GetViews(), childView.Msg.GetView().GetId()) ||
		workspaceHasView(workspace.Msg.GetViews(), rootB.Msg.GetView().GetId()) {
		t.Fatalf("workspace response = %+v, want only tenant A root and child views", workspace.Msg)
	}
	assertStoredOrgID(t, db, "views", root.Msg.GetView().GetId(), orgA)
	assertStoredOrgID(t, db, "views", rootB.Msg.GetView().GetId(), orgB)
	assertStoredOrgID(t, db, "elements", apiElement.Msg.GetElement().GetId(), orgA)
	assertStoredOrgID(t, db, "elements", otherTenantElement.Msg.GetElement().GetId(), orgB)
	assertStoredOrgID(t, db, "connectors", connector.Msg.GetConnector().GetId(), orgA)

	if _, err := clientA.workspace.DeleteConnector(ctx, connect.NewRequest(&diagv1.DeleteConnectorRequest{
		ConnectorId: connector.Msg.GetConnector().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	afterConnectorDelete, err := clientA.workspace.ListConnectors(ctx, connect.NewRequest(&diagv1.ListConnectorsRequest{
		ViewId: root.Msg.GetView().GetId(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterConnectorDelete.Msg.GetConnectors()) != 0 {
		t.Fatalf("connectors after delete = %+v, want none", afterConnectorDelete.Msg.GetConnectors())
	}

	if _, err := clientA.workspace.DeletePlacement(ctx, connect.NewRequest(&diagv1.DeletePlacementRequest{
		ViewId:    root.Msg.GetView().GetId(),
		ElementId: dbElement.Msg.GetElement().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	afterPlacementDelete, err := clientA.workspace.GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{
		ViewId:         root.Msg.GetView().GetId(),
		IncludeContent: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterPlacementDelete.Msg.GetContent().GetPlacements()) != 1 {
		t.Fatalf("placements after delete = %+v, want one remaining placement", afterPlacementDelete.Msg.GetContent().GetPlacements())
	}
	library, err := clientA.workspace.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Search: "Database"}))
	if err != nil {
		t.Fatal(err)
	}
	if library.Msg.GetPagination().GetTotalCount() != 1 {
		t.Fatalf("database library search total = %d, want element preserved after placement delete", library.Msg.GetPagination().GetTotalCount())
	}

	if _, err := clientA.workspace.DeleteElement(ctx, connect.NewRequest(&diagv1.DeleteElementRequest{
		ElementId: dbElement.Msg.GetElement().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	expectConnectCode(t, connect.CodeNotFound, func() error {
		_, err := clientA.workspace.GetElement(ctx, connect.NewRequest(&diagv1.GetElementRequest{
			ElementId: dbElement.Msg.GetElement().GetId(),
		}))
		return err
	})
}

func requirePostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("TLD_TEST_POSTGRES_URL not set")
	}
	return dsn
}

func openPostgresLocalStore(t *testing.T, dsn string) *app.Store {
	t.Helper()
	resetPostgresSchema(t, dsn)
	cfg := &workspacecfg.Config{
		Database: workspacecfg.DatabaseConfig{
			Driver:      "postgres",
			DatabaseURL: dsn,
		},
	}
	store, err := localstore.OpenLocal(context.Background(), cfg, filepath.Join(t.TempDir(), "data"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newPostgresAPIClient(t *testing.T, store *app.Store, orgID uuid.UUID) postgresAPIClient {
	t.Helper()
	apiStore := coreapi.NewAPIStore(store)
	workspacePath, workspaceHandler := diagv1connect.NewWorkspaceServiceHandler(&coreapi.WorkspaceService{
		Store: apiStore,
	})
	orgPath, orgHandler := diagv1connect.NewOrgServiceHandler(&coreapi.OrgService{
		Store: apiStore,
	})
	withWorkspace := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(coreapi.WithWorkspaceID(r.Context(), orgID)))
		})
	}
	mux := http.NewServeMux()
	mux.Handle("/api"+workspacePath, http.StripPrefix("/api", withWorkspace(workspaceHandler)))
	mux.Handle("/api"+orgPath, http.StripPrefix("/api", withWorkspace(orgHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return postgresAPIClient{
		workspace: diagv1connect.NewWorkspaceServiceClient(server.Client(), server.URL+"/api"),
		org:       diagv1connect.NewOrgServiceClient(server.Client(), server.URL+"/api"),
	}
}

func updatePlacement(ctx context.Context, client diagv1connect.WorkspaceServiceClient, viewID, elementID int32, x, y float64) error {
	_, err := client.UpdatePlacementPosition(ctx, connect.NewRequest(&diagv1.UpdatePlacementPositionRequest{
		ViewId:    viewID,
		ElementId: elementID,
		PositionX: x,
		PositionY: y,
	}))
	return err
}

func assertPlacementCount(t *testing.T, db *sql.DB, viewID, elementID int32, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM placements WHERE view_id = $1 AND element_id = $2`, viewID, elementID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("placement count for view %d element %d = %d, want %d", viewID, elementID, got, want)
	}
}

func assertPlacementPosition(t *testing.T, placements []*diagv1.PlacedElement, elementID int32, x, y float64) {
	t.Helper()
	for _, placement := range placements {
		if placement.GetElementId() != elementID {
			continue
		}
		if placement.GetPositionX() != x || placement.GetPositionY() != y {
			t.Fatalf("placement for element %d = (%v,%v), want (%v,%v)", elementID, placement.GetPositionX(), placement.GetPositionY(), x, y)
		}
		return
	}
	t.Fatalf("placement for element %d not found in %+v", elementID, placements)
}

func assertRawElementJSON(t *testing.T, db *sql.DB, elementID int32, wantTags []string, wantTechSlug string) {
	t.Helper()
	var rawTags, rawTech string
	if err := db.QueryRowContext(context.Background(), `SELECT tags, technology_connectors FROM elements WHERE id = $1`, elementID).Scan(&rawTags, &rawTech); err != nil {
		t.Fatal(err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
		t.Fatalf("unmarshal tags %q: %v", rawTags, err)
	}
	for _, want := range wantTags {
		if !stringSliceContains(tags, want) {
			t.Fatalf("tags = %v, want %q", tags, want)
		}
	}
	var links []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(rawTech), &links); err != nil {
		t.Fatalf("unmarshal technology links %q: %v", rawTech, err)
	}
	if len(links) != 1 || links[0].Slug != wantTechSlug {
		t.Fatalf("technology links = %+v, want slug %q", links, wantTechSlug)
	}
}

func assertRFC3339Timestamps(t *testing.T, db *sql.DB, table string, id int32) {
	t.Helper()
	var createdAt, updatedAt string
	if err := db.QueryRowContext(context.Background(), `SELECT created_at, updated_at FROM `+table+` WHERE id = $1`, id).Scan(&createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{createdAt, updatedAt} {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s timestamp %q is not RFC3339: %v", table, value, err)
		}
	}
}

func assertTagPresent(t *testing.T, tags map[string]*diagv1.Tag, name string) {
	t.Helper()
	tag, ok := tags[name]
	if !ok {
		t.Fatalf("tag %q missing from %+v", name, tags)
	}
	if tag.GetColor() == "" {
		t.Fatalf("tag %q color is empty", name)
	}
}

func assertTagAbsent(t *testing.T, tags map[string]*diagv1.Tag, name string) {
	t.Helper()
	if _, ok := tags[name]; ok {
		t.Fatalf("tag %q unexpectedly present in %+v", name, tags)
	}
}

func expectConnectCode(t *testing.T, want connect.Code, fn func() error) {
	t.Helper()
	err := fn()
	if err == nil {
		t.Fatalf("expected connect code %s, got nil", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("connect code = %s, want %s: %v", got, want, err)
	}
}

func workspaceHasView(views []*diagv1.View, id int32) bool {
	for _, view := range views {
		if view.GetId() == id || workspaceHasView(view.GetChildren(), id) {
			return true
		}
	}
	return false
}

func assertStoredOrgID(t *testing.T, db *sql.DB, table string, id int32, want uuid.UUID) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(), `SELECT org_id::text FROM `+table+` WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want.String() {
		t.Fatalf("%s id %d org_id = %q, want %q", table, id, got, want.String())
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func ptr[T any](value T) *T {
	return &value
}

func TestPostgresSnapshotMembershipE2E(t *testing.T) {
	dsn := requirePostgresDSN(t)
	sq := openPostgresLocalStore(t, dsn)
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	ctx := context.Background()
	src := &graph.Source{Path: "a.go", Hash: "hash", Text: []byte("func A() {}")}
	first := graph.NewGraph("repo-membership", "snapshot-first")
	first.Sources[src.Path] = src
	fact := first.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(src.Text)), string(src.Text), "func A()", nil)
	edge := first.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, fact.Id, "", "external", fact.Anchor, nil)
	second := graph.NewGraph(first.RepositoryID, "snapshot-second")
	second.Sources[src.Path] = src
	adopted := second.AdoptFactAnchored(fact, fact.Anchor, fact.BodyHash, fact.Signature)
	second.AdoptEdgeFact(edge, adopted.Id, "", "external")
	for _, g := range []*graph.Graph{first, second} {
		snap := &pb.Snapshot{Id: g.SnapshotID, RepositoryId: g.RepositoryID, Sources: []*pb.SourceFile{{Path: src.Path, Hash: src.Hash, Size: uint64(len(src.Text))}}}
		for range 2 {
			if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
				t.Fatal(err)
			}
			stored, err := idx.LoadGraph(ctx, snap.Id)
			if err != nil {
				t.Fatal(err)
			}
			if len(stored.Facts) != 1 || len(stored.EdgeFacts) != 1 {
				t.Fatalf("roundtrip counts: %d/%d", len(stored.Facts), len(stored.EdgeFacts))
			}
		}
	}
	if err := idx.SaveCompletedMap(ctx, first.RepositoryID, &pb.CompletedMap{Result: &pb.MapResult{RunId: "membership-map", SnapshotId: first.SnapshotID}, ConfigHash: "cfg"}); err != nil {
		t.Fatal(err)
	}
	active, err := idx.ActiveMap(ctx, first.RepositoryID)
	if err != nil || active == nil || active.Result.RunId != "membership-map" {
		t.Fatalf("active map: %+v %v", active, err)
	}
	if err := idx.DeleteSnapshot(ctx, first.SnapshotID); err != nil {
		t.Fatal(err)
	}
	stored, err := idx.LoadGraph(ctx, second.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Facts[fact.Id] == nil {
		t.Fatal("shared entities were lost")
	}
}
