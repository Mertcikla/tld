package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Finder launches apps with a minimal PATH. Load the user's shell PATH so the
// repository watcher can find the CLI and its external indexing tools.
func configureDesktopEnvironment() error {
	if runtime.GOOS != "darwin" || useMacContainerRuntimePaths() {
		return nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path, err := desktopShellPath(ctx, shell)
	if err != nil {
		return err
	}
	return os.Setenv("PATH", path)
}

func desktopShellPath(ctx context.Context, shell string) (string, error) {
	// Interactive login shells also read tool-manager setup in .zshrc/.bashrc.
	// NUL delimiters separate PATH from any shell startup messages.
	cmd := exec.CommandContext(ctx, shell, "-ilc", `printf '\000%s\000' "$PATH"`)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read shell PATH: %w", err)
	}
	_, rest, ok := strings.Cut(string(out), "\x00")
	if !ok {
		return "", fmt.Errorf("shell did not return a delimited PATH")
	}
	path, _, ok := strings.Cut(rest, "\x00")
	if !ok || path == "" {
		return "", fmt.Errorf("shell returned an empty or incomplete PATH")
	}
	return path, nil
}
