package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestAPIStoreTenantScopedCounts(t *testing.T) {
	store, err := app.OpenStore(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	apiStore := NewAPIStore(store)
	orgA := uuid.New()
	orgB := uuid.New()
	ctxA := WithWorkspaceID(context.Background(), orgA)
	ctxB := WithWorkspaceID(context.Background(), orgB)

	viewA, err := apiStore.CreateView(ctxA, orgA, nil, "Tenant A", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	sourceA, err := apiStore.CreateElement(ctxA, orgA, ElementInput{Name: "Source A"})
	if err != nil {
		t.Fatal(err)
	}
	targetA, err := apiStore.CreateElement(ctxA, orgA, ElementInput{Name: "Target A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apiStore.CreateConnector(ctxA, orgA, ConnectorInput{
		ViewID:    viewA.GetId(),
		SourceID:  sourceA.GetId(),
		TargetID:  targetA.GetId(),
		Direction: "forward",
		Style:     "bezier",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := apiStore.CreateView(ctxB, orgB, nil, "Tenant B", nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := apiStore.CreateElement(ctxB, orgB, ElementInput{Name: "Only B"}); err != nil {
		t.Fatal(err)
	}

	views, elements, connectors, err := apiStore.GetWorkspaceResourceCounts(context.Background(), orgA)
	if err != nil {
		t.Fatal(err)
	}
	if views != 1 || elements != 2 || connectors != 1 {
		t.Fatalf("org A counts = views:%d elements:%d connectors:%d, want 1/2/1", views, elements, connectors)
	}
}
