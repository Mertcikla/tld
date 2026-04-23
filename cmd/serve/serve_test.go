package serve_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertcikla/diag/tld/cmd"
	"github.com/spf13/cobra"
)

func TestServeCommandInvokesInjectedHandler(t *testing.T) {
	called := false
	root := cmd.NewRootCmd(cmd.WithServeCommand(func(_ *cobra.Command, _ []string) error {
		called = true
		return nil
	}))

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute serve: %v", err)
	}
	if !called {
		t.Fatal("serve handler was not invoked")
	}
}

func TestRootCommandWithoutArgsShowsHelp(t *testing.T) {
	root := cmd.NewRootCmd(cmd.WithServeCommand(func(_ *cobra.Command, _ []string) error {
		t.Fatal("serve handler should not run for bare root command")
		return nil
	}))

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(nil)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root: %v", err)
	}

	output := outBuf.String() + errBuf.String()
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected help output, got %q", output)
	}
	if !strings.Contains(output, "serve") {
		t.Fatalf("expected serve command in help output, got %q", output)
	}
}
