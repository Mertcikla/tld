package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldIgnorePath(t *testing.T) {
	r := &Rules{Exclude: []string{"vendor/", "node_modules/", ".git/", "**/*.pb.go", "**/*_test.go"}}
	cases := []struct {
		path   string
		expect bool
	}{
		{"vendor", true},
		{"vendor/foo", true},
		{"node_modules", true},
		{".venv/", true},
		{"node_modules/lodash", true},
		{".git", true},
		{"src/vendor/lib", false}, // vendor only at root segment
		{"myvendor", false},
		{"src", false},
		{"foo_test.go", true},
		{"src/foo_test.go", true},
		{"service.pb.go", true},
		{"main.go", false},
	}
	for _, c := range cases {
		got := r.ShouldIgnorePath(c.path)
		if got != c.expect {
			t.Errorf("ShouldIgnorePath(%q) = %v, want %v", c.path, got, c.expect)
		}
	}
}

func TestShouldIgnoreSymbol(t *testing.T) {
	r := &Rules{Exclude: []string{"internal*", "test*", "TestMain"}}
	cases := []struct {
		name   string
		expect bool
	}{
		{"internalHelper", true},
		{"testSetup", true},
		{"TestMain", true},
		{"MyPublicFunc", false},
		{"HandleRequest", false},
	}
	for _, c := range cases {
		got := r.ShouldIgnoreSymbol(c.name)
		if got != c.expect {
			t.Errorf("ShouldIgnoreSymbol(%q) = %v, want %v", c.name, got, c.expect)
		}
	}
}

func TestNilRules(t *testing.T) {
	var r *Rules
	if r.ShouldIgnorePath("foo.go") {
		t.Error("nil rules should never ignore")
	}
	if r.ShouldIgnoreSymbol("foo") {
		t.Error("nil rules should never ignore")
	}
}

func TestLoadGitIgnore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nignored.go\n/generated/\n!important.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", ".gitignore"), []byte("local.go\nnested/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadGitIgnore(dir)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path   string
		expect bool
	}{
		{"ignored.go", true},
		{"sub/ignored.go", true},
		{"generated", true},
		{"generated/file.go", true},
		{"debug.log", true},
		{"important.log", false},
		{"pkg/local.go", true},
		{"pkg/sub/local.go", true},
		{"pkg/nested/file.go", true},
		{"other/local.go", false},
	}
	for _, c := range cases {
		if got := rules.ShouldIgnorePath(c.path); got != c.expect {
			t.Errorf("ShouldIgnorePath(%q) = %v, want %v", c.path, got, c.expect)
		}
	}
}
