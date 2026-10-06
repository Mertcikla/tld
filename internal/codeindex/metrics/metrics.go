// Package metrics collects coarse-grained, per-stage performance metrics from
// the codeindex index and map pipelines. A "stage" is one of the pipelines'
// own coarse phases (discover, tree-sitter, scip, loading, grouping, ...) rather
// than an individual file or fact, so the overhead is negligible and the output
// stays readable next to the pipeline's progress reporting.
package metrics

import "time"

// Stage is one coarse pipeline phase: its wall-clock cost and the number of
// items it produced (sources, projects, facts, ...). Items is best-effort and
// stage-specific.
type Stage struct {
	Name       string  `json:"stage"`
	DurationMS float64 `json:"duration_ms"`
	Items      int64   `json:"items"`
}

// Collector accumulates per-stage timings and item counts. A nil *Collector is
// valid and records nothing, so callers can always pass a zero-value collector
// pointer without nil checks.
type Collector struct {
	stages map[string]*Stage
	order  []string
	total  time.Duration
}

// New returns an empty Collector.
func New() *Collector {
	return &Collector{stages: map[string]*Stage{}}
}

// Measure starts timing stage and returns a function that records the elapsed
// time together with the stage's item count. Calling the returned function more
// than once accumulates. When the collector is nil, the returned function is a
// no-op.
func (c *Collector) Measure(stage string) func(items int64) {
	if c == nil {
		return func(int64) {}
	}
	start := time.Now()
	return func(items int64) { c.Record(stage, time.Since(start), items) }
}

// Record adds a completed measurement to stage. Durations and item counts
// accumulate, so a phase that runs more than once (for example discover, which
// runs before and after indexing, or an indexing retry) is represented by its
// total cost.
func (c *Collector) Record(stage string, duration time.Duration, items int64) {
	if c == nil {
		return
	}
	s, ok := c.stages[stage]
	if !ok {
		s = &Stage{Name: stage}
		c.stages[stage] = s
		c.order = append(c.order, stage)
	}
	s.DurationMS += float64(duration) / float64(time.Millisecond)
	s.Items += items
	c.total += duration
}

// Stages returns the accumulated stages in first-seen order.
func (c *Collector) Stages() []Stage {
	if c == nil {
		return nil
	}
	out := make([]Stage, 0, len(c.order))
	for _, name := range c.order {
		out = append(out, *c.stages[name])
	}
	return out
}

// Total returns the summed wall-clock duration across every recorded stage.
func (c *Collector) Total() time.Duration {
	if c == nil {
		return 0
	}
	return c.total
}

// Merge adds every stage from other into c, accumulating matching names. It lets
// a caller combine an index run and a map run into a single report.
func (c *Collector) Merge(other *Collector) {
	for _, s := range other.Stages() {
		c.Record(s.Name, time.Duration(s.DurationMS*float64(time.Millisecond)), s.Items)
	}
}
