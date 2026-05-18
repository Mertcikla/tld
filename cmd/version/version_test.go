package version_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd/version"
)

func TestVersionCmdPrintsCurrentVersion(t *testing.T) {
	cmd := version.NewVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "tld version "+version.Version) {
		t.Fatalf("stdout = %q, want current version", got)
	}
}

func TestVersionUpdateCmdExists(t *testing.T) {
	root := version.NewVersionCmd()
	cmd, args, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find('update') error = %v", err)
	}
	if cmd.Name() != "update" {
		t.Fatalf("cmd.Name() = %q, want 'update'", cmd.Name())
	}
	if len(args) != 0 {
		t.Fatalf("len(args) = %d, want 0", len(args))
	}
}
