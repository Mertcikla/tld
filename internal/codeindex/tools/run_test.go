package tools

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTailOutput(t *testing.T) {
	var tail tailOutput
	chunks := [][]byte{bytes.Repeat([]byte("a"), outputLimit-10), bytes.Repeat([]byte("b"), 30), bytes.Repeat([]byte("c"), outputLimit*2), []byte("last diagnostic")}
	var full []byte
	for _, chunk := range chunks {
		n, err := tail.Write(chunk)
		if n != len(chunk) || err != nil {
			t.Fatal(n, err)
		}
		full = append(full, chunk...)
	}
	want := append([]byte(truncatedOutput), full[len(full)-outputLimit:]...)
	if !bytes.Equal(tail.bytes(), want) {
		t.Fatal("diagnostic tail differs")
	}
}

func TestRunHelper(t *testing.T) {
	// Activated only in an explicitly launched subprocess.
	for _, arg := range os.Args {
		switch arg {
		case "tool-noise":
			_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), outputLimit*4))
			_, _ = os.Stderr.Write([]byte("final error"))
			os.Exit(1)
		case "tool-wait":
			for {
				time.Sleep(time.Second)
			}
		}
	}
}

func TestRunBoundsOutput(t *testing.T) {
	out, err := Run(context.Background(), os.Args[0], []string{"-test.run=^TestRunHelper$", "--", "tool-noise"}, "")
	if err == nil || len(out) != outputLimit+len(truncatedOutput) || !strings.HasSuffix(string(out), "final error") {
		t.Fatalf("len=%d err=%v", len(out), err)
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, os.Args[0], []string{"-test.run=^TestRunHelper$", "--", "tool-wait"}, "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost context error: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("cancellation did not bound wait")
	}
}
