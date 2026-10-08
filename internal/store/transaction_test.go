package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func TestTransactionIsolatesConcurrentRequest(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "commit"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			store := openAdapterTestStore(t)
			adapter := NewAPIAdapter(store)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			started := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			planErr := errors.New("plan failed")
			go func() {
				done <- adapter.RunInTransaction(ctx, func(txCtx context.Context, txStore api.Store) error {
					if _, err := txStore.CreateElement(txCtx, uuid.Nil, api.ElementInput{Name: "plan", Tags: []string{"plan-tag"}}); err != nil {
						return err
					}
					// View reads include the lazy markdown schema queries, which
					// must also use the reserved transaction connection.
					if _, err := txStore.ListViews(txCtx, uuid.Nil); err != nil {
						return err
					}
					close(started)
					select {
					case <-release:
					case <-txCtx.Done():
						return txCtx.Err()
					}
					if rollback {
						return planErr
					}
					return nil
				})
			}()
			select {
			case <-started:
			case err := <-done:
				t.Fatalf("transaction failed before concurrent request: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			waitCount := store.DB().Stats().WaitCount
			requestDone := make(chan error, 1)
			go func() {
				_, err := adapter.CreateElement(ctx, uuid.Nil, api.ElementInput{Name: "independent"})
				requestDone <- err
			}()
			// Wait for database/sql to queue the independent request rather
			// than relying on a sleep to arrange the interleaving.
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for store.DB().Stats().WaitCount == waitCount {
				select {
				case err := <-requestDone:
					t.Fatalf("request joined the open transaction: %v", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-ticker.C:
				}
			}
			close(release)
			err := <-done
			if rollback && !errors.Is(err, planErr) || !rollback && err != nil {
				t.Fatalf("transaction error = %v", err)
			}
			if err := <-requestDone; err != nil {
				t.Fatal(err)
			}
			elements, _, err := adapter.ListElements(ctx, uuid.Nil, 0, 0, "")
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if rollback {
				want = 1
			}
			if len(elements) != want || elements[0].Name != "independent" {
				t.Fatalf("elements after transaction = %v, want %d including independent request", elements, want)
			}
			tags, err := store.Tags(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := tags["plan-tag"]; exists == rollback {
				t.Fatalf("plan tag exists = %t after %s", exists, name)
			}
		})
	}
}

func TestApplyWorkspacePlanRollsBackPartialWrites(t *testing.T) {
	store := openAdapterTestStore(t)
	adapter := NewAPIAdapter(store)
	svc := &api.WorkspaceService{Store: adapter}
	_, err := svc.ApplyWorkspacePlan(context.Background(), connect.NewRequest(&diagv1.ApplyPlanRequest{
		OrgId: uuid.NewString(),
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API", HasView: true, Tags: []string{"new-tag"}},
			{Ref: "db", Name: "Database", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "api"}}},
		},
		Connectors: []*diagv1.PlanConnector{{ViewRef: "root", SourceElementRef: "api", TargetElementRef: "missing"}},
	}))
	if err == nil || !strings.Contains(err.Error(), `unknown target element ref "missing"`) {
		t.Fatalf("expected missing target error after writes, got %v", err)
	}
	for _, table := range []string{"elements", "placements", "connectors", "tags"} {
		var count int
		if err := store.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s has %d rows after failed plan", table, count)
		}
	}
	var views int
	if err := store.DB().QueryRow("SELECT COUNT(*) FROM views").Scan(&views); err != nil {
		t.Fatal(err)
	}
	if views != 1 {
		t.Fatalf("views = %d, want only bootstrap view", views)
	}
}

func TestApplyWorkspacePlanCommitsGraphAndLayout(t *testing.T) {
	store := openAdapterTestStore(t)
	adapter := NewAPIAdapter(store)
	svc := &api.WorkspaceService{Store: adapter}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := svc.ApplyWorkspacePlan(ctx, connect.NewRequest(&diagv1.ApplyPlanRequest{
		OrgId: uuid.NewString(),
		Elements: []*diagv1.PlanElement{
			{Ref: "api", Name: "API", HasView: true, Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
			{Ref: "db", Name: "Database", Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}}},
		},
		Connectors: []*diagv1.PlanConnector{{Id: new(int32(999)), Ref: "link", ViewRef: "root", SourceElementRef: "api", TargetElementRef: "db"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Msg.CreatedPlacements) != 2 || len(resp.Msg.CreatedConnectors) != 1 {
		t.Fatalf("incomplete plan response: %v", resp.Msg)
	}
	placements := resp.Msg.CreatedPlacements
	if placements[0].PositionX == placements[1].PositionX && placements[0].PositionY == placements[1].PositionY {
		t.Fatal("plan placements were not laid out")
	}
	// Query through the original adapter after commit, checking the raw
	// explicit-id connector insert and placement layout persisted together.
	connector, err := adapter.GetConnector(ctx, 999, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if connector.SourceElementId != placements[0].ElementId || connector.TargetElementId != placements[1].ElementId {
		t.Fatalf("unexpected committed connector: %v", connector)
	}
	for _, placement := range placements {
		var x, y float64
		if err := store.DB().QueryRowContext(ctx, `SELECT position_x, position_y FROM placements WHERE id = ?`, placement.Id).Scan(&x, &y); err != nil {
			t.Fatal(err)
		}
		if x != placement.PositionX || y != placement.PositionY {
			t.Fatalf("committed position = (%f, %f), response = (%f, %f)", x, y, placement.PositionX, placement.PositionY)
		}
	}
}
