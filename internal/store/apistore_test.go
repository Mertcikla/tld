package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld"
	"github.com/mertcikla/tld/pkg/api"
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
		VALUES (1, 1, 2, 'forward', 'solid', 'now', 'now');
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

func TestListElementsMapsSearchPaginationAndViewMetadata(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	db := sqliteStore.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, kind, description, tags, technology_connectors, created_at, updated_at)
		VALUES
			(10, 'API', 'service', 'Public runtime API', '["runtime"]', '[]', 'now', '2026-01-02T00:00:00Z'),
			(11, 'Worker', 'service', 'Background jobs', '["runtime"]', '[]', 'now', '2026-01-03T00:00:00Z');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES (20, 10, 'API view', NULL, 'Service', 2, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	items, total, err := NewAPIAdapter(sqliteStore).ListElements(context.Background(), uuid.Nil, 1, 0, "runtime")
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
	if total != 2 || len(items) != 1 || items[0].GetId() != 10 {
		t.Fatalf("paginated elements = total:%d items:%+v, want API as second updated item", total, items)
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
		Style:        "solid",
		SourceHandle: &sourceHandle,
		TargetHandle: &targetHandle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connector.GetDirection() != "forward" || connector.GetStyle() != "solid" {
		t.Fatalf("connector defaults = direction:%q style:%q, want forward/solid", connector.GetDirection(), connector.GetStyle())
	}
	if connector.GetSourceHandle() != "right" || connector.GetTargetHandle() != "left" {
		t.Fatalf("connector handles = %q/%q, want right/left", connector.GetSourceHandle(), connector.GetTargetHandle())
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
}
