package term

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProgressLineRendersInPlace(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	line := NewProgressLine(&out, ProgressLineOptions{
		ForceTerminal: true,
		Throttle:      -1,
		Now: func() time.Time {
			return now
		},
	})
	line.Start("Scanning", 10)
	now = now.Add(2 * time.Second)
	line.Advance("internal/codeindex/indexer/scan.go")

	got := out.String()
	for _, want := range []string{"\r\033[K", "Scanning", "1/10", "10%", "elapsed 2s", "0.5/s", "internal/codeindex/indexer/scan.go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress output missing %q:\n%q", want, got)
		}
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("progress output should stay on one line:\n%q", got)
	}
}

func TestProgressLineFinishClearsPinnedLine(t *testing.T) {
	var out bytes.Buffer
	line := NewProgressLine(&out, ProgressLineOptions{ForceTerminal: true})
	line.Start("Scanning", 1)
	line.Advance("done")
	line.Finish()

	got := out.String()
	if !strings.Contains(got, "\r\033[K    Finished Scanning") {
		t.Fatalf("Finish should clear the active line and print completion, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("finished line should end with newline, got %q", got)
	}
}

func TestProgressLineWritesFinishedLineToConfiguredWriter(t *testing.T) {
	var pinned bytes.Buffer
	var finished bytes.Buffer
	line := NewProgressLine(&pinned, ProgressLineOptions{ForceTerminal: true, FinishedWriter: &finished})
	line.Start("Scanning", 1)
	line.Finish()

	if strings.Contains(pinned.String(), "Finished Scanning") {
		t.Fatalf("finished line should not be written to pinned writer:\n%q", pinned.String())
	}
	if !strings.Contains(finished.String(), "Finished Scanning") {
		t.Fatalf("finished line missing from configured writer:\n%q", finished.String())
	}
}

func TestProgressLineFinishForcesFinalState(t *testing.T) {
	var out bytes.Buffer
	line := NewProgressLine(&out, ProgressLineOptions{ForceTerminal: true, Throttle: time.Hour})
	line.Start("Scanning", 5)
	line.Finish()

	got := out.String()
	if !strings.Contains(got, "5/5") || !strings.Contains(got, "100%") {
		t.Fatalf("Finish should force final progress state before completion:\n%q", got)
	}
}

func TestProgressLineTruncatesLongDetailsToWidth(t *testing.T) {
	var out bytes.Buffer
	line := NewProgressLine(&out, ProgressLineOptions{ForceTerminal: true, Throttle: -1, Width: 64})
	line.Start("Scanning", 100)
	line.Advance("internal/codeindex/some/really/very/long/path/with/SymbolName.That.Would.Wrap.go")

	got := out.String()
	rendered := got[strings.LastIndex(got, "\r\033[K")+len("\r\033[K"):]
	if len([]rune(rendered)) > 64 {
		t.Fatalf("progress line exceeded configured width: len=%d\n%q", len([]rune(rendered)), rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("expected long detail to be truncated with ellipsis:\n%q", rendered)
	}
}

func TestProgressLineNonTerminalIsSilent(t *testing.T) {
	var out bytes.Buffer
	line := NewProgressLine(&out, ProgressLineOptions{})
	if line != nil {
		t.Fatal("expected nil progress line for non-terminal writer")
	}
	line.Start("Scanning", 1)
	line.Advance("done")
	line.Finish()
	if out.Len() != 0 {
		t.Fatalf("non-terminal progress wrote %q", out.String())
	}
}

func TestByteProgressWriterClearsOnCompletion(t *testing.T) {
	var out bytes.Buffer
	writer := NewByteProgressWriter(&out, 4, "Downloading")
	if _, err := writer.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("non-terminal byte progress should be silent, got %q", got)
	}

	out.Reset()
	lineWriter := NewProgressLine(&out, ProgressLineOptions{ForceTerminal: true, Throttle: -1})
	lineWriter.Start64("Downloading", 4)
	lineWriter.Add(4, "4 B")
	lineWriter.Finish()
	if !strings.Contains(out.String(), "Downloading") || !strings.Contains(out.String(), "Finished Downloading") {
		t.Fatalf("forced byte-style progress did not render and clear:\n%q", out.String())
	}
}

func TestByteProgressWriterReturnsDiscardForNonTerminal(t *testing.T) {
	var out bytes.Buffer
	if writer := NewByteProgressWriter(&out, 10, "Downloading"); writer != io.Discard {
		t.Fatalf("expected io.Discard for non-terminal writer, got %T", writer)
	}
}
