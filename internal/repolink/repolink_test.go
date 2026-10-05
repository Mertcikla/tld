package repolink

import "testing"

func TestNormalizeRemote(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"https", "https://github.com/owner/repo", "https://github.com/owner/repo", true},
		{"https dot git", "https://github.com/owner/repo.git", "https://github.com/owner/repo", true},
		{"https trailing slash", "https://github.com/owner/repo/", "https://github.com/owner/repo", true},
		{"https with credentials", "https://user:token@github.com/owner/repo.git", "https://github.com/owner/repo", true},
		{"http", "http://gitlab.com/group/repo", "https://gitlab.com/group/repo", true},
		{"ssh scp", "git@github.com:owner/repo.git", "https://github.com/owner/repo", true},
		{"ssh url", "ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo", true},
		{"git url", "git://github.com/owner/repo", "https://github.com/owner/repo", true},
		{"host slash path", "github.com/owner/repo", "https://github.com/owner/repo", true},
		{"gitlab subgroups", "https://gitlab.com/group/sub/repo.git", "https://gitlab.com/group/sub/repo", true},
		{"empty", "", "", false},
		{"absolute path", "/Users/me/repo", "", false},
		{"home path", "~/repo", "", false},
		{"windows path", `C:\Users\me\repo`, "", false},
		{"plain branch", "main", "", false},
		{"owner repo only", "owner/repo", "", false},
		{"file scheme", "file:///tmp/repo", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeRemote(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeRemote(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRemoteKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/Owner/Repo.git", "github.com/owner/repo"},
		{"git@github.com:Owner/Repo.git", "github.com/owner/repo"},
		{"Owner/Repo", "github.com/owner/repo"},
		{"https://gitlab.com/group/sub/repo", "gitlab.com/group/sub/repo"},
		{"/Users/me/repo", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := RemoteKey(tc.in); got != tc.want {
			t.Fatalf("RemoteKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestByRemote(t *testing.T) {
	repos := []Repository{
		{ID: "one", Root: "/work/alpha", RemoteURL: "https://github.com/owner/alpha"},
		{ID: "two", Root: "/work/beta"},
	}

	if got, ok := ByRemote("/work/alpha", repos); !ok || got.ID != "one" {
		t.Fatalf("root match failed: %+v %v", got, ok)
	}
	if got, ok := ByRemote("owner/alpha", repos); !ok || got.ID != "one" {
		t.Fatalf("slug match failed: %+v %v", got, ok)
	}
	if got, ok := ByRemote("git@github.com:Owner/Alpha.git", repos); !ok || got.ID != "one" {
		t.Fatalf("scp match failed: %+v %v", got, ok)
	}
	if _, ok := ByRemote("/work/other", repos); ok {
		t.Fatal("unexpected match for unrelated path")
	}
	if _, ok := ByRemote("owner/missing", repos); ok {
		t.Fatal("unexpected match for unrelated slug")
	}
}

func TestResolvePrefersExplicitID(t *testing.T) {
	repos := []Repository{
		{ID: "one", Root: "/work/alpha", RemoteURL: "https://github.com/owner/alpha"},
		{ID: "two", Root: "/work/beta", RemoteURL: "https://github.com/owner/alpha"},
	}
	got, ok := Resolve("two", "owner/alpha", "", repos)
	if !ok || got.ID != "two" {
		t.Fatalf("Resolve() = %+v %v, want id two", got, ok)
	}
}

func TestResolveFallsBackToFilePath(t *testing.T) {
	repos := []Repository{{ID: "one", Root: "/work/alpha"}}
	got, ok := Resolve("", "", "/work/alpha/pkg/main.go#L12", repos)
	if !ok || got.ID != "one" {
		t.Fatalf("Resolve() = %+v %v, want id one", got, ok)
	}
	if _, ok := Resolve("", "", "/work/other/pkg/main.go", repos); ok {
		t.Fatal("unexpected resolution outside repository root")
	}
}
