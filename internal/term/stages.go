package term

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	stageSpinnerFrames  = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	stageAnimationTick  = 90 * time.Millisecond
	stageNameMinWidth   = 12
	stageNameMaxWidth   = 28
	defaultJokeInterval = 3 * time.Second
)

// StageStatus is the lifecycle state of a tracked stage.
type StageStatus int

const (
	StagePending StageStatus = iota
	StageActive
	StageDone
	StageFailed
)

// StageTrackerOptions configures a StageTracker.
type StageTrackerOptions struct {
	// ForceTerminal renders the pinned/animated view even for non-terminal
	// writers. Intended for tests.
	ForceTerminal bool
	// DisableAnimation keeps the pinned line static (no spinner ticks).
	DisableAnimation bool
	// Width overrides the detected terminal width.
	Width int
	// Throttle is the minimum delay between re-renders of the active line.
	Throttle time.Duration
	// Jokes rotates an occasional quip on the active line, switching every
	// JokeInterval. The order is shuffled so successive indexes feel random.
	Jokes []string
	// JokeInterval is how long each joke stays on screen. Defaults to 3s.
	JokeInterval time.Duration
	// Now overrides the clock. Intended for tests.
	Now func() time.Time
}

type stageState struct {
	name     string
	status   StageStatus
	started  time.Time
	finished time.Time
	total    int64
	current  int64
	detail   string
	err      string
}

// StageTracker renders a stack of indexing stages: completed stages become
// permanent lines that move upward, while the active stage stays pinned to the
// bottom and updates in place with a spinner, counts, elapsed time, and rate.
type StageTracker struct {
	out      io.Writer
	now      func() time.Time
	throttle time.Duration
	width    int
	terminal bool
	animate  bool

	jokes        []string
	jokeInterval time.Duration
	jokeStart    time.Time

	mu         sync.Mutex
	order      []string
	states     map[string]*stageState
	nameWidth  int
	active     string
	lastRender time.Time
	rendered   bool
	finished   bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewStageTracker creates a tracker over out. stages pre-registers display names
// so the label column is sized consistently; later stages may still be added.
func NewStageTracker(out io.Writer, stages []string, opts StageTrackerOptions) *StageTracker {
	if out == nil {
		out = io.Discard
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	throttle := opts.Throttle
	if throttle == 0 {
		throttle = defaultProgressThrottle
	}
	t := &StageTracker{
		out:      out,
		now:      now,
		throttle: throttle,
		width:    terminalWidth(out, opts.Width),
		terminal: opts.ForceTerminal || IsTerminal(out),
		states:   map[string]*stageState{},
	}
	t.animate = t.terminal && !opts.DisableAnimation
	if len(opts.Jokes) > 0 {
		t.jokes = append([]string(nil), opts.Jokes...)
		rand.Shuffle(len(t.jokes), func(i, j int) { t.jokes[i], t.jokes[j] = t.jokes[j], t.jokes[i] })
		t.jokeInterval = opts.JokeInterval
		if t.jokeInterval <= 0 {
			t.jokeInterval = defaultJokeInterval
		}
		t.jokeStart = t.now()
	}
	for _, name := range stages {
		t.addLocked(name)
	}
	return t
}

// Begin marks a stage active, completing the previously active stage.
func (t *StageTracker) Begin(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	st := t.addLocked(stage)
	if t.active == st.name {
		return
	}
	t.completeActiveLocked()
	st.status = StageActive
	st.started = t.now()
	st.finished = time.Time{}
	st.current = 0
	st.total = 0
	st.detail = ""
	st.err = ""
	t.active = st.name
	t.lastRender = time.Time{}
	t.ensureAnimationLocked()
	t.renderActiveLocked()
}

// Report sets the active stage's progress counters and detail, beginning the
// stage first when it is not already active.
func (t *StageTracker) Report(stage string, current, total int64, detail string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	st := t.addLocked(stage)
	if t.active != st.name {
		t.completeActiveLocked()
		st.status = StageActive
		st.started = t.now()
		st.finished = time.Time{}
		st.current = 0
		st.total = 0
		st.detail = ""
		st.err = ""
		t.active = st.name
		t.lastRender = time.Time{}
		t.ensureAnimationLocked()
	}
	if total > 0 {
		st.total = total
	}
	if current >= 0 {
		st.current = current
	}
	if d := strings.TrimSpace(detail); d != "" {
		st.detail = d
	}
	t.renderActiveLocked()
}

// Complete marks a stage done and commits its finished line.
func (t *StageTracker) Complete(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.states[strings.TrimSpace(stage)]
	if !ok || st.status != StageActive {
		return
	}
	st.status = StageDone
	st.finished = t.now()
	t.commitLocked(t.lineLocked(st))
	if t.active == st.name {
		t.active = ""
	}
}

// Fail marks a stage failed, commits its line, and unpins it.
func (t *StageTracker) Fail(stage string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.addLocked(stage)
	if st.started.IsZero() {
		st.started = t.now()
	}
	st.status = StageFailed
	st.finished = t.now()
	if err != nil {
		st.err = err.Error()
	}
	t.commitLocked(t.lineLocked(st))
	if t.active == st.name {
		t.active = ""
	}
}

// Message prints a permanent line above the pinned active stage.
func (t *StageTracker) Message(text string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.commitLocked("  " + text)
	if t.active != "" {
		t.lastRender = time.Time{}
		t.renderActiveLocked()
	}
}

// Finish completes the active stage, stops animation, and clears the pinned
// line. It is safe to call more than once.
func (t *StageTracker) Finish() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.finished {
		t.completeActiveLocked()
		if t.terminal && t.rendered {
			_, _ = fmt.Fprint(t.out, "\r\033[K")
			t.rendered = false
		}
		t.finished = true
	}
	t.mu.Unlock()
	t.stopAnimation()
}

func (t *StageTracker) addLocked(name string) *stageState {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "working"
	}
	if st, ok := t.states[name]; ok {
		return st
	}
	st := &stageState{name: name, status: StagePending}
	t.states[name] = st
	t.order = append(t.order, name)
	if w := len([]rune(name)); w > t.nameWidth {
		if w > stageNameMaxWidth {
			w = stageNameMaxWidth
		}
		t.nameWidth = w
	}
	return st
}

func (t *StageTracker) completeActiveLocked() {
	if t.active == "" {
		return
	}
	st := t.states[t.active]
	st.status = StageDone
	st.finished = t.now()
	t.commitLocked(t.lineLocked(st))
	t.active = ""
}

func (t *StageTracker) commitLocked(line string) {
	if !t.terminal {
		_, _ = fmt.Fprintln(t.out, line)
		return
	}
	if t.rendered {
		_, _ = fmt.Fprint(t.out, "\r\033[K")
		t.rendered = false
	}
	_, _ = fmt.Fprintln(t.out, line)
}

func (t *StageTracker) renderActiveLocked() {
	if !t.terminal || t.active == "" {
		return
	}
	now := t.now()
	if !t.rendered && t.throttle > 0 && !t.lastRender.IsZero() && now.Sub(t.lastRender) < t.throttle {
		return
	}
	st := t.states[t.active]
	_, _ = fmt.Fprintf(t.out, "\r\033[K%s", t.activeLineLocked(st, now))
	t.rendered = true
	t.lastRender = now
}

func (t *StageTracker) activeLineLocked(st *stageState, now time.Time) string {
	pad := t.padNameLocked(st.name)
	frame := t.spinnerFrameLocked(st, now)

	plain := "  " + frame + " " + pad
	styled := "  " + Colorize(t.out, ColorCyan, frame) + " " + Colorize(t.out, ColorBold, pad)

	if st.total > 0 {
		percent := int((float64(st.current) / float64(st.total)) * 100)
		if percent > 100 {
			percent = 100
		}
		segment := fmt.Sprintf(" %d/%d %d%%", st.current, st.total, percent)
		plain += segment
		styled += segment
	}

	elapsed := now.Sub(st.started)
	if elapsed < 0 {
		elapsed = 0
	}
	duration := formatDuration(elapsed.Round(time.Second))
	plain += " " + duration
	styled += " " + Dim(t.out, duration)

	if st.current > 0 && st.total > 0 {
		if seconds := elapsed.Seconds(); seconds > 0 {
			segment := fmt.Sprintf(" %.1f/s", float64(st.current)/seconds)
			plain += segment
			styled += segment
		}
	}

	joke := t.jokeLocked(now)
	if r := []rune(joke); len(r) > t.width/2 {
		joke = truncateEnd(joke, t.width/2)
	}
	jokeSpace := 0
	if joke != "" {
		jokeSpace = len([]rune(joke)) + 1
	}

	if st.detail != "" {
		remaining := t.width - len([]rune(plain)) - jokeSpace - 1
		if remaining > 3 {
			detail := truncateMiddle(st.detail, remaining)
			plain += " " + detail
			styled += " " + Dim(t.out, detail)
		}
	}

	if joke != "" {
		remaining := t.width - len([]rune(plain)) - 1
		if remaining > 3 {
			joke = truncateEnd(joke, remaining)
			plain += " " + joke
			styled += " " + Colorize(t.out, ColorYellow, joke)
		}
	}

	if len([]rune(plain)) > t.width {
		return truncateEnd(plain, t.width)
	}
	return styled
}

// jokeLocked returns the joke for the current interval, cycling through a
// shuffled list so the quips change every JokeInterval without repeating until
// the list wraps.
func (t *StageTracker) jokeLocked(now time.Time) string {
	if len(t.jokes) == 0 || t.jokeInterval <= 0 {
		return ""
	}
	elapsed := now.Sub(t.jokeStart)
	if elapsed < 0 {
		elapsed = 0
	}
	return t.jokes[int(elapsed/t.jokeInterval)%len(t.jokes)]
}

func (t *StageTracker) lineLocked(st *stageState) string {
	pad := t.padNameLocked(st.name)
	symbol := "✓"
	color := ColorGreen
	if st.status == StageFailed {
		symbol = "✗"
		color = ColorRed
	}

	duration := ""
	if !st.started.IsZero() && !st.finished.IsZero() {
		d := st.finished.Sub(st.started)
		if d < 0 {
			d = 0
		}
		duration = formatDuration(d.Round(time.Second))
	}

	plain := "  " + symbol + " " + pad
	styled := "  " + Colorize(t.out, color, symbol) + " " + pad
	if st.total > 0 {
		counts := fmt.Sprintf("  %d/%d", st.current, st.total)
		plain += counts
		styled += counts
	}
	if duration != "" {
		plain += "  " + duration
		styled += "  " + Dim(t.out, duration)
	}
	if st.status == StageFailed && st.err != "" {
		remaining := t.width - len([]rune(plain)) - 2
		message := st.err
		if remaining > 3 {
			message = truncateEnd(st.err, remaining)
		}
		plain += "  " + message
		styled += "  " + Colorize(t.out, ColorRed, message)
	}
	if len([]rune(plain)) > t.width {
		return truncateEnd(plain, t.width)
	}
	return styled
}

func (t *StageTracker) padNameLocked(name string) string {
	width := t.nameWidth
	if width < stageNameMinWidth {
		width = stageNameMinWidth
	}
	return fmt.Sprintf("%-*s", width, name)
}

func (t *StageTracker) spinnerFrameLocked(st *stageState, now time.Time) string {
	if !t.animate {
		return "•"
	}
	elapsed := now.Sub(st.started)
	if elapsed < 0 {
		elapsed = 0
	}
	frames := []rune(stageSpinnerFrames)
	return string(frames[int(elapsed/stageAnimationTick)%len(frames)])
}

func (t *StageTracker) ensureAnimationLocked() {
	if !t.animate || t.stopCh != nil || t.finished {
		return
	}
	stop := make(chan struct{})
	t.stopCh = stop
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		ticker := time.NewTicker(stageAnimationTick)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				t.mu.Lock()
				if t.active != "" {
					t.renderActiveLocked()
				}
				t.mu.Unlock()
			}
		}
	}()
}

func (t *StageTracker) stopAnimation() {
	t.mu.Lock()
	stop := t.stopCh
	t.stopCh = nil
	t.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	t.wg.Wait()
}
