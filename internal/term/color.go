package term

import (
	"io"
	"os"
)

const (
	ColorGreen     = "\033[32m"
	ColorBlue      = "\033[34m"
	ColorYellow    = "\033[33m"
	ColorRed       = "\033[31m"
	ColorUnderline = "\033[4m"
	ColorReset     = "\033[0m"

	// ColorBgSubtle is a low-contrast background used to zebra-stripe table
	// rows so long lines are easier to follow.
	ColorBgSubtle = "\033[48;5;236m"
)

// Stripe applies a subtle background to alternating rows when color output is
// enabled. index is the zero-based row index; the first row is left unstyled.
func Stripe(w io.Writer, index int, text string) string {
	if !IsColorEnabled(w) || index == 0 || index%2 != 0 {
		return text
	}
	return ColorBgSubtle + text + ColorReset
}

func IsTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		fi, err := f.Stat()
		return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
	}
	return false
}

func IsColorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return IsTerminal(w)
}

func Colorize(w io.Writer, color, text string) string {
	if !IsColorEnabled(w) {
		return text
	}
	return color + text + ColorReset
}
