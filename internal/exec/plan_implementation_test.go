package exec_test

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"context"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
	"path/filepath"
	"testing"
)

func TestExistingLocalRunnerHonorsLaterLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	runner, err := exec.NewRunner(workspace.Config{}, exec.TargetLocal, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	st, err := app.OpenStore(filepath.Join(dir, "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := api.WithWorkspaceID(context.Background(), uuid.Nil)
	svc := &api.PlanService{Store: st, Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx) }}
	p, err := svc.CreatePlan(ctx, connect.NewRequest(&pb.CreatePlanRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.LockImplementation(ctx, connect.NewRequest(&pb.LockImplementationRequest{PlanId: p.Msg.Plan.Id}))
	if err != nil {
		t.Fatal(err)
	}
	element, err := runner.CreateElement(ctx, api.ElementInput{Name: "Staged"})
	if err != nil {
		t.Fatal(err)
	}
	if element.Id < 1000000000 {
		t.Fatal("runner bypassed lock", element)
	}
	live, _, err := st.Elements(ctx, 0, 0, "")
	if err != nil || len(live) != 0 {
		t.Fatalf("live element created: %v %v", live, err)
	}
	projected, err := runner.ListElements(ctx, "")
	if err != nil || len(projected) != 1 {
		t.Fatalf("staged read failed: %v %v", projected, err)
	}
	if err = runner.DeleteElement(ctx, element.Id); err != nil {
		t.Fatal(err)
	}
	projected, err = runner.ListElements(ctx, "")
	if err != nil || len(projected) != 0 {
		t.Fatalf("staged removal failed: %v %v", projected, err)
	}
}
