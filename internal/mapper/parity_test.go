package mapper

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

// The fixture is the shared Python/Rust parity corpus copied from
// ../mapper/tests/fixtures/python_pipeline.json. Every case asserts cluster
// membership, leftovers, reasons, metrics, folder bins, hierarchy counts and
// bin metrics match the reference pipeline.
type parityFixtures struct {
	FormatVersion int          `json:"format_version"`
	Cases         []parityCase `json:"cases"`
}

type parityCase struct {
	Name          string            `json:"name"`
	Dataset       Dataset           `json:"dataset"`
	Configuration Configuration     `json:"configuration"`
	Expected      parityExpectation `json:"expected"`
}

type parityExpectation struct {
	Members    [][]int                     `json:"members"`
	Leftovers  []int                       `json:"leftovers"`
	Reasons    map[string]string           `json:"reasons"`
	Metrics    map[string]float64          `json:"metrics"`
	Folders    map[string]parityFolderBins `json:"folders"`
	RootCounts FolderCounts                `json:"root_counts"`
	BinMetrics parityBinMetrics            `json:"bin_metrics"`
}

type parityFolderBins struct {
	Sizes      []int `json:"sizes"`
	Standalone []int `json:"standalone"`
}

type parityBinMetrics struct {
	BinOutsideIdealFraction float64 `json:"bin_outside_ideal_fraction"`
}

func loadParityFixtures(t *testing.T) []parityCase {
	t.Helper()
	data, err := os.ReadFile("testdata/python_pipeline.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixtures parityFixtures
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return fixtures.Cases
}

func TestParity(t *testing.T) {
	cases := loadParityFixtures(t)
	if len(cases) == 0 {
		t.Fatal("no parity cases loaded")
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			checkParityCase(t, testCase)
		})
	}
}

func checkParityCase(t *testing.T, testCase parityCase) {
	t.Helper()
	if err := testCase.Dataset.Validate(); err != nil {
		t.Fatalf("dataset invalid: %v", err)
	}
	result, err := RunPipeline(testCase.Dataset.Vectors, &testCase.Configuration.Clustering)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	bins, err := BuildBins(&testCase.Dataset, result, &testCase.Configuration.Binning)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}

	members := make([][]int, len(result.Domains))
	for i, domain := range result.Domains {
		members[i] = append([]int(nil), domain.Members...)
	}
	sort.Slice(members, func(i, j int) bool { return lessIntSlice(members[i], members[j]) })
	if !reflect.DeepEqual(members, testCase.Expected.Members) {
		t.Fatalf("memberships:\n got %v\nwant %v", members, testCase.Expected.Members)
	}
	if !reflect.DeepEqual(result.Leftovers, testCase.Expected.Leftovers) {
		t.Fatalf("leftovers: got %v want %v", result.Leftovers, testCase.Expected.Leftovers)
	}
	reasons := map[string]string{}
	for index, reason := range result.LeftoverReasons {
		reasons[strconv.Itoa(index)] = reason
	}
	if !reflect.DeepEqual(reasons, testCase.Expected.Reasons) {
		t.Fatalf("reasons: got %v want %v", reasons, testCase.Expected.Reasons)
	}
	actualMetrics := numericMetrics(t, result.Metrics)
	for key, expected := range testCase.Expected.Metrics {
		actual, ok := actualMetrics[key]
		if !ok {
			t.Fatalf("missing metric %q", key)
		}
		if math.Abs(actual-expected) >= 1e-10 {
			t.Fatalf("metric %q: got %v want %v", key, actual, expected)
		}
	}
	folders := map[string]parityFolderBins{}
	for folder, entries := range bins.Folders {
		sizes := make([]int, len(entries.Bins))
		for i, bin := range entries.Bins {
			sizes[i] = bin.Size
		}
		sort.Ints(sizes)
		standalone := entries.Standalone
		if standalone == nil {
			standalone = []int{}
		}
		folders[folder] = parityFolderBins{Sizes: sizes, Standalone: standalone}
	}
	if !reflect.DeepEqual(folders, testCase.Expected.Folders) {
		t.Fatalf("folder bins:\n got %v\nwant %v", folders, testCase.Expected.Folders)
	}
	if !reflect.DeepEqual(bins.RootCounts, testCase.Expected.RootCounts) {
		t.Fatalf("root counts: got %+v want %+v", bins.RootCounts, testCase.Expected.RootCounts)
	}
	if math.Abs(bins.Metrics.BinOutsideIdealFraction-testCase.Expected.BinMetrics.BinOutsideIdealFraction) >= 1e-10 {
		t.Fatalf("bin outside ideal: got %v want %v", bins.Metrics.BinOutsideIdealFraction, testCase.Expected.BinMetrics.BinOutsideIdealFraction)
	}
	if bins.Metrics.BinHardSizeViolations != 0 {
		t.Fatalf("bin hard size violations: %d", bins.Metrics.BinHardSizeViolations)
	}
}

func numericMetrics(t *testing.T, metrics Metrics) map[string]float64 {
	t.Helper()
	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	out := map[string]float64{}
	for key, value := range raw {
		var number float64
		if err := json.Unmarshal(value, &number); err == nil {
			out[key] = number
		}
	}
	return out
}

func lessIntSlice(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
