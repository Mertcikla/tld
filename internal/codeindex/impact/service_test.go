package impact

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
)

func TestForTargetTagsProgressUpdates(t *testing.T) {
	var got []indexer.Progress
	record := func(update indexer.Progress) { got = append(got, update) }

	forTarget(record, TargetBase)(indexer.Progress{Stage: "discover"})
	forTarget(record, TargetHead)(indexer.Progress{Stage: "discover"})
	forTarget(record, TargetHead)(indexer.Progress{Stage: "overlay"})

	want := []indexer.Progress{
		{Stage: "discover", Target: TargetBase},
		{Stage: "discover", Target: TargetHead},
		{Stage: "overlay", Target: TargetHead},
	}
	if len(got) != len(want) {
		t.Fatalf("updates = %+v", got)
	}
	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("update %d = %+v, want %+v", i, got[i], expected)
		}
	}
	if forTarget(nil, TargetBase) != nil {
		t.Fatal("nil progress must stay nil")
	}
}