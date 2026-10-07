package parser

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceLineHelpersPreservePositions(t *testing.T) {
	for _, data := range []string{"", "a", "a\n", " a \r\n b \n\nlast", strings.Repeat("line\n", 100) + "needle"} {
		b := []byte(data)
		lines := bytes.Split(b, []byte("\n"))
		for n := -1; n <= len(lines)+1; n++ {
			want := ""
			if n >= 1 && n <= len(lines) {
				want = strings.TrimSpace(string(lines[n-1]))
			}
			if got := sourceLine(b, n); got != want {
				t.Fatalf("sourceLine(%q,%d)=%q want %q", data, n, got, want)
			}
		}
		for _, token := range []string{"", "a", "b", "needle", "missing", "a\nb"} {
			want := 1
			for i, line := range lines {
				if bytes.Contains(line, []byte(token)) {
					want = i + 1
					break
				}
			}
			if got := lineOfString(b, token); got != want {
				t.Fatalf("lineOfString(%q,%q)=%d want %d", data, token, got, want)
			}
		}
	}
}

func BenchmarkContractSourceLookup(b *testing.B) {
	data := []byte(strings.Repeat("{\"schema\": \"value\"},\n", 20000) + "needle\n")
	b.Run("split", func(b *testing.B) {
		for b.Loop() {
			for i, line := range bytes.Split(data, []byte("\n")) {
				if bytes.Contains(line, []byte("needle")) {
					_ = i
					break
				}
			}
		}
	})
	b.Run("search", func(b *testing.B) {
		for b.Loop() {
			lineOfString(data, "needle")
		}
	})
}
