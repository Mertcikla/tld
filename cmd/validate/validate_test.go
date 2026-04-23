package validate_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCmd_ValidWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "add", "System", "--ref", "sys", "--kind", "workspace"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "validate")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout, "Workspace valid") {
		t.Errorf("stdout %q does not contain 'Workspace valid'", stdout)
	}
	if !strings.Contains(stdout, "Element workspace: 1 elements, 1 diagrams, 0 connectors") {
		t.Errorf("stdout %q does not contain count summary", stdout)
	}
}

func TestValidateCmd_InvalidWorkspace(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte("bad:\n  kind: service\n"), 0600); err != nil {
		t.Fatalf("write elements: %v", err)
	}

	_, stderr, err := cmd.RunCmd(t, dir, "validate")
	if err == nil {
		t.Fatal("expected error for invalid workspace")
	}
	if !strings.Contains(stderr, "Validation errors") {
		t.Errorf("stderr %q does not contain 'Validation errors'", stderr)
	}
}

func TestValidateCmd_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	// No .tld.yaml
	_, _, err := cmd.RunCmd(t, dir, "validate")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "load workspace") {
		t.Errorf("error %q does not contain 'load workspace'", err.Error())
	}
}
