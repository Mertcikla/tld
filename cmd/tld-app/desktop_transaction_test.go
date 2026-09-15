package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMacUpdateTransaction(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS ditto")
	}
	for _, missingSource := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing-source-%v", missingSource), func(t *testing.T) {
			dir := t.TempDir()
			script, _, err := writeMacUpdateScripts(dir)
			if err != nil {
				t.Fatal(err)
			}
			dst := filepath.Join(dir, "installed app.app")
			src := filepath.Join(dir, "new app.app")
			if err := os.Mkdir(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dst, "version"), []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			if !missingSource {
				if err := os.Mkdir(src, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(src, "version"), []byte("new"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out, err := exec.Command("/bin/sh", script, "--apply", src, dst).CombinedOutput()
			if (err != nil) != missingSource {
				t.Fatalf("transaction error = %v: %s", err, out)
			}
			want := "new"
			if missingSource {
				want = "old"
			}
			data, err := os.ReadFile(filepath.Join(dst, "version"))
			if err != nil || string(data) != want {
				t.Fatalf("installed version = %q, %v; want %s", data, err, want)
			}
		})
	}
}
