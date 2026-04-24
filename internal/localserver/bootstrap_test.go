package localserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestBootstrapServesEmbeddedAppIndex(t *testing.T) {
	app, err := localserver.Bootstrap(t.TempDir())
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}

	server := httptest.NewServer(app.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get root endpoint: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close response body: %v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read root response body: %v", err)
	}

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("root status = %d, want %d; body=%q", got, want, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("root content-type = %q, want HTML", got)
	}
	if !strings.Contains(strings.ToLower(string(body)), "<!doctype html") {
		t.Fatalf("root response does not look like the embedded app index")
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
