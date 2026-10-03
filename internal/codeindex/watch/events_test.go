package watch

import (
	"context"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestIgnoredPaths(t *testing.T) {
	d := &Detector{opts: Options{Root: "/repo", Exclude: []string{"generated"}}}
	ignored := []string{
		".git/objects/ab/cdef", ".git/logs/HEAD", ".tld/elements.yaml",
		"node_modules/pkg/index.js", "dist/app.js", "generated/api.go",
		"schema.scip", "notes.txt~", ".#lock",
	}
	for _, rel := range ignored {
		if !d.ignored(rel) {
			t.Errorf("expected %q to be ignored", rel)
		}
	}
	interesting := []string{".git/index", ".git/HEAD", ".git/refs/heads/main", "src/main.go", "README.md"}
	for _, rel := range interesting {
		if d.ignored(rel) {
			t.Errorf("expected %q to matter", rel)
		}
	}
}

func TestPollingFallbackReturnsChangedSignature(t *testing.T) {
	d := &Detector{opts: Options{Root: t.TempDir(), PollInterval: 5 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	calls := 0
	current := "one"
	capture := func(context.Context) (string, error) {
		calls++
		if calls >= 2 {
			return "two", nil
		}
		return "one", nil
	}
	got, err := d.Next(ctx, current, capture)
	if err != nil {
		t.Fatal(err)
	}
	if got != "two" {
		t.Fatalf("signature = %q, want two", got)
	}
}

func TestPollingFallbackStopsOnContext(t *testing.T) {
	d := &Detector{opts: Options{Root: t.TempDir(), PollInterval: 5 * time.Millisecond}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Next(ctx, "same", func(context.Context) (string, error) { return "same", nil }); err == nil {
		t.Fatal("expected context error")
	}
}

func TestSettleReportsDebouncing(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	var states []string
	d := &Detector{watcher: w, opts: Options{Debounce: time.Millisecond, MaxWait: time.Second, OnState: func(state string) { states = append(states, state) }}}
	signature, err := d.settle(context.Background(), func(context.Context) (string, error) { return "changed", nil })
	if err != nil || signature != "changed" || len(states) != 1 || states[0] != "debouncing" {
		t.Fatalf("settle: %s %v states %v", signature, err, states)
	}
}
