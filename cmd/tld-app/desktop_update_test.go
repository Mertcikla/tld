package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopAssetName(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		arch  string
		asset string
		ok    bool
	}{
		{name: "mac arm64", goos: "darwin", arch: "arm64", asset: "tld-desktop-macos-arm64.zip", ok: true},
		{name: "mac amd64", goos: "darwin", arch: "amd64", asset: "tld-desktop-macos-x64.zip", ok: true},
		{name: "windows amd64", goos: "windows", arch: "amd64", asset: "tld-desktop-windows-x64-installer.exe", ok: true},
		{name: "linux unsupported", goos: "linux", arch: "amd64", asset: "", ok: false},
		{name: "windows arm64 unsupported", goos: "windows", arch: "arm64", asset: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset, ok := desktopAssetName(tt.goos, tt.arch)
			if asset != tt.asset || ok != tt.ok {
				t.Fatalf("desktopAssetName(%q, %q) = %q, %v; want %q, %v", tt.goos, tt.arch, asset, ok, tt.asset, tt.ok)
			}
		})
	}
}

func TestDesktopReleaseWorkflowPublishesUpdaterAssets(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "wails-release.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)

	expected := []string{
		"tld-desktop-macos-arm64.zip",
		"tld-desktop-macos-x64.zip",
		"tld-desktop-windows-x64-installer.exe",
	}
	for _, asset := range expected {
		t.Run(asset, func(t *testing.T) {
			if !strings.Contains(workflow, asset) {
				t.Fatalf("%s does not publish %s", workflowPath, asset)
			}
		})
	}

	oldNames := []string{
		"tld-macos-arm64.zip",
		"tld-macos-amd64.zip",
		"tld-windows-amd64-installer.exe",
	}
	for _, asset := range oldNames {
		t.Run("old "+asset, func(t *testing.T) {
			if strings.Contains(workflow, asset) {
				t.Fatalf("%s still references old desktop asset name %s", workflowPath, asset)
			}
		})
	}
}

func TestWailsReleaseWorkflowUsesProductVersionHelper(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "wails-release.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)

	call := `bash scripts/ci/wails-product-version.sh "$RELEASE_TAG"`
	if got := strings.Count(workflow, call); got != 2 {
		t.Fatalf("workflow should call %s once per desktop release job; found %d", call, got)
	}
}

func TestWailsProductVersionScriptNormalizesPrereleaseTags(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}

	script := filepath.Join("..", "..", "scripts", "ci", "wails-product-version.sh")
	tests := []struct {
		tag  string
		want string
	}{
		{tag: "v2.3.0", want: "2.3.0"},
		{tag: "v2.3.0-beta.1", want: "2.3.0"},
		{tag: "v2.3.0+build.7", want: "2.3.0"},
		{tag: "v2.3.0-beta.1+build.7", want: "2.3.0"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			cmd := exec.Command(bash, script, tt.tag)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", script, err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tt.want {
				t.Fatalf("%s returned %q, want %q", script, got, tt.want)
			}
		})
	}
}

func TestWailsProductVersionScriptRejectsInvalidNumericVersion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}

	script := filepath.Join("..", "..", "scripts", "ci", "wails-product-version.sh")
	cmd := exec.Command(bash, script, "v2.3.0.1")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("%s accepted invalid Wails productVersion tag: %s", script, out)
	}
}

func TestWindowsDesktopUpdateWorkflowChecksProcessExitCode(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "wails-release.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)

	expected := "Start-Process -FilePath $installed.FullName -ArgumentList @('--desktop-update-e2e', '--desktop-update-asset', $asset) -Wait -PassThru"
	if !strings.Contains(workflow, expected) {
		t.Fatalf("Windows desktop update e2e must use Start-Process so GUI app exit codes are available")
	}
	if strings.Contains(workflow, "$LASTEXITCODE") {
		t.Fatalf("Windows desktop update e2e must not rely on LASTEXITCODE for the Wails GUI app")
	}
}

func TestCurrentAppBundleFromExecutable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tld.app")
	exe := filepath.Join(root, "Contents", "MacOS", "tld")

	got, err := currentAppBundleFromExecutable(exe)
	if err != nil {
		t.Fatalf("currentAppBundleFromExecutable returned error: %v", err)
	}
	if got != root {
		t.Fatalf("bundle = %q, want %q", got, root)
	}
}

func TestCurrentAppBundleFromExecutableRejectsNonBundle(t *testing.T) {
	if _, err := currentAppBundleFromExecutable(filepath.Join(t.TempDir(), "tld")); err == nil {
		t.Fatal("currentAppBundleFromExecutable returned nil error outside app bundle")
	}
}

func TestFindExtractedAppBundlePrefersTLDApp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Other.app"), 0o755); err != nil {
		t.Fatalf("mkdir Other.app: %v", err)
	}
	tldApp := filepath.Join(dir, "tld.app")
	if err := os.Mkdir(tldApp, 0o755); err != nil {
		t.Fatalf("mkdir tld.app: %v", err)
	}

	got, err := findExtractedAppBundle(dir)
	if err != nil {
		t.Fatalf("findExtractedAppBundle returned error: %v", err)
	}
	if got != tldApp {
		t.Fatalf("bundle = %q, want %q", got, tldApp)
	}
}

func TestFindExtractedAppBundleRequiresApp(t *testing.T) {
	if _, err := findExtractedAppBundle(t.TempDir()); err == nil {
		t.Fatal("findExtractedAppBundle returned nil error without app bundle")
	}
}
