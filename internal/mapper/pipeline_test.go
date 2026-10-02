package mapper

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

func testDataset(vectors [][]float64, root string) Dataset {
	facts := make([]Fact, len(vectors))
	for i := range vectors {
		facts[i] = Fact{
			ID:          fmt.Sprintf("id-%d", i),
			Path:        fmt.Sprintf("src/%d.rs", i),
			DisplayName: fmt.Sprintf("fact-%d", i),
			Language:    "rust",
		}
	}
	rootValue := root
	return Dataset{
		Snapshot: "snapshot",
		Profile:  "profile",
		Root:     &rootValue,
		Facts:    facts,
		Vectors:  vectors,
	}
}

func same(n int) [][]float64 {
	vectors := make([][]float64, n)
	for i := range vectors {
		vectors[i] = []float64{1.0, 0.0, 0.0}
	}
	return vectors
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertPartition(t *testing.T, result *PipelineResult, n int) {
	t.Helper()
	indices := make([]int, 0, n)
	for _, domain := range result.Domains {
		indices = append(indices, domain.Members...)
	}
	indices = append(indices, result.Leftovers...)
	sort.Ints(indices)
	expected := make([]int, n)
	for i := range expected {
		expected[i] = i
	}
	if !reflect.DeepEqual(indices, expected) {
		t.Fatalf("partition = %v, want %v", indices, expected)
	}
	for _, domain := range result.Domains {
		if !result.Options.ClusterSizes.Accepts(domain.Size()) {
			t.Fatalf("domain size %d violates bounds", domain.Size())
		}
	}
	if result.Metrics.ClusterHardSizeViolations != 0 {
		t.Fatalf("hard size violations = %d", result.Metrics.ClusterHardSizeViolations)
	}
	reasonKeys := make([]int, 0, len(result.LeftoverReasons))
	for index := range result.LeftoverReasons {
		reasonKeys = append(reasonKeys, index)
	}
	sort.Ints(reasonKeys)
	leftovers := append([]int(nil), result.Leftovers...)
	sort.Ints(leftovers)
	if !equalIntSlices(reasonKeys, leftovers) {
		t.Fatalf("reason keys = %v, want %v", reasonKeys, leftovers)
	}
}

func TestAllBoundariesAndForcedPartitionTerminate(t *testing.T) {
	for _, n := range []int{0, 1, 2, 4, 5, 14, 15, 16, 24, 25, 26, 30, 51, 100, 300} {
		opt := DefaultOptions()
		opt.SplitMaxPasses = 0
		result, err := RunPipeline(same(n), &opt)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		assertPartition(t, result, n)
		if n > 25 {
			if result.Metrics.FallbackPartitions == 0 {
				t.Fatalf("n=%d: expected fallback partitions", n)
			}
			for _, domain := range result.Domains {
				if domain.Size() > 15 {
					t.Fatalf("n=%d: domain size %d > ideal_max", n, domain.Size())
				}
			}
		}
		expectedLeftovers := 0
		if n == 1 {
			expectedLeftovers = 1
		}
		if len(result.Leftovers) != expectedLeftovers {
			t.Fatalf("n=%d leftovers = %v", n, result.Leftovers)
		}
	}
}

func TestInvalidAndHugeVectors(t *testing.T) {
	vectors := [][]float64{
		{1e308, 1e308},
		{1e308, 1e308},
		{math.NaN(), 1.0},
		{math.Inf(1), 0.0},
		{0.0, 0.0},
	}
	opt := DefaultOptions()
	result, err := RunPipeline(vectors, &opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	assertPartition(t, result, 5)
	if !reflect.DeepEqual(result.Leftovers, []int{2, 3, 4}) {
		t.Fatalf("leftovers = %v", result.Leftovers)
	}
	for _, reason := range result.LeftoverReasons {
		if len(reason) < len("invalid_vector") || reason[:len("invalid_vector")] != "invalid_vector" {
			t.Fatalf("reason = %q", reason)
		}
	}
	if _, err := RunPipeline([][]float64{{1.0}, {1.0, 2.0}}, &opt); err == nil {
		t.Fatal("expected rectangular validation error")
	}
	emptyResult, err := RunPipeline([][]float64{{}, {}}, &opt)
	if err != nil {
		t.Fatalf("empty run: %v", err)
	}
	if !reflect.DeepEqual(emptyResult.Leftovers, []int{0, 1}) {
		t.Fatalf("empty leftovers = %v", emptyResult.Leftovers)
	}
}

func TestDeterministicAcrossBlocksAndRepeatedRuns(t *testing.T) {
	vectors := make([][]float64, 100)
	for i := range vectors {
		row := make([]float64, 8)
		for j := range row {
			row[j] = math.Sin(float64(i*17 + j*7))
		}
		vectors[i] = row
	}
	base := DefaultOptions()
	first, err := RunPipeline(vectors, &base)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, blockSize := range []int{1, 3, 17, 512} {
		opt := DefaultOptions()
		opt.BlockSize = blockSize
		result, err := RunPipeline(vectors, &opt)
		if err != nil {
			t.Fatalf("block %d: %v", blockSize, err)
		}
		if !reflect.DeepEqual(first.Domains, result.Domains) {
			t.Fatalf("block %d: domains differ", blockSize)
		}
		if !reflect.DeepEqual(first.Leftovers, result.Leftovers) {
			t.Fatalf("block %d: leftovers differ", blockSize)
		}
		assertPartition(t, result, 100)
	}
}

func TestPolicyValidationAndUnknownConfigurationFields(t *testing.T) {
	invalidSizes := []SizePolicy{
		{Min: 1, IdealMin: 5, IdealMax: 15, Max: 25},
		{Min: 2, IdealMin: 16, IdealMax: 15, Max: 25},
		{Min: 2, IdealMin: 5, IdealMax: 15, Max: 14},
	}
	for _, sizes := range invalidSizes {
		if err := sizes.Validate(); err == nil {
			t.Fatalf("expected invalid size policy %+v", sizes)
		}
	}
	nan := math.NaN()
	invalidOptions := []Options{}
	opt := DefaultOptions()
	opt.BlockSize = 0
	invalidOptions = append(invalidOptions, opt)
	opt = DefaultOptions()
	opt.Neighbors = 0
	invalidOptions = append(invalidOptions, opt)
	opt = DefaultOptions()
	opt.SplitStep = 0.0
	invalidOptions = append(invalidOptions, opt)
	opt = DefaultOptions()
	opt.SweepFloor = &nan
	invalidOptions = append(invalidOptions, opt)
	opt = DefaultOptions()
	opt.UnclusteredTarget = 1.1
	invalidOptions = append(invalidOptions, opt)
	for _, candidate := range invalidOptions {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("expected invalid options %+v", candidate)
		}
	}
	var unknown Options
	if err := DecodeJSON([]byte(`{"top_n":10}`), &unknown); err == nil {
		t.Fatal("expected unknown field error")
	}
	var partial Options
	if err := DecodeJSON([]byte(`{"member_floor":0.6}`), &partial); err != nil {
		t.Fatalf("partial decode: %v", err)
	}
	if partial.MemberFloor != 0.6 || partial.Neighbors != 10 {
		t.Fatalf("partial = %+v", partial)
	}
	var disabled Options
	if err := DecodeJSON([]byte(`{"sweep_floor":null}`), &disabled); err != nil {
		t.Fatalf("disabled decode: %v", err)
	}
	if disabled.SweepFloor != nil {
		t.Fatalf("sweep_floor = %v, want nil", disabled.SweepFloor)
	}
}

func TestBinBoundsAndPartitionValidation(t *testing.T) {
	data := testDataset(same(4), "repo")
	opt := DefaultOptions()
	result, err := RunPipeline(data.Vectors, &opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	binOpt := DefaultBinOptions()
	binOpt.Sizes.Max = 20
	if _, err := BuildBins(&data, result, &binOpt); err == nil {
		t.Fatal("expected bin bounds error")
	}
	result.Leftovers = append(result.Leftovers, 0)
	defaultBins := DefaultBinOptions()
	if _, err := BuildBins(&data, result, &defaultBins); err == nil {
		t.Fatal("expected partition error")
	}
	if _, err := Assignments([]Domain{{Members: []int{0, 0}, Tightness: 1.0}}, 2); err == nil {
		t.Fatal("expected duplicate membership error")
	}
	if _, err := Assignments([]Domain{{Members: []int{2}, Tightness: 1.0}}, 2); err == nil {
		t.Fatal("expected out-of-range membership error")
	}
}

func TestWindowsAndDotPathsHaveCorrectTreeCounts(t *testing.T) {
	data := testDataset(same(6), "repo")
	for i := range data.Facts {
		if i%2 == 0 {
			data.Facts[i].Path = fmt.Sprintf(".\\src\\deep\\%d.rs", i)
		} else {
			data.Facts[i].Path = fmt.Sprintf("./src/deep/%d.rs", i)
		}
	}
	opt := DefaultOptions()
	result, err := RunPipeline(data.Vectors, &opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defaultBins := DefaultBinOptions()
	bins, err := BuildBins(&data, result, &defaultBins)
	if err != nil {
		t.Fatalf("build bins: %v", err)
	}
	if bins.RootCounts.Facts != 6 {
		t.Fatalf("root facts = %d, want 6", bins.RootCounts.Facts)
	}
	if bins.Units[0].Folder != "src/deep" {
		t.Fatalf("folder = %q", bins.Units[0].Folder)
	}
	if len(bins.Tree.Children) == 0 || bins.Tree.Children[0].Path != "src" {
		t.Fatalf("tree children = %+v", bins.Tree.Children)
	}
}

func TestInvalidVectorComponentsRoundtripThroughJSONAsNull(t *testing.T) {
	data := testDataset([][]float64{{math.Inf(1), 1.0}, {math.NaN(), 0.0}}, "repo")
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	vectors, ok := document["vectors"].([]any)
	if !ok || len(vectors) == 0 {
		t.Fatalf("vectors = %v", document["vectors"])
	}
	firstRow, ok := vectors[0].([]any)
	if !ok || len(firstRow) == 0 || firstRow[0] != nil {
		t.Fatalf("first component = %v, want null", vectors[0])
	}
	var restored Dataset
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("unmarshal dataset: %v", err)
	}
	opt := DefaultOptions()
	result, err := RunPipeline(restored.Vectors, &opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !reflect.DeepEqual(result.Leftovers, []int{0, 1}) {
		t.Fatalf("leftovers = %v", result.Leftovers)
	}
}
