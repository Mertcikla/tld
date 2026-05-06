package stop_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd/stop"
)

func TestStopCmdReportsNoServerRunningForEmptyDataDir(t *testing.T) {
	cmd := stop.NewStopCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--data-dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("err = %v, want no server running", err)
	}
}
