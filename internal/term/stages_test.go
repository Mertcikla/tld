package term

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func testTracker(out *bytes.Buffer, names []string, terminal bool, now *time.Time) *StageTracker {
	return NewStageTracker(out, names, StageTrackerOptions{
		ForceTerminal:    terminal,
		DisableAnimation: true,
		Throttle:         -1,
		Width:            80,
		Now:              func() time.Time { return *now },
	})
}

func TestStageTrackerShiftsCompletedStagesAbovePinnedActive(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover", "Parse sources", "Index symbols"}, true, &now)

	tracker.Begin("Discover")
	now = now.Add(1 * time.Second)
	tracker.Begin("Parse sources")
	now = now.Add(2 * time.Second)
	tracker.Report("Index symbols", 3, 10, "internal/codeindex/indexer.go")

	got := out.String()
	discover := strings.Index(got, "✓ Discover")
	parse := strings.Index(got, "✓ Parse sources")
	active := strings.LastIndex(got, "\r\033[K")
	if discover < 0 || parse < 0 || active < 0 {
		t.Fatalf("missing expected output:\n%q", got)
	}
	if discover >= parse || parse >= active {
		t.Fatalf("completed stages should stack above the pinned active line:\n%q", got)
	}
	activeLine := got[active:]
	for _, want := range []string{"Index symbols", "3/10", "30%", "internal/codeindex/indexer.go"} {
		if !strings.Contains(activeLine, want) {
			t.Fatalf("active line missing %q:\n%q", want, activeLine)
		}
	}
}

func TestStageTrackerFinishCommitsActiveStage(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover"}, true, &now)

	tracker.Begin("Discover")
	now = now.Add(1500 * time.Millisecond)
	tracker.Finish()

	got := out.String()
	if !strings.Contains(got, "✓ Discover") {
		t.Fatalf("finished stage not committed:\n%q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("output should end with a committed line:\n%q", got)
	}
	tracker.Finish() // idempotent
}

func TestStageTrackerNonTerminalPrintsCompletedStages(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover", "Verify"}, false, &now)

	tracker.Begin("Discover")
	now = now.Add(time.Second)
	tracker.Begin("Verify")
	now = now.Add(time.Second)
	tracker.Finish()

	got := out.String()
	if strings.Contains(got, "\r") {
		t.Fatalf("non-terminal tracker must not move the cursor:\n%q", got)
	}
	if !strings.Contains(got, "✓ Discover") || !strings.Contains(got, "✓ Verify") {
		t.Fatalf("expected completed stages only:\n%q", got)
	}
}

func TestStageTrackerFailRendersError(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover"}, true, &now)

	tracker.Begin("Discover")
	now = now.Add(time.Second)
	tracker.Fail("Discover", errors.New("boom"))

	got := out.String()
	if !strings.Contains(got, "✗ Discover") || !strings.Contains(got, "boom") {
		t.Fatalf("failed stage not rendered:\n%q", got)
	}
}

func TestStageTrackerRotatesJokes(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	jokes := []string{"joke-alpha", "joke-beta", "joke-gamma"}
	tracker := NewStageTracker(&out, []string{"Discover"}, StageTrackerOptions{
		ForceTerminal:    true,
		DisableAnimation: true,
		Throttle:         -1,
		Width:            200,
		Now:              func() time.Time { return now },
		Jokes:            jokes,
		JokeInterval:     3 * time.Second,
	})

	tracker.Begin("Discover")
	seen := map[string]bool{}
	for i := 0; i < len(jokes); i++ {
		active := out.String()[strings.LastIndex(out.String(), "\r\033[K"):]
		for _, joke := range jokes {
			if strings.Contains(active, joke) {
				seen[joke] = true
			}
		}
		now = now.Add(3 * time.Second)
		tracker.Report("Discover", 1, 1, "")
	}
	if len(seen) != len(jokes) {
		t.Fatalf("expected every joke to rotate in, saw %v:\n%q", seen, out.String())
	}
}

func TestStageTrackerNoJokesByDefault(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover"}, true, &now)

	tracker.Begin("Discover")
	active := out.String()[strings.LastIndex(out.String(), "\r\033[K"):]
	if strings.Contains(active, "joke") {
		t.Fatalf("unexpected joke on active line:\n%q", active)
	}
}

func TestStageTrackerMessageKeepsActivePinnedBelow(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	tracker := testTracker(&out, []string{"Discover"}, true, &now)

	tracker.Begin("Discover")
	tracker.Message("warning: something")

	got := out.String()
	message := strings.Index(got, "warning: something")
	active := strings.LastIndex(got, "\r\033[K")
	if message < 0 {
		t.Fatalf("message not printed:\n%q", got)
	}
	if active < message {
		t.Fatalf("active line should be re-pinned below the message:\n%q", got)
	}
}
