package index

import (
	"context"
	"time"

	"github.com/mertcikla/codeindex/watch"
)

const maxWatchDebounce = 10 * time.Second

// watchDebounce backs off filesystem bursts that do not change Git inputs.
// It is owned by the detector goroutine, not the heartbeat goroutine.
type watchDebounce struct {
	base, delay time.Duration
	deadline    time.Time
}

func newWatchDebounce(base time.Duration) *watchDebounce {
	if base <= 0 {
		base = watch.DefaultDebounce
	}
	base = min(base, maxWatchDebounce)
	return &watchDebounce{base: base, delay: base}
}

func (d *watchDebounce) begin(now time.Time) {
	d.deadline = now.Add(d.delay)
}

// The upstream detector supplies the initial quiet window and collects events.
// Extend its capture to the adaptive deadline without recreating filesystem
// watches. The deadline also bounds delays during continuous event bursts.
func (d *watchDebounce) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	remaining := time.Until(d.deadline)
	if remaining <= 0 {
		return nil
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *watchDebounce) finish(changed bool) {
	if changed {
		d.delay = d.base
	} else if !d.deadline.IsZero() {
		d.delay = min(d.delay*2, maxWatchDebounce)
	}
	d.deadline = time.Time{}
}
