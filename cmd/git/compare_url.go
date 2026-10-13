package git

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// compareURLBase follows the local viewer, not server_url (the sync target).
// Reading the registry does not prune it or start a server during an export.
func compareURLBase(cfg *workspace.Config, dataDir, explicit string) (string, error) {
	if explicit != "" {
		_, err := sceneurl.WebURLPrefix(explicit)
		return explicit, err
	}
	serve := workspace.ResolveServeOptions(cfg, "", "")
	if serve.PublicURL != "" {
		_, err := sceneurl.WebURLPrefix(serve.PublicURL)
		return serve.PublicURL, err
	}
	if registry, err := localserver.LoadProcessRegistry(); err == nil {
		for i := len(registry.Processes) - 1; i >= 0; i-- {
			process := registry.Processes[i]
			if process.Kind != localserver.ProcessKindServer || filepath.Clean(process.DataDir) != filepath.Clean(dataDir) || !localserver.IsRunning(process.PID) {
				continue
			}
			if base, err := comparisonServerBase(process.Addr); err == nil {
				return base, nil
			}
		}
	}
	return comparisonServerBase(localserver.ResolveAddr(localserver.ServeOptions{Host: serve.Host, Port: serve.Port}))
}

func comparisonServerBase(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("resolve comparison viewer address: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("invalid comparison viewer port %q", port)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}
