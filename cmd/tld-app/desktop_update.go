package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	cmdversion "github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/selfupdate"
	"github.com/mertcikla/tld/v2/internal/workspace"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const desktopUpdateStateFilename = "desktop-update-check.json"

type DesktopUpdateStatus struct {
	Checked         bool   `json:"checked"`
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
	AssetName       string `json:"assetName"`
	Cached          bool   `json:"cached"`
	Supported       bool   `json:"supported"`
	CanInstall      bool   `json:"canInstall"`
	InstallStarted  bool   `json:"installStarted"`
	RestartRequired bool   `json:"restartRequired"`
	Message         string `json:"message,omitempty"`
}

func (b *DesktopBridge) CheckForUpdate() (DesktopUpdateStatus, error) {
	if b.ctx == nil {
		return DesktopUpdateStatus{}, errors.New("desktop bridge is not ready")
	}
	if appStoreBuild {
		return DesktopUpdateStatus{
			Current:   cmdversion.Version,
			Supported: false,
			Message:   "Updates are managed by the Mac App Store.",
		}, nil
	}
	ctx, cancel := context.WithTimeout(b.ctx, selfupdate.DefaultCheckTimeout)
	defer cancel()
	status, _, err := checkDesktopUpdate(ctx, true)
	return status, err
}

func (b *DesktopBridge) InstallUpdate() (DesktopUpdateStatus, error) {
	if b.ctx == nil {
		return DesktopUpdateStatus{}, errors.New("desktop bridge is not ready")
	}
	if appStoreBuild {
		return DesktopUpdateStatus{
			Current:   cmdversion.Version,
			Supported: false,
			Message:   "Updates are managed by the Mac App Store.",
		}, errors.New("self update is disabled for Mac App Store builds")
	}
	ctx, cancel := context.WithTimeout(b.ctx, selfupdate.DefaultInstallTimeout)
	defer cancel()

	status, raw, err := checkDesktopUpdate(ctx, true)
	if err != nil {
		return status, err
	}
	if !status.Supported {
		return status, errors.New(status.Message)
	}
	if !status.UpdateAvailable {
		status.Message = "tlDiagram is up to date."
		return status, nil
	}
	if raw.AssetURL == "" {
		return status, fmt.Errorf("release %s does not include desktop asset %s", status.Latest, status.AssetName)
	}

	assetPath, err := downloadDesktopUpdate(ctx, raw)
	if err != nil {
		return status, err
	}
	if err := startDesktopUpdate(ctx, assetPath); err != nil {
		return status, err
	}

	status.InstallStarted = true
	status.RestartRequired = true
	status.Message = "Update installer started. tlDiagram will close to finish updating."
	scheduleDesktopQuit(b.ctx)
	return status, nil
}

func checkDesktopUpdate(ctx context.Context, force bool) (DesktopUpdateStatus, selfupdate.Status, error) {
	assetName, supported := desktopAssetName(runtime.GOOS, runtime.GOARCH)
	raw := selfupdate.Status{Current: cmdversion.Version}
	if !supported {
		status := DesktopUpdateStatus{
			Current:   cmdversion.Version,
			Supported: false,
			Message:   fmt.Sprintf("Desktop updates are not available for %s/%s.", runtime.GOOS, runtime.GOARCH),
		}
		return status, raw, nil
	}

	raw, err := selfupdate.Check(ctx, desktopUpdateOptions(assetName, force))
	if err != nil {
		return toDesktopUpdateStatus(raw, true), raw, err
	}
	status := toDesktopUpdateStatus(raw, true)
	if status.UpdateAvailable && !status.CanInstall {
		status.Message = fmt.Sprintf("Release %s does not include desktop asset %s.", status.Latest, assetName)
	}
	return status, raw, nil
}

func desktopUpdateOptions(assetName string, force bool) selfupdate.Options {
	interval := selfupdate.DefaultCheckInterval
	if cfg, err := workspace.LoadGlobalConfig(); err == nil {
		if parsed, err := time.ParseDuration(cfg.Updates.CheckInterval); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	return selfupdate.Options{
		Current:       cmdversion.Version,
		AssetName:     assetName,
		CheckInterval: interval,
		StatePath:     desktopUpdateStatePath(),
		Force:         force,
	}
}

func desktopUpdateStatePath() string {
	dir, err := workspace.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, desktopUpdateStateFilename)
}

func toDesktopUpdateStatus(status selfupdate.Status, supported bool) DesktopUpdateStatus {
	return DesktopUpdateStatus{
		Checked:         status.Checked,
		Current:         status.Current,
		Latest:          status.Latest,
		UpdateAvailable: status.UpdateAvailable,
		ReleaseURL:      status.ReleaseURL,
		AssetName:       status.AssetName,
		Cached:          status.Cached,
		Supported:       supported,
		CanInstall:      status.UpdateAvailable && status.AssetURL != "",
	}
}

func desktopAssetName(goos, goarch string) (string, bool) {
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return "tld-desktop-macos-arm64.zip", true
		case "amd64":
			return "tld-desktop-macos-x64.zip", true
		}
	case "windows":
		if goarch == "amd64" {
			return "tld-desktop-windows-x64-installer.exe", true
		}
	}
	return "", false
}

func downloadDesktopUpdate(ctx context.Context, status selfupdate.Status) (string, error) {
	if status.AssetURL == "" || status.AssetName == "" {
		return "", errors.New("desktop update asset is unavailable")
	}

	dir, err := desktopUpdateDownloadDir()
	if err != nil {
		return "", err
	}
	assetPath := filepath.Join(dir, status.AssetName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, status.AssetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "tld-desktop-update")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", status.AssetName, resp.StatusCode)
	}

	file, err := os.OpenFile(assetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return assetPath, nil
}

func desktopUpdateDownloadDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	root := filepath.Join(base, "tldiagram", "updates")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, "desktop-*")
}

func startDesktopUpdate(ctx context.Context, assetPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return stageMacDesktopUpdate(ctx, assetPath)
	case "windows":
		return startWindowsDesktopInstaller(assetPath)
	default:
		return fmt.Errorf("desktop updates are not available for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func stageMacDesktopUpdate(ctx context.Context, archivePath string) error {
	bundlePath, err := currentAppBundle()
	if err != nil {
		return err
	}

	stageDir := filepath.Join(filepath.Dir(archivePath), "extracted")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ditto", "-x", "-k", archivePath, stageDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract desktop update: %w: %s", err, strings.TrimSpace(string(out)))
	}

	newBundlePath, err := findExtractedAppBundle(stageDir)
	if err != nil {
		return err
	}
	scriptPath, osascriptPath, err := writeMacUpdateScripts(filepath.Dir(archivePath))
	if err != nil {
		return err
	}

	helper := exec.Command("/bin/sh", scriptPath, strconv.Itoa(os.Getpid()), newBundlePath, bundlePath, osascriptPath)
	if err := helper.Start(); err != nil {
		return fmt.Errorf("start desktop update helper: %w", err)
	}
	return helper.Process.Release()
}

func currentAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return currentAppBundleFromExecutable(exe)
}

func currentAppBundleFromExecutable(exe string) (string, error) {
	dir := filepath.Clean(filepath.Dir(exe))
	for {
		if strings.EqualFold(filepath.Ext(dir), ".app") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("current executable is not inside a macOS app bundle")
}

func findExtractedAppBundle(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	fallback := ""
	for _, entry := range entries {
		if !entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".app") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if strings.EqualFold(entry.Name(), "tld.app") {
			return path, nil
		}
		if fallback == "" {
			fallback = path
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", errors.New("update archive did not contain a macOS app bundle")
}

func writeMacUpdateScripts(dir string) (string, string, error) {
	scriptPath := filepath.Join(dir, "install-tld-update.sh")
	osascriptPath := filepath.Join(dir, "install-tld-update.applescript")
	shellScript := `#!/bin/sh
pid="$1"
src="$2"
dst="$3"
osa="$4"
backup="${dst}.old-update"

while /bin/kill -0 "$pid" 2>/dev/null; do
  /bin/sleep 0.2
done

/bin/rm -rf "$backup"
if /bin/mv "$dst" "$backup" 2>/dev/null && /bin/mv "$src" "$dst" 2>/dev/null; then
  /bin/rm -rf "$backup"
  /usr/bin/open "$dst"
  exit 0
fi

if [ -e "$backup" ] && [ ! -e "$dst" ]; then
  /bin/mv "$backup" "$dst" 2>/dev/null || true
fi

if /usr/bin/osascript "$osa" "$src" "$dst"; then
  /usr/bin/open "$dst"
  exit 0
fi

if [ -e "$dst" ]; then
  /usr/bin/open "$dst"
fi
exit 1
`
	appleScript := `on run argv
  set srcPath to item 1 of argv
  set dstPath to item 2 of argv
  do shell script "/bin/rm -rf " & quoted form of dstPath & " && /bin/mv " & quoted form of srcPath & " " & quoted form of dstPath with administrator privileges
end run
`
	if err := os.WriteFile(scriptPath, []byte(shellScript), 0o700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(osascriptPath, []byte(appleScript), 0o600); err != nil {
		return "", "", err
	}
	return scriptPath, osascriptPath, nil
}

func startWindowsDesktopInstaller(assetPath string) error {
	if !strings.EqualFold(filepath.Ext(assetPath), ".exe") {
		return fmt.Errorf("windows desktop update asset must be an exe: %s", filepath.Base(assetPath))
	}
	scriptPath := filepath.Join(filepath.Dir(assetPath), "install-tld-update.cmd")
	script := `@echo off
set PID=%~1
set INSTALLER=%~2

:wait
tasklist /FI "PID eq %PID%" 2>NUL | find "%PID%" >NUL
if not errorlevel 1 (
  timeout /T 1 /NOBREAK >NUL
  goto wait
)

start "" "%INSTALLER%"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", scriptPath, strconv.Itoa(os.Getpid()), assetPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start desktop update installer: %w", err)
	}
	return cmd.Process.Release()
}

func scheduleDesktopQuit(ctx context.Context) {
	go func() {
		time.Sleep(750 * time.Millisecond)
		wailsruntime.Quit(ctx)
	}()
}
