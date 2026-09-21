package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CLIRuntimeStatus describes whether a standalone `tld` CLI is reachable from
// the desktop app, plus the install method the platform supports.
type CLIRuntimeStatus struct {
	// Available reports whether a `tld` executable was found on PATH.
	Available bool `json:"available"`
	// Path is the resolved CLI path when available.
	Path string `json:"path,omitempty"`
	// Platform is the current operating system (darwin, windows, linux).
	Platform string `json:"platform"`
	// InstallSupported reports whether an automated install action exists.
	InstallSupported bool `json:"installSupported"`
	// InstallHint describes what the install action will do.
	InstallHint string `json:"installHint,omitempty"`
}

const (
	cliInstallScriptURL = "https://tldiagram.com/install.sh"
	cliInstallPwshURL   = "https://tldiagram.com/install.ps1"
)

// CLIRuntimeStatus reports whether the standalone CLI is installed. The desktop
// app cannot run `tld watch` itself, so the settings UI uses this to decide
// between a start button and an install/copy-command flow.
func (b *DesktopBridge) CLIRuntimeStatus() CLIRuntimeStatus {
	status := CLIRuntimeStatus{
		Platform:         runtime.GOOS,
		InstallSupported: runtime.GOOS == "darwin" || runtime.GOOS == "windows",
	}
	if path, ok := resolveCLIPath(os.Getenv("PATH")); ok {
		status.Available = true
		status.Path = path
	}
	switch runtime.GOOS {
	case "darwin":
		status.InstallHint = "Installs the tld CLI via the official installer."
	case "windows":
		status.InstallHint = "Installs the tld CLI into %LOCALAPPDATA%\\tld\\bin and updates PATH."
	default:
		status.InstallHint = "Install the tld CLI with the official installer, then reopen the app."
	}
	return status
}

// resolveCLIPath looks up the tld binary on the given PATH value and, failing
// that, in the directories the official installer targets. This lets the app
// detect a freshly installed CLI without restarting for a refreshed PATH.
func resolveCLIPath(pathEnv string) (string, bool) {
	name := cliBinaryName()
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	if pathEnv != "" {
		if path, err := lookPathIn(name, filepath.SplitList(pathEnv)); err == nil {
			return path, true
		}
	}
	if path, err := lookPathIn(name, cliSearchDirs()); err == nil {
		return path, true
	}
	return "", false
}

// lookPathIn resolves name within the provided directories, returning the first
// executable match. It mirrors exec.LookPath semantics for the installer dirs.
func lookPathIn(name string, dirs []string) (string, error) {
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", exec.ErrNotFound
}

// WatchCommand returns the exact `tld watch` command to run for a repository.
func (b *DesktopBridge) WatchCommand(repoPath string, extraArgs []string) string {
	cleanPath := strings.TrimSpace(repoPath)
	if cleanPath == "" {
		cleanPath = "."
	}
	parts := []string{"tld", "watch", shellQuote(cleanPath)}
	if len(extraArgs) > 0 {
		parts = append(parts, extraArgs...)
	}
	return strings.Join(parts, " ")
}

// InstallCLI downloads and runs the official tld CLI installer in a terminal
// so the user can complete any prompts. It returns the status after launching.
func (b *DesktopBridge) InstallCLI() (CLIRuntimeStatus, error) {
	status := b.CLIRuntimeStatus()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return status, fmt.Errorf("automated CLI install is not available for %s", runtime.GOOS)
	}
	if err := b.startCLIInstall(); err != nil {
		return status, err
	}
	return status, nil
}

func (b *DesktopBridge) startCLIInstall() error {
	switch runtime.GOOS {
	case "darwin":
		return startUnixCLIInstall()
	case "windows":
		return startWindowsCLIInstall()
	default:
		return fmt.Errorf("automated CLI install is not available for %s", runtime.GOOS)
	}
}

// startUnixCLIInstall opens Terminal.app and runs the official install script.
// Running in Terminal (rather than silently) lets the user see progress and
// answer any sudo/PATH prompts.
func startUnixCLIInstall() error {
	if _, err := exec.LookPath("osascript"); err != nil {
		return errors.New("osascript is unavailable; install the CLI manually")
	}
	command := fmt.Sprintf("curl -LsSf %s | sh", cliInstallScriptURL)
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script %s
end tell`, appleScriptString(command))
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("open installer terminal: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// startWindowsCLIInstall launches the PowerShell installer in a new window so
// the user can complete PATH prompts and credential elevation.
func startWindowsCLIInstall() error {
	command := fmt.Sprintf("irm %s | iex; Read-Host 'Press Enter to close'", cliInstallPwshURL)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start CLI installer: %w", err)
	}
	return nil
}

func appleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// shellQuote wraps a path in single quotes when it contains characters the
// shell would otherwise interpret.
func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\"'\\$&;|<>()*?[]#~`") {
		return value
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// cliBinaryName is exposed for tests to reason about platform expectations.
func cliBinaryName() string {
	if runtime.GOOS == "windows" {
		return "tld.exe"
	}
	return "tld"
}

// cliSearchDirs documents the locations the installer may place the CLI, used
// by tests and diagnostics.
func cliSearchDirs() []string {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"))
	}
	dirs = append(dirs, "/usr/local/bin")
	return dirs
}
