package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mertcikla/codeindex/watch"
)

func TestWatchExcludesRuntimeData(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name, dataDir string
		want          []string
	}{
		{"inside checkout", filepath.Join(root, "data", "dev"), []string{"generated", "data/dev"}},
		{"outside checkout", t.TempDir(), []string{"generated"}},
		{"no data directory", "", []string{"generated"}},
		{"checkout root", root, []string{"generated", "tld.db", "tld.db-wal", "tld.db-shm", "tld.db-journal"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			configured := []string{"generated"}
			got := watchExcludes(root, test.dataDir, configured)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("excludes = %v, want %v", got, test.want)
			}
			if len(configured) != 1 || configured[0] != "generated" {
				t.Fatal("modified configured excludes")
			}
		})
	}
}

func TestWatchRuntimeWritesDoNotDebounceButSourceEditsDo(t *testing.T) {
	for _, atRoot := range []bool{false, true} {
		t.Run(fmt.Sprintf("data_at_root=%v", atRoot), func(t *testing.T) {
			root := t.TempDir()
			dataDir := root
			if !atRoot {
				dataDir = filepath.Join(root, "data", "dev")
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "main.go")
			if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			states := make(chan string, 100)
			captured := make(chan struct{}, 100)
			detector := watch.NewDetector(watch.Options{
				Root: root, Exclude: watchExcludes(root, dataDir, nil),
				Debounce: 20 * time.Millisecond, MaxWait: time.Second, PollInterval: time.Hour,
				OnState: func(state string) {
					states <- state
					// The status callback itself writes a heartbeat to this file.
					_ = os.WriteFile(filepath.Join(dataDir, "tld.db-wal"), []byte(state), 0o600)
				},
			})
			defer func() { _ = detector.Close() }()
			done := make(chan error, 1)
			go func() {
				signature, err := detector.Next(ctx, "original", func(context.Context) (string, error) {
					captured <- struct{}{}
					content, readErr := os.ReadFile(source)
					return string(content), readErr
				})
				if err == nil && signature != "edited" {
					err = fmt.Errorf("signature = %q, want edited", signature)
				}
				done <- err
			}()
			if state := <-states; state != "idle" {
				t.Fatalf("initial state = %q", state)
			}
			for _, name := range []string{"tld.db", "tld.db-wal", "tld.db-shm", "tld.db-journal"} {
				if err := os.WriteFile(filepath.Join(dataDir, name), []byte("heartbeat"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case state := <-states:
				t.Fatalf("runtime write triggered %q", state)
			case <-captured:
				t.Fatal("runtime write triggered a Git capture")
			case <-time.After(150 * time.Millisecond):
			}
			if err := os.WriteFile(source, []byte("edited"), 0o600); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("source edit did not wake the filesystem watcher")
			}
			if state := <-states; state != "debouncing" {
				t.Fatalf("source edit state = %q, want debouncing", state)
			}
		})
	}
}
