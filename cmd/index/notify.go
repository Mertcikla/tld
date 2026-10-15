package index

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// notifyLive is best effort: persistence succeeds even without a running viewer.
// Reconnecting subscribers always receive the latest persisted scene.
func (e *engine) notifyLive(ctx context.Context) {
	if e.repoID == "" {
		return
	}
	addresses := map[string]struct{}{}
	if registry, err := localserver.LoadProcessRegistry(); err == nil {
		for _, process := range registry.Processes {
			if process.Kind == localserver.ProcessKindServer && filepath.Clean(process.DataDir) == filepath.Clean(e.dataDir) && localserver.IsRunning(process.PID) {
				addresses[process.Addr] = struct{}{}
			}
		}
	}
	// Also supports dev servers that do not register a process record.
	if len(addresses) == 0 {
		options := workspace.ResolveServeOptions(e.global, "", "")
		addresses[localserver.ResolveAddr(localserver.ServeOptions{Host: options.Host, Port: options.Port})] = struct{}{}
	}
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	for addr := range addresses {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
			host = "127.0.0.1"
		}
		client := codeindexv1connect.NewCodeIndexServiceClient(http.DefaultClient, "http://"+net.JoinHostPort(host, port)+"/api")
		_, _ = client.PublishLiveImpact(notifyCtx, connect.NewRequest(&pb.PublishLiveImpactRequest{RepositoryId: e.repoID}))
	}
}
