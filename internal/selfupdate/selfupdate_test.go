package selfupdate

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsNewerNormalizesTags(t *testing.T) {
	if !IsNewer("2.0.2", "v2.0.3") {
		t.Fatal("expected v2.0.3 to be newer than 2.0.2")
	}
	if IsNewer("2.0.3", "v2.0.3") {
		t.Fatal("same version should not be newer")
	}
	if IsNewer("2.0.3", "not-a-version") {
		t.Fatal("invalid latest version should not be newer")
	}
}

func TestCheckUsesFreshCachedState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "update-check.json")
	if err := writeState(statePath, stateFile{
		CheckedAt:  time.Now().UTC(),
		Latest:     "v2.0.3",
		ReleaseURL: "https://github.com/Mertcikla/tld/releases/tag/v2.0.3",
		AssetName:  assetName(),
		AssetURL:   "https://example.test/tld.tar.gz",
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	status, err := Check(context.Background(), Options{
		Current:       "2.0.2",
		CheckInterval: time.Hour,
		StatePath:     statePath,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.Cached || !status.UpdateAvailable || status.Latest != "v2.0.3" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestCheckFetchesLatestReleaseWhenCacheIsStale(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Mertcikla/tld/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.4","prerelease":false,"html_url":"https://github.com/Mertcikla/tld/releases/tag/v2.0.4","assets":[{"name":"` + assetName() + `","browser_download_url":"https://example.test/asset"}]}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	status, err := Check(context.Background(), Options{
		Current:       "2.0.2",
		CheckInterval: time.Hour,
		StatePath:     filepath.Join(t.TempDir(), "update-check.json"),
		HTTPClient:    server.Client(),
		APIBaseURL:    server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.UpdateAvailable || status.Latest != "v2.0.4" || status.AssetURL == "" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestCheckUsesExplicitAssetName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Mertcikla/tld/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.4","prerelease":false,"html_url":"https://github.com/Mertcikla/tld/releases/tag/v2.0.4","assets":[{"name":"tld-desktop-macos-arm64.zip","browser_download_url":"https://example.test/desktop"}]}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	status, err := Check(context.Background(), Options{
		Current:       "2.0.2",
		AssetName:     "tld-desktop-macos-arm64.zip",
		CheckInterval: time.Hour,
		StatePath:     filepath.Join(t.TempDir(), "desktop-update-check.json"),
		HTTPClient:    server.Client(),
		APIBaseURL:    server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.UpdateAvailable || status.AssetName != "tld-desktop-macos-arm64.zip" || status.AssetURL != "https://example.test/desktop" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestInstallDownloadUsesContextDeadline(t *testing.T) {
	name := assetName()
	if name == "" {
		t.Skip("unsupported test platform")
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/repos/Mertcikla/tld/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2.0.4","prerelease":false,"html_url":"https://github.com/Mertcikla/tld/releases/tag/v2.0.4","assets":[{"name":"` + name + `","browser_download_url":"` + server.URL + `/asset"}]}]`))
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := Install(ctx, Options{
		Current:       "2.0.2",
		CheckInterval: time.Hour,
		StatePath:     filepath.Join(t.TempDir(), "update-check.json"),
		HTTPClient:    server.Client(),
		APIBaseURL:    server.URL,
		Force:         true,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Install() error = %v, want context deadline exceeded", err)
	}
}

func TestReplacementPreservesSymlinkAndPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "actual")
	link := filepath.Join(dir, "tld")
	replacement := filepath.Join(t.TempDir(), "new")
	for path, body := range map[string]string{target: "old", replacement: "new"} {
		if err := os.WriteFile(path, []byte(body), 0o751); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := replaceExecutable(link, replacement); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("launcher symlink lost: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatalf("target = %q, %v", data, err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o751 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestReplacementFailureLeavesInstalledBinary(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "tld")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(exe, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error")
	}
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "old" {
		t.Fatalf("installed binary lost: %q, %v", data, err)
	}
}

func TestCheckSelectsNewestCompleteStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `[
		{"tag_name":"v9.0.0","draft":true,"assets":[{"name":%q,"browser_download_url":"draft"}]},
		{"tag_name":"v8.0.0-preview.1","assets":[{"name":%q,"browser_download_url":"preview"}]},
		{"tag_name":"v7.0.0","assets":[]},
		{"tag_name":"v2.0.0","assets":[{"name":%q,"browser_download_url":"older"}]},
		{"tag_name":"v3.0.0","assets":[{"name":%q,"browser_download_url":"newest"}]}
		]`, assetName(), assetName(), assetName(), assetName())
	}))
	defer server.Close()
	status, err := Check(context.Background(), Options{Current: "1.0.0", APIBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if status.Latest != "v3.0.0" || status.AssetURL != "newest" {
		t.Fatalf("status = %+v", status)
	}
}

func TestCheckIgnoresCacheForDifferentAsset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeState(path, stateFile{CheckedAt: time.Now(), Latest: "v9.0.0", AssetName: "wrong", AssetURL: "wrong"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"tag_name":"v2.0.0","assets":[{"name":%q,"browser_download_url":"correct"}]}]`, assetName())
	}))
	defer server.Close()
	status, err := Check(context.Background(), Options{Current: "1.0.0", StatePath: path, APIBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if status.Cached || status.AssetURL != "correct" {
		t.Fatalf("status = %+v", status)
	}
}

// Run an actual image while replacing it; Windows rejects direct overwrite.
func TestReplaceRunningExecutable(t *testing.T) {
	if os.Getenv("TLD_UPDATE_TEST_CHILD") == "1" {
		fmt.Println("ready")
		time.Sleep(time.Minute)
		return
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "running.exe")
	if err := os.WriteFile(exe, data, 0o755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestReplaceRunningExecutable$")
	child.Env = append(os.Environ(), "TLD_UPDATE_TEST_CHILD=1")
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("child not ready: %q, %v", line, err)
	}
	if err := replaceExecutable(exe, source); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(exe); err != nil || info.Size() != int64(len(data)) {
		t.Fatalf("replacement missing: %v", err)
	}
}

func TestExtractRejectsCorruptGzipTrailer(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "tld", Mode: 0o755, Size: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	data := archive.Bytes()
	data[len(data)-8] ^= 0xff
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBinary(path, t.TempDir()); err == nil {
		t.Fatal("accepted corrupt gzip checksum")
	}
}
