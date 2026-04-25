package localserver_test

import (
	"encoding/json"
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
	if !app.InitializedData {
		t.Fatal("bootstrap should report initialized data when creating a new database")
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

	var body struct {
		OK        bool `json:"ok"`
		Resources struct {
			Views      int `json:"views"`
			Elements   int `json:"elements"`
			Connectors int `json:"connectors"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if !body.OK {
		t.Fatalf("ready response ok = false")
	}
	if body.Resources.Views < 0 || body.Resources.Elements < 0 || body.Resources.Connectors < 0 {
		t.Fatalf("ready response contains negative resources: %+v", body.Resources)
	}
	if body.Resources.Views != app.Resources.Views || body.Resources.Elements != app.Resources.Elements || body.Resources.Connectors != app.Resources.Connectors {
		t.Fatalf("ready resources = %+v, want bootstrap resources %+v", body.Resources, app.Resources)
	}
}

func TestBootstrapReportsExistingDatabase(t *testing.T) {
	dir := t.TempDir()
	first, err := localserver.Bootstrap(dir)
	if err != nil {
		t.Fatalf("bootstrap first app: %v", err)
	}
	if !first.InitializedData {
		t.Fatal("first bootstrap should report initialized data")
	}

	second, err := localserver.Bootstrap(dir)
	if err != nil {
		t.Fatalf("bootstrap second app: %v", err)
	}
	if second.InitializedData {
		t.Fatal("second bootstrap should report existing data")
	}
	if second.Resources.Views < 0 || second.Resources.Elements < 0 || second.Resources.Connectors < 0 {
		t.Fatalf("second bootstrap contains negative resources: %+v", second.Resources)
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
