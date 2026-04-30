package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld"
)

func TestGetWorkspaceResourceCountsUsesTableCounts(t *testing.T) {
	sqliteStore, err := Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer sqliteStore.Legacy().Close()

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
	sqliteStore, err := Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer sqliteStore.Legacy().Close()

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
