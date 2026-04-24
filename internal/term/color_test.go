package term

import (
	"bytes"
	"os"
	"testing"
)

func TestColorize(t *testing.T) {
	t.Run("no color", func(t *testing.T) {
		os.Setenv("NO_COLOR", "1")
		defer os.Unsetenv("NO_COLOR")

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
}
