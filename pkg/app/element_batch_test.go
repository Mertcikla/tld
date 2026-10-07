package app

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"
)

func TestCreateElementsBatch(t *testing.T) {
	s := openAppStore(t)
	ctx := context.Background()
	inputs := []LibraryElement{{Name: " first ", Tags: []string{"go"}, FilePath: new("a.go"), BypassNoiseGate: true}, {Name: "second", Tags: []string{"go", "source"}, Description: new("description")}}
	rows, err := s.CreateElements(ctx, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID == rows[1].ID || rows[0].Name != "first" || rows[1].Name != "second" {
		t.Fatalf("wrong order/IDs: %+v", rows)
	}
	for _, row := range rows {
		persisted, err := s.ElementByID(ctx, row.ID)
		if err != nil || !reflect.DeepEqual(row, persisted) {
			t.Fatalf("batch readback mismatch: %+v %+v %v", row, persisted, err)
		}
	}
	tags, err := s.Tags(ctx)
	if err != nil || len(tags) != 2 {
		t.Fatalf("tag union: %v %v", tags, err)
	}
}

func TestCreateElementsBatchRollback(t *testing.T) {
	s := openAppStore(t)
	ctx := context.Background()
	_, err := s.db.Exec(`CREATE TRIGGER fail_batch BEFORE INSERT ON elements WHEN NEW.name = 'fail' BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.CreateElements(ctx, []LibraryElement{{Name: "first", Tags: []string{"new"}}, {Name: "fail"}})
	if err == nil || len(rows) != 0 {
		t.Fatal("expected atomic failure", rows, err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM elements`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial element batch", count, err)
	}
	tags, err := s.Tags(ctx)
	if err != nil || len(tags) != 0 {
		t.Fatal("partial tags", tags, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = s.CreateElements(canceled, []LibraryElement{{Name: "canceled"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

func TestCreateElementsTenant(t *testing.T) {
	s := openAppStore(t)
	orgID := uuid.New()
	ctx := WithTenantOrgID(context.Background(), orgID)
	rows, err := s.CreateElements(ctx, []LibraryElement{{Name: "one"}, {Name: "two"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		var got string
		if err := s.db.QueryRow(`SELECT org_id FROM elements WHERE id = ?`, row.ID).Scan(&got); err != nil || got != orgID.String() {
			t.Fatalf("tenant scope: %q %v", got, err)
		}
	}
}

func TestPlacementBatchMatchesSequential(t *testing.T) {
	s := openAppStore(t)
	ctx := context.Background()
	rows, err := s.CreateElements(ctx, []LibraryElement{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "outside"}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.CreateView(ctx, "source", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sequential, err := s.CreateView(ctx, "sequential", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := s.CreateView(ctx, "batch", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{0, 1}, {1, 2}, {1, 1}, {0, 3}} {
		if _, err := s.CreateConnector(ctx, Connector{ViewID: source.ID, SourceElementID: rows[pair[0]].ID, TargetElementID: rows[pair[1]].ID, Label: new("link"), Description: new("description"), Tags: []string{"edge"}, Direction: "forward"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, view := range []int64{sequential.ID, batch.ID} {
		if _, err := s.AddPlacement(ctx, view, rows[0].ID, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	inputs := []ElementPlacement{{ElementID: rows[1].ID, PositionX: 1}, {ElementID: rows[2].ID, PositionX: 2}, {ElementID: rows[1].ID, PositionX: 3}}
	for _, p := range inputs {
		if _, err := s.AddPlacement(ctx, sequential.ID, p.ElementID, p.PositionX, p.PositionY); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddPlacements(ctx, batch.ID, inputs); err != nil {
		t.Fatal(err)
	}
	normalize := func(viewID int64) []Connector {
		connectors, err := s.Connectors(ctx, viewID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range connectors {
			connectors[i].ID = 0
			connectors[i].ViewID = 0
			connectors[i].CreatedAt = ""
			connectors[i].UpdatedAt = ""
		}
		sort.Slice(connectors, func(i, j int) bool {
			if connectors[i].SourceElementID != connectors[j].SourceElementID {
				return connectors[i].SourceElementID < connectors[j].SourceElementID
			}
			return connectors[i].TargetElementID < connectors[j].TargetElementID
		})
		return connectors
	}
	// The batch sees both original and sequential-view connectors, just as
	// AddPlacement would. Each source connector is copied once.
	got := normalize(batch.ID)
	want := normalize(sequential.ID)
	if len(want) != 3 || len(got) != 6 {
		t.Fatalf("related connector inclusion: sequential=%d batch=%d", len(want), len(got))
	}
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i*2]) {
			t.Fatalf("connector fields differ: %+v %+v", want[i], got[i*2])
		}
	}
	placements, err := s.ElementPlacements(ctx, batch.ID)
	if err != nil || len(placements) != 3 {
		t.Fatalf("deduplicated placements: %+v %v", placements, err)
	}
	for _, p := range placements {
		if p.ElementID == rows[1].ID && p.PositionX != 3 {
			t.Fatal("last position did not win", p)
		}
	}
}
