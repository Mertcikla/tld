package index

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mertcikla/codeindex/watch"
)

func TestWatchDebounceBacksOffToTenSeconds(t *testing.T) {
	debounce := newWatchDebounce(100 * time.Millisecond)
	now := time.Now()
	for _, want := range []time.Duration{
		100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond,
		800 * time.Millisecond, 1600 * time.Millisecond, 3200 * time.Millisecond,
		6400 * time.Millisecond, 10 * time.Second, 10 * time.Second,
	} {
		debounce.begin(now)
		if got := debounce.deadline.Sub(now); got != want {
			t.Fatalf("debounce = %s, want %s", got, want)
		}
		debounce.finish(false)
	}
	debounce.begin(now)
	debounce.finish(true)
	if debounce.delay != 100*time.Millisecond || !debounce.deadline.IsZero() {
		t.Fatalf("real edit did not reset backoff: %+v", debounce)
	}
}

func TestWatchDebounceIgnoresUnchangedIdleCaptures(t *testing.T) {
	debounce := newWatchDebounce(100 * time.Millisecond)
	debounce.begin(time.Now())
	debounce.finish(false)
	for range 10 {
		debounce.finish(false)
	}
	if debounce.delay != 200*time.Millisecond {
		t.Fatalf("idle checks increased backoff: %s", debounce.delay)
	}
	debounce.finish(true)
	if debounce.delay != debounce.base {
		t.Fatal("a real edit from a failsafe capture did not reset backoff")
	}
}

func TestWatchDebounceBoundsAndCancellation(t *testing.T) {
	if got := newWatchDebounce(0).delay; got != watch.DefaultDebounce {
		t.Fatalf("default = %s", got)
	}
	if got := newWatchDebounce(time.Minute).delay; got != maxWatchDebounce {
		t.Fatalf("configured delay exceeded ceiling: %s", got)
	}
	debounce := newWatchDebounce(time.Second)
	debounce.begin(time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := debounce.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled debounce = %v", err)
	}
	debounce.deadline = time.Now().Add(-time.Second)
	if err := debounce.wait(context.Background()); err != nil {
		t.Fatalf("elapsed debounce = %v", err)
	}
}
