package tagcolors

import (
	crand "crypto/rand"
	"fmt"
	"hash/fnv"
	"strings"
)

var SwatchColors = []string{
	"#F56565", "#ED8936", "#ECC94B", "#48BB78", "#38B2AC",
	"#4299E1", "#667EEA", "#9F7AEA", "#ED64A6", "#A0AEC0",
}

func PickUnusedColor(usedColors []string) string {
	used := make(map[string]bool)
	for _, c := range usedColors {
		used[strings.ToUpper(c)] = true
	}

	var pool []string
	for _, c := range SwatchColors {
		if !used[strings.ToUpper(c)] {
			pool = append(pool, c)
		}
	}

	source := pool
	if len(source) == 0 {
		return randomUnusedColor(used)
	}

	return source[randomIndex(len(source))]
}

func randomIndex(n int) int {
	if n <= 1 {
		return 0
	}
	var b [1]byte
	if _, err := crand.Read(b[:]); err == nil {
		return int(b[0]) % n
	}
	return 0
}

func randomUnusedColor(used map[string]bool) string {
	var b [3]byte
	for range 32 {
		if _, err := crand.Read(b[:]); err == nil {
			color := fmt.Sprintf("#%02X%02X%02X", b[0], b[1], b[2])
			if !used[color] {
				return color
			}
		}
	}
	return fallbackUnusedColor(used)
}

func fallbackUnusedColor(used map[string]bool) string {
	for i := 0; ; i++ {
		h := fnv.New32a()
		_, _ = fmt.Fprintf(h, "tld-tag-color-%d", i)
		sum := h.Sum32()
		color := fmt.Sprintf("#%06X", sum&0xFFFFFF)
		if !used[color] {
			return color
		}
	}
}
