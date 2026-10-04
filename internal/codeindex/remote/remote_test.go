package remote

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		provider Provider
		clone    string
		web      string
		wantErr  bool
	}{
		{name: "github shorthand", in: "facebook/react", provider: ProviderGitHub, clone: "https://github.com/facebook/react.git", web: "https://github.com/facebook/react"},
		{name: "github https", in: "https://github.com/facebook/react.git", provider: ProviderGitHub, clone: "https://github.com/facebook/react.git", web: "https://github.com/facebook/react"},
		{name: "github scp", in: "git@github.com:facebook/react.git", provider: ProviderGitHub, clone: "https://github.com/facebook/react.git", web: "https://github.com/facebook/react"},
		{name: "gitlab https", in: "https://gitlab.com/group/sub/repo", provider: ProviderGit, clone: "https://gitlab.com/group/sub/repo.git", web: "https://gitlab.com/group/sub/repo"},
		{name: "gitlab scp", in: "git@gitlab.com:group/repo.git", provider: ProviderGit, clone: "git@gitlab.com:group/repo.git", web: "https://gitlab.com/group/repo"},
		{name: "credentials rejected", in: "https://token@github.com/facebook/react", wantErr: true},
		{name: "password rejected", in: "https://user:pass@github.com/facebook/react", wantErr: true},
		{name: "local path rejected", in: "/Users/me/repo", wantErr: true},
		{name: "home path rejected", in: "~/repo", wantErr: true},
		{name: "windows path rejected", in: `C:\code\repo`, wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
		{name: "file scheme rejected", in: "file:///tmp/repo", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := Parse(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.in, err)
			}
			if spec.Provider != tc.provider || spec.CloneURL != tc.clone || spec.WebURL != tc.web {
				t.Fatalf("Parse(%q) = %+v, want provider=%s clone=%s web=%s", tc.in, spec, tc.provider, tc.clone, tc.web)
			}
		})
	}
}

func TestManagedDirIsDeterministicAndContained(t *testing.T) {
	spec, err := Parse("facebook/react")
	if err != nil {
		t.Fatal(err)
	}
	first := ManagedDir("/data", spec)
	second := ManagedDir("/data", spec)
	if first != second {
		t.Fatalf("ManagedDir not deterministic: %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, filepath.Join("/data", "repositories")+string(filepath.Separator)) {
		t.Fatalf("ManagedDir %q not under repositories root", first)
	}
	if !IsManagedPath("/data", first) {
		t.Fatalf("IsManagedPath(%q) = false", first)
	}
	if IsManagedPath("/data", "/data/elsewhere") {
		t.Fatal("IsManagedPath accepted an unmanaged path")
	}
	if IsManagedPath("/data", ManagedRoot("/data")) {
		t.Fatal("IsManagedPath accepted the managed root itself")
	}
}

func TestCloneLocalRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	source := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = source
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")

	dest := filepath.Join(t.TempDir(), "clone")
	if err := Clone(context.Background(), Spec{CloneURL: source, WebURL: "https://example.com/repo"}, dest); err != nil {
		t.Fatalf("Clone() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "main.go")); err != nil {
		t.Fatalf("clone missing file: %v", err)
	}
	// A second clone call reuses the existing checkout.
	if err := Clone(context.Background(), Spec{CloneURL: source, WebURL: "https://example.com/repo"}, dest); err != nil {
		t.Fatalf("Clone() reuse error: %v", err)
	}
}
