package localserver_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/internal/localserver"
)

func TestBootstrapCreatesDatabaseAndReadyEndpoint(t *testing.T) {
	app, err := localserver.Bootstrap(t.TempDir())
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}

	if _, err := os.Stat(app.DBPath); err != nil {
		t.Fatalf("stat db path %s: %v", app.DBPath, err)
	}
	if got, want := filepath.Base(app.DBPath), "tld.db"; got != want {
		t.Fatalf("db filename = %q, want %q", got, want)
	}

	server := httptest.NewServer(app.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/ready")
	if err != nil {
		t.Fatalf("get ready endpoint: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close response body: %v", closeErr)
		}
	}()

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("ready status = %d, want %d", got, want)
	}
}

func TestAddrFromEnvPrefersTLDAddrAndFallsBackToPort(t *testing.T) {
	t.Setenv("PORT", "9091")
	if got, want := localserver.AddrFromEnv(), "127.0.0.1:9091"; got != want {
		t.Fatalf("addr from PORT = %q, want %q", got, want)
	}

	t.Setenv("TLD_ADDR", "0.0.0.0:7000")
	if got, want := localserver.AddrFromEnv(), "0.0.0.0:7000"; got != want {
		t.Fatalf("addr from TLD_ADDR = %q, want %q", got, want)
	}
}
