package term

import (
	"bytes"
	"os"
	"testing"
)

func TestColorize(t *testing.T) {
	t.Run("no color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		var buf bytes.Buffer
		text := "hello"
		result := Colorize(&buf, ColorBlue, text)
		if result != text {
			t.Errorf("expected %q, got %q", text, result)
		}
	})

	// Testing with color enabled is hard because IsTerminal depends on the writer being an *os.File
}

func TestConstants(t *testing.T) {
	if ColorBlue != "\033[34m" {
		t.Errorf("expected ColorBlue to be \"\\033[34m\", got %q", ColorBlue)
	}
	if ColorUnderline != "\033[4m" {
		t.Errorf("expected ColorUnderline to be \"\\033[4m\", got %q", ColorUnderline)
	}
}

func TestStripe(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	if got := Stripe(devNull, 0, "header"); got != "header" {
		t.Errorf("row 0 should be unstyled, got %q", got)
	}
	if got := Stripe(devNull, 1, "row1"); got != "row1" {
		t.Errorf("odd row should be unstyled, got %q", got)
	}
	want := ColorBgSubtle + "row2" + ColorReset
	if got := Stripe(devNull, 2, "row2"); got != want {
		t.Errorf("even row should be striped: got %q want %q", got, want)
	}
}
