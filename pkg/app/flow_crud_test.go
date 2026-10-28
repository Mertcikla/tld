package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func flowTreeFixture() []FlowStep {
	return []FlowStep{
		{ID: "start", Title: "Start → 日本語", Description: "First line\nsecond line", ElementIDs: []int32{999, 888}, ConnectorIDs: []int32{777, 666}},
		{ID: "choice", Branches: &FlowBranches{Mode: "alternate", Paths: []FlowPath{
			{ID: "empty", Name: "Empty path"},
			{ID: "chosen", Name: "Continue", Steps: []FlowStep{
				{ID: "nested", Title: "Concurrent", Branches: &FlowBranches{Mode: "parallel", Paths: []FlowPath{
					{ID: "left", Name: "Left", Steps: []FlowStep{{ID: "link", GoToFlowID: "missing-flow", ConnectorIDs: []int32{555}}}},
					{ID: "right", Name: "Right", Steps: []FlowStep{{ID: "note", Description: "Text only"}}},
				}}},
			}},
		}}},
		{ID: "end", Title: "Finish"},
	}
}

func equalFlowTrees(a, b []FlowStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.ID != y.ID || x.Title != y.Title || x.Description != y.Description || x.GoToFlowID != y.GoToFlowID || !slices.Equal(x.ElementIDs, y.ElementIDs) || !slices.Equal(x.ConnectorIDs, y.ConnectorIDs) || (x.Branches == nil) != (y.Branches == nil) {
			return false
		}
		if x.Branches != nil {
			if x.Branches.Mode != y.Branches.Mode || len(x.Branches.Paths) != len(y.Branches.Paths) {
				return false
			}
			for j, p := range x.Branches.Paths {
				q := y.Branches.Paths[j]
				if p.ID != q.ID || p.Name != q.Name || !equalFlowTrees(p.Steps, q.Steps) {
					return false
				}
			}
		}
	}
	return true
}

func assertFlowTableCounts(t *testing.T, s *Store, flows, steps, paths int) {
	t.Helper()
	for _, entry := range []struct {
		table string
		want  int
	}{{"flows", flows}, {"flow_steps", steps}, {"flow_paths", paths}} {
		var count int
		if err := s.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+entry.table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != entry.want {
			t.Fatalf("%s rows=%d want=%d", entry.table, count, entry.want)
		}
	}
}

func TestFlowNormalizedCRUD(t *testing.T) { runFlowNormalizedCRUD(t, openAppStore(t)) }
func runFlowNormalizedCRUD(t *testing.T, s *Store) {
	ctx := WithTenantOrgID(t.Context(), uuid.New())
	view, err := s.CreateView(ctx, "Flow CRUD", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := FlowRecord{ViewID: view.ID, Name: "Original", Description: "A story", Steps: flowTreeFixture()}
	saved, err := s.SaveFlow(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 1, 6, 4)
	check := func(want FlowRecord) {
		t.Helper()
		rows, err := s.ListFlows(ctx, view.ID)
		if err != nil || len(rows) != 1 {
			t.Fatalf("list: %v %v", rows, err)
		}
		got := rows[0]
		if got.ID != want.ID || got.Name != want.Name || got.Description != want.Description || got.ViewID != want.ViewID || got.Revision != want.Revision || !equalFlowTrees(got.Steps, want.Steps) {
			t.Fatalf("stored flow differs:\ngot=%+v\nwant=%+v", got, want)
		}
	}
	check(saved)
	// Updating changes order, both kinds of highlights, controls, path order and
	// ownership of nested steps, while preserving their stable IDs.
	update := saved
	update.Name = "Updated"
	update.Description = "Changed"
	update.Steps = flowTreeFixture()
	update.Steps[0], update.Steps[2] = update.Steps[2], update.Steps[0]
	update.Steps[0].Title = "Changed finish"
	update.Steps[2].ElementIDs = []int32{123}
	update.Steps[2].ConnectorIDs = []int32{456}
	branch := update.Steps[1].Branches
	branch.Mode = "parallel"
	branch.Paths[0], branch.Paths[1] = branch.Paths[1], branch.Paths[0]
	branch.Paths[0].Name = "Renamed"
	branch.Paths[1].Steps = []FlowStep{{ID: "added", Title: "New step", GoToFlowID: "also-missing"}}
	update.Steps = append(update.Steps, FlowStep{ID: "last", Description: "New root"})
	updated, err := s.SaveFlow(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatal("revision not incremented")
	}
	check(updated)
	assertFlowTableCounts(t, s, 1, 8, 4)
	if _, err := s.SaveFlow(ctx, saved); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if err := s.DeleteFlow(ctx, saved.ID, saved.ViewID, saved.Revision); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	check(updated)
	other, err := s.CreateView(ctx, "Another view", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved := updated
	moved.ViewID = other.ID
	if _, err := s.SaveFlow(ctx, moved); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("reassigned home view: %v", err)
	}
	foreign := WithTenantOrgID(t.Context(), uuid.New())
	if _, err := s.SaveFlow(foreign, updated); err == nil {
		t.Fatal("foreign update accepted")
	}
	if err := s.DeleteFlow(foreign, updated.ID, updated.ViewID, updated.Revision); err == nil {
		t.Fatal("foreign delete accepted")
	}
	rows, err := s.ListFlows(foreign, 0)
	if err != nil || len(rows) != 0 {
		t.Fatalf("foreign read: %v %v", rows, err)
	}
	check(updated)
	// Replacing branches with text removes every old descendant/path row.
	empty := updated
	empty.Steps = []FlowStep{{ID: "choice", Description: "Now text only"}}
	empty, err = s.SaveFlow(ctx, empty)
	if err != nil {
		t.Fatal(err)
	}
	check(empty)
	assertFlowTableCounts(t, s, 1, 1, 0)
	empty.Steps = nil
	empty, err = s.SaveFlow(ctx, empty)
	if err != nil {
		t.Fatal(err)
	}
	check(empty)
	assertFlowTableCounts(t, s, 1, 0, 0)
	empty.Steps = flowTreeFixture()
	empty, err = s.SaveFlow(ctx, empty)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFlow(ctx, empty.ID, view.ID, empty.Revision); err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 0, 0, 0)
	if err := s.DeleteFlow(ctx, empty.ID, view.ID, empty.Revision); !errors.Is(err, ErrFlowConflict) {
		t.Fatalf("repeated delete: %v", err)
	}
	// Step and path IDs are local to each flow, so duplication can retain them.
	first, err := s.SaveFlow(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveFlow(ctx, original); err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 2, 12, 8)
	if err := s.DeleteFlow(ctx, first.ID, view.ID, first.Revision); err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 1, 6, 4)
	if err := s.DeleteView(ctx, view.ID); err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 0, 0, 0)
}

func TestFlowSaveRollback(t *testing.T) { runFlowSaveRollback(t, openAppStore(t)) }
func runFlowSaveRollback(t *testing.T, s *Store) {
	ctx := t.Context()
	view, err := s.CreateView(ctx, "Rollback", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "Original", Steps: flowTreeFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if s.Dialect() == dbrepo.DialectPostgres {
		for _, query := range []string{`CREATE FUNCTION reject_flow_path() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name = 'reject' THEN RAISE EXCEPTION 'injected path failure'; END IF; RETURN NEW; END $$`, `CREATE TRIGGER reject_flow_path BEFORE INSERT ON flow_paths FOR EACH ROW EXECUTE FUNCTION reject_flow_path()`} {
			if _, err := s.DB().ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		if _, err := s.DB().ExecContext(ctx, `CREATE TRIGGER reject_flow_path BEFORE INSERT ON flow_paths WHEN NEW.name = 'reject' BEGIN SELECT RAISE(ABORT, 'injected path failure'); END`); err != nil {
			t.Fatal(err)
		}
	}
	bad := saved
	bad.Name = "Must roll back"
	bad.Steps = flowTreeFixture()
	bad.Steps[1].Branches.Paths[1].Name = "reject"
	if _, err := s.SaveFlow(ctx, bad); err == nil {
		t.Fatal("injected step-tree write succeeded")
	}
	bad.ID = ""
	bad.Revision = 0
	if _, err := s.SaveFlow(ctx, bad); err == nil {
		t.Fatal("injected create succeeded")
	}
	rows, err := s.ListFlows(ctx, 0)
	if err != nil || len(rows) != 1 || rows[0].Name != saved.Name || rows[0].Revision != saved.Revision || !equalFlowTrees(rows[0].Steps, saved.Steps) {
		t.Fatalf("failed save changed original: %v %v", rows, err)
	}
	assertFlowTableCounts(t, s, 1, 6, 4)
	// An enclosing store transaction must still own the final commit.
	sentinel := errors.New("rollback outer transaction")
	err = s.RunInTransaction(ctx, func(ctx context.Context, tx *Store) error {
		_, err := tx.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "Nested", Steps: []FlowStep{{ID: "nested"}}})
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 1, 6, 4)
}

func TestFlowConcurrentRevisionAndSnapshots(t *testing.T) {
	runFlowConcurrentRevisionAndSnapshots(t, openAppStore(t))
}
func runFlowConcurrentRevisionAndSnapshots(t *testing.T, s *Store) {
	ctx := t.Context()
	view, err := s.CreateView(ctx, "Concurrent", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "Initial", Steps: flowTreeFixture()})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := range 2 {
		go func() {
			<-start
			row := saved
			row.Name = fmt.Sprintf("writer-%d", i)
			row.Steps = []FlowStep{{ID: row.Name, Title: row.Name}}
			_, err := s.SaveFlow(ctx, row)
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ErrFlowConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent writers: successes=%d conflicts=%d", successes, conflicts)
	}
	rows, err := s.ListFlows(ctx, 0)
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 || rows[0].Steps[0].ID != rows[0].Name {
		t.Fatalf("partial concurrent write: %v %v", rows, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Go(func() {
		current := rows[0]
		for i := range 20 {
			current.Name = fmt.Sprintf("version-%d", i)
			current.Steps = []FlowStep{{ID: current.Name}}
			var err error
			current, err = s.SaveFlow(ctx, current)
			if err != nil {
				errs <- err
				return
			}
		}
	})
	wg.Go(func() {
		for range 40 {
			snapshot, err := s.ListFlows(ctx, 0)
			if err != nil {
				errs <- err
				return
			}
			if len(snapshot) != 1 || len(snapshot[0].Steps) != 1 || snapshot[0].Name != snapshot[0].Steps[0].ID {
				errs <- errors.New("inconsistent envelope and step snapshot")
				return
			}
		}
	})
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestFlowBulkEmptyPathsAndInvalidTrees(t *testing.T) {
	runFlowBulkEmptyPathsAndInvalidTrees(t, openAppStore(t))
}
func runFlowBulkEmptyPathsAndInvalidTrees(t *testing.T, s *Store) {
	ctx := t.Context()
	view, err := s.CreateView(ctx, "Bulk", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	row := FlowRecord{ViewID: view.ID, Name: "Bulk"}
	for i := range 600 {
		row.Steps = append(row.Steps, FlowStep{ID: fmt.Sprintf("step-%d", i), Branches: &FlowBranches{Mode: "alternate", Paths: []FlowPath{{ID: fmt.Sprintf("a-%d", i), Name: "A"}, {ID: fmt.Sprintf("b-%d", i), Name: "B"}}}})
	}
	saved, err := s.SaveFlow(ctx, row)
	if err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 1, 600, 1200)
	rows, err := s.ListFlows(ctx, 0)
	if err != nil || len(rows) != 1 || !equalFlowTrees(rows[0].Steps, row.Steps) {
		t.Fatalf("bulk round trip: %v", err)
	}
	for _, steps := range [][]FlowStep{{{ID: "same"}, {ID: "same"}}, {{ID: ""}}, {{ID: "step", Branches: &FlowBranches{Mode: "alternate", Paths: []FlowPath{{ID: "step", Name: "A"}, {ID: "b", Name: "B"}}}}}, {{ID: "step", GoToFlowID: "other", Branches: &FlowBranches{Mode: "parallel", Paths: []FlowPath{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}}}}} {
		invalid := saved
		invalid.Steps = steps
		if _, err := s.SaveFlow(ctx, invalid); err == nil {
			t.Fatalf("accepted invalid tree: %+v", steps)
		}
	}
	rows, err = s.ListFlows(ctx, 0)
	if err != nil || rows[0].Revision != 1 || !equalFlowTrees(rows[0].Steps, row.Steps) {
		t.Fatalf("invalid tree changed saved flow: %v", err)
	}
}

func TestFlowSchemaConstraints(t *testing.T) { runFlowSchemaConstraints(t, openAppStore(t)) }
func runFlowSchemaConstraints(t *testing.T, s *Store) {
	ctx := t.Context()
	view, err := s.CreateView(ctx, "Constraints", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "A", Steps: flowTreeFixture()})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveFlow(ctx, FlowRecord{ViewID: view.ID, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	// Composite references must never attach B's rows to A's step/path IDs.
	invalid := []string{
		fmt.Sprintf(`INSERT INTO flow_steps(flow_id,id,path_id,position) VALUES ('%s','orphan','chosen',0)`, b.ID),
		fmt.Sprintf(`INSERT INTO flow_paths(flow_id,id,step_id,position,name) VALUES ('%s','orphan','choice',0,'Orphan')`, b.ID),
		fmt.Sprintf(`INSERT INTO flow_steps(flow_id,id,position) VALUES ('%s','duplicate-position',0)`, a.ID),
		fmt.Sprintf(`INSERT INTO flow_steps(flow_id,id,position,branch_mode,go_to_flow_id) VALUES ('%s','conflicting',99,'alternate','target')`, a.ID),
	}
	for _, query := range invalid {
		if _, err := s.DB().ExecContext(ctx, query); err == nil {
			t.Fatalf("accepted invalid relationship: %s", query)
		}
	}
	// The path relationship itself cascades, independently of whole-flow saves.
	if _, err := s.QueryDB().ExecContext(ctx, "DELETE FROM flow_steps WHERE flow_id = ? AND id = 'choice'", a.ID); err != nil {
		t.Fatal(err)
	}
	assertFlowTableCounts(t, s, 2, 2, 0)
	var column string
	query := `SELECT name FROM pragma_table_info('flows') WHERE name = 'document'`
	if s.Dialect() == dbrepo.DialectPostgres {
		query = `SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'flows' AND column_name = 'document'`
	}
	if err := s.DB().QueryRowContext(ctx, query).Scan(&column); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("whole-flow document column check: %q %v", column, err)
	}
	if rows, err := s.ListFlows(ctx, 0); err != nil || len(rows) != 2 || strings.Join([]string{rows[0].Name, rows[1].Name}, "") == "" {
		t.Fatalf("post-cascade read: %v %v", rows, err)
	}
}
