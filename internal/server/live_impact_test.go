package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestWatchLiveImpactPublishesAndReplaysPersistedScene(t *testing.T) {
	ws, _ := newTestServer(t, uuid.Nil, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := idx.Publish(ctx, "/repo", &pb.Snapshot{Id: "head", RepositoryId: "repo"}, graph.NewGraph("repo", "head")); err != nil {
		t.Fatal(err)
	}
	svc := &codeIndexService{store: idx}
	_, handler := codeindexv1connect.NewCodeIndexServiceHandler(svc)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL)
	stream, err := client.WatchLiveImpact(ctx, connect.NewRequest(&pb.WatchLiveImpactRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Receive() || stream.Msg().Scene != nil {
		t.Fatalf("initial empty state: %v", stream.Err())
	}
	save := func(version string) {
		t.Helper()
		scene := &model.Assembly{RepositoryId: "repo", ComparisonKey: version, Version: version, SchemaVersion: impact.PipelineVersion}
		if err := idx.SaveImpact(ctx, &model.Diagram{RepositoryId: "repo", ComparisonKey: "live", Version: version, Scene: scene}); err != nil {
			t.Fatal(err)
		}
	}
	publish := func() {
		t.Helper()
		if _, err := client.PublishLiveImpact(ctx, connect.NewRequest(&pb.PublishLiveImpactRequest{RepositoryId: "repo"})); err != nil {
			t.Fatal(err)
		}
	}
	save("v1")
	publish()
	if !stream.Receive() || stream.Msg().GetScene().GetComparison().GetVersion() != "v1" {
		t.Fatalf("v1 not pushed: %v", stream.Err())
	}
	publish()
	if !stream.Receive() || stream.Msg().Scene != nil {
		t.Fatalf("status update retransmitted scene: %v", stream.Err())
	}
	save("v2")
	publish()
	if !stream.Receive() || stream.Msg().GetScene().GetComparison().GetVersion() != "v2" {
		t.Fatalf("v2 not pushed: %v", stream.Err())
	}
	// A new connection recovers without depending on an event retained in memory.
	replay, err := client.WatchLiveImpact(ctx, connect.NewRequest(&pb.WatchLiveImpactRequest{RepositoryId: "repo"}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = replay.Close() }()
	if !replay.Receive() || replay.Msg().GetScene().GetComparison().GetVersion() != "v2" {
		t.Fatalf("latest scene not replayed: %v", replay.Err())
	}
}

func TestLiveImpactNotificationsAreScopedAndCoalesced(t *testing.T) {
	ws, _ := newTestServer(t, uuid.Nil, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	a := app.WithTenantOrgID(context.Background(), uuid.New())
	b := app.WithTenantOrgID(context.Background(), uuid.New())
	if err := idx.Publish(a, "/repo", &pb.Snapshot{Id: "head", RepositoryId: "repo"}, graph.NewGraph("repo", "head")); err != nil {
		t.Fatal(err)
	}
	svc := &codeIndexService{store: idx}
	updates, closeA := svc.subscribeLive(a, "repo")
	otherTenant, closeB := svc.subscribeLive(b, "repo")
	otherRepo, closeOther := svc.subscribeLive(a, "other")
	defer closeB()
	defer closeOther()
	for range 10 {
		if _, err := svc.PublishLiveImpact(a, connect.NewRequest(&pb.PublishLiveImpactRequest{RepositoryId: "repo"})); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-updates:
	default:
		t.Fatal("no notification")
	}
	select {
	case <-updates:
		t.Fatal("burst was not coalesced")
	default:
	}
	select {
	case <-otherTenant:
		t.Fatal("notification crossed tenant boundary")
	default:
	}
	select {
	case <-otherRepo:
		t.Fatal("notification crossed repository boundary")
	default:
	}
	closeA()
	svc.mu.Lock()
	remaining := len(svc.liveSubscribers)
	svc.mu.Unlock()
	if remaining != 2 {
		t.Fatalf("unsubscribe retained a subscriber: %d", remaining)
	}
}
