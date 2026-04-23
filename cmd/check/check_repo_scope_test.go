package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"
)

func TestCheckCmd_SkipsForeignRepoSymbols(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	content := `good:
  name: Good Service
  kind: service
  file_path: cmd/init.go
  symbol: newInitCmd
  placements: [ { parent: root } ]
foreign:
  name: Foreign Service
  kind: service
  file_path: /tmp/foreign/foreign.go
  symbol: doesNotExist
  repo: https://example.com/other.git
  placements: [ { parent: root } ]
`
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("write elements.yaml: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "check")
	if err != nil {
		t.Fatalf("check: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS  Symbol Verification") {
		t.Errorf("stdout %q does not contain symbol verification pass", stdout)
	}
	if strings.Contains(stderr, "Foreign Service") || strings.Contains(stderr, "doesNotExist") {
		t.Errorf("stderr %q should not mention the foreign repo symbol", stderr)
	}
}

func TestValidateCmd_SkipsForeignRepoSymbols(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	content := `good:
  name: Good Service
  kind: service
  file_path: cmd/init.go
  symbol: newInitCmd
  placements: [ { parent: root } ]
foreign:
  name: Foreign Service
  kind: service
  file_path: /tmp/foreign/foreign.go
  symbol: doesNotExist
  repo: https://example.com/other.git
  placements: [ { parent: root } ]
`
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("write elements.yaml: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Symbol verification: passed") {
		t.Errorf("stdout %q does not contain symbol verification pass", stdout)
	}
	if strings.Contains(stderr, "Foreign Service") || strings.Contains(stderr, "doesNotExist") {
		t.Errorf("stderr %q should not mention the foreign repo symbol", stderr)
	}
}
