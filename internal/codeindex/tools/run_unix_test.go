//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunInheritedPipe(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child.log")
	// The wrapper exits while a child keeps the output pipe open and writes a
	// heartbeat. WaitDelay must return and process-group cleanup must stop it.
	_, err := Run(context.Background(), "/bin/sh", []string{"-c", `(while :; do echo alive >> "$1"; sleep 0.05; done) &`, "tool", marker}, "")
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("expected inherited pipe timeout, got %v", err)
	}
	before, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(before), "alive") {
		t.Fatalf("child did not run: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(marker)
	if err != nil || string(before) != string(after) {
		t.Fatalf("child continued after cleanup: %v", err)
	}
}
