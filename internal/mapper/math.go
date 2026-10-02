package mapper

import (
	"math"
	"sort"
)

// totalCmp mirrors Rust's f64::total_cmp (IEEE 754 totalOrder) so tie-breaking
// matches the reference implementation, including -0.0 < +0.0.
func totalCmp(x, y float64) int {
	l := int64(math.Float64bits(x))
	r := int64(math.Float64bits(y))
	l ^= int64(uint64(l>>63) >> 1)
	r ^= int64(uint64(r>>63) >> 1)
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
	}
}

func dot(a, b []float64) float64 {
	total := 0.0
	for i := 0; i < len(a) && i < len(b); i++ {
		total += a[i] * b[i]
	}
	return total
}

func add(total, value []float64) {
	for i := 0; i < len(total) && i < len(value); i++ {
		total[i] += value[i]
	}
}

func total(rows [][]float64, members []int) []float64 {
	dimension := 0
	if len(rows) > 0 {
		dimension = len(rows[0])
	}
	sum := make([]float64, dimension)
	for _, i := range members {
		add(sum, rows[i])
	}
	return sum
}

// normalize L2-normalizes finite, nonzero rows and maps non-finite or zero
// vectors to an all-zero sentinel of the inferred dimension.
func normalize(vectors [][]float64) ([][]float64, error) {
	dimension := 0
	if len(vectors) > 0 {
		dimension = len(vectors[0])
	}
	for _, v := range vectors {
		if len(v) != dimension {
			return nil, errVectorsNotRectangular
		}
	}
	out := make([][]float64, len(vectors))
	for idx, v := range vectors {
		scale := 0.0
		finite := true
		for _, x := range v {
			if !isFinite(x) {
				finite = false
				break
			}
			abs := math.Abs(x)
			if abs > scale {
				scale = abs
			}
		}
		if !finite || scale == 0.0 {
			out[idx] = make([]float64, dimension)
			continue
		}
		row := make([]float64, dimension)
		for i, x := range v {
			row[i] = x / scale
		}
		norm := math.Sqrt(dot(row, row))
		for i := range row {
			row[i] /= norm
		}
		out[idx] = row
	}
	return out, nil
}

func tightness(rows [][]float64, members []int) float64 {
	if len(members) < 2 {
		return 0.0
	}
	sum := total(rows, members)
	squared := 0.0
	for _, i := range members {
		squared += dot(rows[i], rows[i])
	}
	denominator := float64(len(members)) * float64(len(members)-1)
	value := (dot(sum, sum) - squared) / denominator
	return math.Max(-1.0, math.Min(1.0, value))
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0.0
	}
	return float64(n) / float64(d)
}

func sizeStats(sizes []int, policy SizePolicy) (float64, int, map[string]float64) {
	outside := 0
	violations := 0
	for _, s := range sizes {
		if !policy.Preferred(s) {
			outside += s
		}
		if !policy.Accepts(s) {
			violations++
		}
	}
	sorted := append([]int(nil), sizes...)
	sort.Ints(sorted)
	percentiles := map[string]float64{}
	if len(sorted) > 0 {
		stops := []struct {
			name string
			p    float64
		}{
			{"p0", 0.0}, {"p25", 0.25}, {"p50", 0.5}, {"p75", 0.75}, {"p100", 1.0},
		}
		for _, stop := range stops {
			position := stop.p * float64(len(sorted)-1)
			low := int(math.Floor(position))
			high := int(math.Ceil(position))
			percentiles[stop.name] = float64(sorted[low]) + float64(sorted[high]-sorted[low])*(position-float64(low))
		}
	}
	totalFacts := 0
	for _, s := range sizes {
		totalFacts += s
	}
	return ratio(outside, totalFacts), violations, percentiles
}
