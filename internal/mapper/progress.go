package mapper

// Progress describes coarse pipeline progress. Stages are stable identifiers:
// "clustering" (neighbor search, growing, splitting, sweeping), "binning" and
// "materializing".
type Progress struct {
	Stage   string
	Current int
	Total   int
	Detail  string
}

// ProgressFunc receives pipeline progress. It must be safe to call from the
// goroutine running the pipeline and must never change pipeline results.
type ProgressFunc func(Progress)

func report(progress ProgressFunc, stage string, current, total int, detail string) {
	if progress == nil {
		return
	}
	progress(Progress{Stage: stage, Current: current, Total: total, Detail: detail})
}
