package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/proto"
)

func TestRepositorySettingsExternalImportsDefaultOff(t *testing.T) {
	ctx := context.Background()
	ws, routes := newTestServerWithOptions(t, uuid.New(), nil, Options{Config: workspace.DefaultConfig()})
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	id := graph.RepositoryID(root)
	snap := &pb.Snapshot{Id: id + "-snap", RepositoryId: id, CreatedUnix: 100}
	if err := idx.Publish(ctx, root, snap, graph.NewGraph(id, snap.Id)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(server.Client(), server.URL+"/api")
	settings, err := client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
	if err != nil {
		t.Fatal(err)
	}
	if settings.Msg.GetEffectiveMap().GetIncludeExternalImports() || settings.Msg.GetMapDefaults().GetIncludeExternalImports() {
		t.Fatalf("external imports must default off: %v", settings.Msg)
	}
	saved, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id, Overrides: &pb.RepositoryMapConfiguration{IncludeExternalImports: proto.Bool(true)}}))
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Msg.GetEffectiveMap().GetIncludeExternalImports() || saved.Msg.GetMapOverrides().GetIncludeExternalImports() != true {
		t.Fatalf("enable override not persisted: %v", saved.Msg)
	}
	saved, err = client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id, Overrides: &pb.RepositoryMapConfiguration{IncludeExternalImports: proto.Bool(false)}}))
	if err != nil {
		t.Fatalf("explicit false override rejected: %v", err)
	}
	if saved.Msg.GetEffectiveMap().GetIncludeExternalImports() || saved.Msg.GetMapOverrides().IncludeExternalImports == nil {
		t.Fatalf("explicit false override not persisted: %v", saved.Msg)
	}
}

func TestRepositorySettingsOverridesAndRemotes(t *testing.T) {
	ctx := context.Background()
	cfg := workspace.DefaultConfig()
	cfg.Map.Grouping.Resolution = 1.5
	ws, routes := newTestServerWithOptions(t, uuid.New(), nil, Options{Config: cfg})
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	id := graph.RepositoryID(root)
	otherRoot := t.TempDir()
	for _, repoRoot := range []string{root, otherRoot} {
		repoID := graph.RepositoryID(repoRoot)
		snap := &pb.Snapshot{Id: repoID + "-snap", RepositoryId: repoID, CreatedUnix: 100}
		if err := idx.Publish(ctx, repoRoot, snap, graph.NewGraph(repoID, snap.Id)); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(routes)
	defer server.Close()
	client := codeindexv1connect.NewRepositoryServiceClient(server.Client(), server.URL+"/api")
	settings, err := client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
	if err != nil || !settings.Msg.IsGit || len(settings.Msg.Remotes) != 0 || settings.Msg.MapDefaults.GetResolution() != 1.5 {
		t.Fatalf("defaults: %v %v", settings, err)
	}
	saved, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id, Overrides: &pb.RepositoryMapConfiguration{MaxLeafFiles: proto.Uint32(12)}}))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Msg.EffectiveMap.GetMaxLeafFiles() != 12 || saved.Msg.EffectiveMap.GetResolution() != 1.5 || saved.Msg.MapOverrides.Resolution != nil {
		t.Fatalf("override presence/inheritance lost: %v", saved.Msg)
	}
	other, err := client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: graph.RepositoryID(otherRoot)}))
	if err != nil || other.Msg.EffectiveMap.GetMaxLeafFiles() != 40 || other.Msg.MapOverrides.MaxLeafFiles != nil || other.Msg.IsGit {
		t.Fatalf("settings leaked into another repository: %v %v", other, err)
	}
	if _, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id, Overrides: &pb.RepositoryMapConfiguration{MinRootGroups: proto.Uint32(30)}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("accepted inverted inherited bounds: %v", err)
	}
	cfg.Map.Grouping.Resolution = 2
	settings, err = client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
	if err != nil || settings.Msg.EffectiveMap.GetResolution() != 2 || settings.Msg.EffectiveMap.GetMaxLeafFiles() != 12 {
		t.Fatalf("global change did not flow through inheritance: %v %v", settings, err)
	}
	remote := &pb.RepositoryRemote{Name: "origin", FetchUrls: []string{"https://example.com/repo.git", "git@example.com:repo.git"}, PushUrls: []string{"ssh://git@example.com/push.git"}}
	saved, err = client.UpdateRepositoryRemote(ctx, connect.NewRequest(&pb.UpdateRepositoryRemoteRequest{RepositoryId: id, Remote: remote}))
	if err != nil || len(saved.Msg.Remotes) != 1 || !proto.Equal(saved.Msg.Remotes[0], remote) {
		t.Fatalf("remote roundtrip: %v %v", saved, err)
	}
	configPath := filepath.Join(root, ".git", "config")
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateRepositoryRemote(ctx, connect.NewRequest(&pb.UpdateRepositoryRemoteRequest{RepositoryId: id, Remote: &pb.RepositoryRemote{Name: "origin", FetchUrls: []string{"https://example.com/changed.git"}}})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("ignored Git config lock: %v", err)
	}
	unchanged, err := os.ReadFile(configPath)
	if err != nil || string(unchanged) != string(original) {
		t.Fatal("failed update changed Git config")
	}
	if err := os.Remove(configPath + ".lock"); err != nil {
		t.Fatal(err)
	}
	remote.PushUrls = nil
	saved, err = client.UpdateRepositoryRemote(ctx, connect.NewRequest(&pb.UpdateRepositoryRemoteRequest{RepositoryId: id, Remote: remote}))
	if err != nil || len(saved.Msg.Remotes[0].PushUrls) != 0 {
		t.Fatalf("push inheritance not restored: %v %v", saved, err)
	}
	push, err := repositoryGit(ctx, root, "remote", "get-url", "--push", "origin")
	if err != nil || push != remote.FetchUrls[0]+"\n" {
		t.Fatalf("Git did not inherit fetch URL: %q %v", push, err)
	}
	if _, err := client.UpdateRepositoryRemote(ctx, connect.NewRequest(&pb.UpdateRepositoryRemoteRequest{RepositoryId: id, Remote: &pb.RepositoryRemote{Name: "--bad", FetchUrls: remote.FetchUrls}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("accepted option-like remote name: %v", err)
	}
	saved, err = client.UpdateRepositoryRemote(ctx, connect.NewRequest(&pb.UpdateRepositoryRemoteRequest{RepositoryId: id, Remote: remote, Remove: true}))
	if err != nil || len(saved.Msg.Remotes) != 0 {
		t.Fatalf("remove remote: %v %v", saved, err)
	}
	if _, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id})); err != nil {
		t.Fatal(err)
	}
	settings, err = client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
	if err != nil || settings.Msg.EffectiveMap.GetMaxLeafFiles() != 40 || settings.Msg.MapOverrides.MaxLeafFiles != nil {
		t.Fatalf("reset overrides: %v %v", settings, err)
	}
	if _, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id, Overrides: &pb.RepositoryMapConfiguration{MaxRootGroups: proto.Uint32(10)}})); err != nil {
		t.Fatal(err)
	}
	cfg.Map.Grouping.MinRootGroups = 15
	settings, err = client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
	if err != nil || settings.Msg.MapValidationError == "" {
		t.Fatalf("conflicting inherited defaults made settings inaccessible: %v %v", settings, err)
	}
	if _, err := client.UpdateRepositoryMapConfiguration(ctx, connect.NewRequest(&pb.UpdateRepositoryMapConfigurationRequest{RepositoryId: id})); err != nil {
		t.Fatalf("could not repair conflicted settings: %v", err)
	}
	if err := idx.DeleteRepository(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("deleted settings still accessible: %v", err)
	}
}
