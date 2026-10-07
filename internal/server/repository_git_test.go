package server

import (
	"math"
	"testing"
)

func TestAddLineCount(t *testing.T) {
	tests := []struct {
		name  string
		total uint32
		count uint64
		want  uint32
	}{
		{name: "ordinary count", total: 10, count: 20, want: 30},
		{name: "exact upper bound", total: 1, count: math.MaxUint32 - 1, want: math.MaxUint32},
		{name: "count exceeds uint32", count: uint64(math.MaxUint32) + 1, want: math.MaxUint32},
		{name: "total exceeds uint32", total: 1, count: math.MaxUint32, want: math.MaxUint32},
		{name: "maximum uint64", total: 1, count: math.MaxUint64, want: math.MaxUint32},
		{name: "saturated total", total: math.MaxUint32, count: 1, want: math.MaxUint32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addLineCount(tt.total, tt.count); got != tt.want {
				t.Fatalf("addLineCount(%d, %d) = %d, want %d", tt.total, tt.count, got, tt.want)
			}
		})
	}
}
