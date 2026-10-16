package index

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestNotifyLiveDiscoversOnlyMatchingDataDirectory(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_ADDR", "127.0.0.1:1")
	received := make(chan string, 2)
	server := httptest.NewServer(connect.NewUnaryHandler(codeindexv1connect.CodeIndexServicePublishLiveImpactProcedure, func(_ context.Context, req *connect.Request[pb.PublishLiveImpactRequest]) (*connect.Response[emptypb.Empty], error) {
		received <- req.Msg.RepositoryId
		return connect.NewResponse(&emptypb.Empty{}), nil
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := localserver.RegisterProcess(localserver.ProcessRecord{Kind: localserver.ProcessKindServer, PID: os.Getpid(), DataDir: dir, Addr: strings.TrimPrefix(server.URL, "http://")}); err != nil {
		t.Fatal(err)
	}
	eng := &engine{repoID: "repo", dataDir: dir, global: &workspace.Config{Serve: workspace.ServeConfig{Host: "127.0.0.1", Port: "1"}}}
	eng.notifyLive(context.Background())
	select {
	case id := <-received:
		if id != "repo" {
			t.Fatalf("repository = %q", id)
		}
	default:
		t.Fatal("CLI did not notify registered server")
	}
	eng.dataDir = t.TempDir()
	eng.notifyLive(context.Background())
	select {
	case <-received:
		t.Fatal("notified server using another database")
	default:
	}
}
