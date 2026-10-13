package git

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareURLFlagsConflictBeforeIndexing(t *testing.T) {
	for _, flags := range [][]string{{"--url", "--url-local"}, {"--url", "--mermaid"}, {"--url", "--markdown"}, {"--url-local", "--mermaid"}, {"--url-local", "--markdown"}} {
		cmd := newCompareCmd()
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"nonexistent-base", "nonexistent-head"}, flags...))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("flags=%v error=%v", flags, err)
		}
		if out.Len() != 0 {
			t.Fatal("invalid invocation wrote stdout")
		}
	}
}

func TestCompareURLExportsCurrentSelectedJSON(t *testing.T) {
	dir := compareRepo(t)
	jsonText, _, err := runGitCompare(t, "compare", dir, "HEAD~1", "HEAD", "--plain", "--debug-provenance", "--max-nodes", "5")
	if err != nil {
		t.Fatal(err)
	}
	normal := &pb.ImpactScene{}
	if err := protojson.Unmarshal([]byte(jsonText), normal); err != nil {
		t.Fatal(err)
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	dataDir, err := workspace.ResolveDataDir(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	base, err := compareURLBase(cfg, dataDir, "")
	if err != nil {
		t.Fatal(err)
	}
	localWebPrefix, err := sceneurl.WebURLPrefix(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		flags  []string
		prefix string
	}{
		{[]string{"--url"}, localWebPrefix},
		{[]string{"--url", "--url-base", "https://tldiagram.com/app"}, sceneurl.WebPrefix},
		{[]string{"--url-local"}, sceneurl.LocalPrefix},
	} {
		args := append([]string{"compare", dir, "HEAD~1", "HEAD"}, test.flags...)
		args = append(args, "--plain", "--debug-provenance", "--max-nodes", "5")
		link, _, err := runGitCompare(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(link, test.prefix) {
			t.Fatalf("link does not use %s", test.prefix)
		}
		token := strings.TrimPrefix(strings.TrimSpace(link), test.prefix)
		compressed, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatal(err)
		}
		r, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		got := &pb.ImpactScene{}
		if err := protojson.Unmarshal(decoded, got); err != nil {
			t.Fatal(err)
		}
		if got.Provenance == nil || !got.GetComparison().GetPlain() || got.GetComparison().GetNodeBudget() != 5 || len(got.Files) != len(normal.Files) {
			t.Fatalf("selected fields lost: %s", decoded)
		}
	}
}

func TestCompareOversizedURLHasNoOutputAndSkippedReport(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	random := rand.New(rand.NewPCG(1, 2))
	data := make([]byte, 30_000)
	for i := range data {
		data[i] = byte(random.IntN(256))
	}
	payload := `{"comparison":{"key":"` + base64.RawURLEncoding.EncodeToString(data) + `"}}`
	reportPath := filepath.Join(t.TempDir(), "report.json")
	report := CompareReport{Status: "ready", Scene: &pb.ImpactScene{Comparison: &pb.ImpactComparison{}}}
	err := writeCompareOutput(cmd, compareOptions{url: true, urlBase: "http://127.0.0.1:8060", reportJSON: reportPath}, scopedDiagram{text: payload}, report)
	if err == nil || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("error=%v", err)
	}
	if out.Len() != 0 {
		t.Fatal("oversized export wrote partial stdout")
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var got CompareReport
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "skipped" || !strings.Contains(got.Reason, "10000") {
		t.Fatalf("report=%s", raw)
	}
}

func TestCompareURLBaseRequiresURL(t *testing.T) {
	cmd := newCompareCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"base", "head", "--url-base", "http://localhost:8060"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--url-base requires --url") {
		t.Fatalf("error=%v", err)
	}
}

func TestCompareURLBaseDefaultsToLocalServeSettings(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	cfg := workspace.DefaultConfig()
	base, err := compareURLBase(cfg, "/data", "")
	if err != nil || base != "http://127.0.0.1:8060" {
		t.Fatalf("default=%s error=%v", base, err)
	}
	cfg.Serve.Host, cfg.Serve.Port = "0.0.0.0", "9191"
	base, err = compareURLBase(cfg, "/data", "")
	if err != nil || base != "http://127.0.0.1:9191" {
		t.Fatalf("configured=%s error=%v", base, err)
	}
	cfg.Serve.PublicURL = "https://selfhost.example/app/"
	base, err = compareURLBase(cfg, "/data", "")
	if err != nil || base != cfg.Serve.PublicURL {
		t.Fatalf("public=%s error=%v", base, err)
	}
	base, err = compareURLBase(cfg, "/data", "https://tldiagram.com/app")
	if err != nil || base != "https://tldiagram.com/app" {
		t.Fatalf("explicit=%s error=%v", base, err)
	}
	if _, err := compareURLBase(cfg, "/data", "file:///tmp/viewer"); err == nil {
		t.Fatal("invalid explicit base accepted")
	}
}

func TestCompareURLBaseUsesRunningServerForSelectedDataDir(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	cfg := workspace.DefaultConfig()
	records := localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: os.Getpid(), DataDir: "/selected", Addr: "[::]:9123"},
		{Kind: localserver.ProcessKindServer, PID: os.Getpid(), DataDir: "/other", Addr: "127.0.0.1:9991"},
		{Kind: localserver.ProcessKindWatch, PID: os.Getpid(), DataDir: "/selected", Addr: "127.0.0.1:9992"},
		{Kind: localserver.ProcessKindServer, PID: 99999999, DataDir: "/selected", Addr: "127.0.0.1:9993"},
	}}
	if err := localserver.SaveProcessRegistry(records); err != nil {
		t.Fatal(err)
	}
	base, err := compareURLBase(cfg, "/selected", "")
	if err != nil || base != "http://127.0.0.1:9123" {
		t.Fatalf("running=%s error=%v", base, err)
	}
	records.Processes = nil
	if err := localserver.SaveProcessRegistry(records); err != nil {
		t.Fatal(err)
	}
	base, err = compareURLBase(cfg, "/selected", "")
	if err != nil || base != "http://127.0.0.1:8060" {
		t.Fatalf("fallback=%s error=%v", base, err)
	}
}

func TestCompareURLBaseUsesServeEnvironment(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	t.Setenv("TLD_ADDR", "")
	t.Setenv("PORT", "9292")
	t.Setenv("TLD_HOST", "0.0.0.0")
	t.Setenv("TLD_PUBLIC_URL", "")
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	base, err := compareURLBase(cfg, "/data", "")
	if err != nil || base != "http://127.0.0.1:9292" {
		t.Fatalf("environment=%s error=%v", base, err)
	}
}
