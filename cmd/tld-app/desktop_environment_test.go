package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, tt := range []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "login shell startup messages",
			script: `
if [ "$1" != "-ilc" ]; then exit 2; fi
printf 'shell startup message\n'
PATH='/user/go/bin:/opt/homebrew/bin:/usr/bin:/bin'
eval "$2"
printf 'shell shutdown message\n'
`,
			want: "/user/go/bin:/opt/homebrew/bin:/usr/bin:/bin",
		},
		{name: "shell failure", script: "exit 1"},
		{name: "missing delimiters", script: "printf 'startup message'"},
		{name: "empty path", script: `printf '\000\000'`},
		{name: "incomplete path", script: `printf '\000/usr/bin'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			shell := filepath.Join(t.TempDir(), "shell")
			if err := os.WriteFile(shell, []byte("#!/bin/sh\n"+tt.script), 0o700); err != nil {
				t.Fatal(err)
			}
			got, err := desktopShellPath(context.Background(), shell)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("desktopShellPath = %q, %v; want %q", got, err, tt.want)
			}
			// The parsed PATH must be usable directly, without shell banners.
			if strings.ContainsAny(got, "\x00\n") {
				t.Fatalf("PATH contains shell output: %q", got)
			}
		})
	}
}
