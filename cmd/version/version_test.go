package version_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd/version"
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
