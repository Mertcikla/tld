package term

import (
	"fmt"
	"io"
)

// ANSI color/style constants for richer output.
const (
	ColorCyan              = "\033[36m"
	ColorBold              = "\033[1m"
	ColorDim               = "\033[2m"
	StyleUnderlineGreenURL = ColorGreen + ColorUnderline
)

// DefaultLabelWidth is the shared left-column width for status-style labels.
const DefaultLabelWidth = 16

func Success(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "%s\n", msg)
}

func Successf(w io.Writer, format string, args ...any) {
	Success(w, fmt.Sprintf(format, args...))
}

func Info(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "%s\n", msg)
}

func Infof(w io.Writer, format string, args ...any) {
	Info(w, fmt.Sprintf(format, args...))
}

func Warn(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "%s \n", msg)
}

func Warnf(w io.Writer, format string, args ...any) {
	Warn(w, fmt.Sprintf(format, args...))
}

func Fail(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "%s \n", msg)
}

func Failf(w io.Writer, format string, args ...any) {
	Fail(w, fmt.Sprintf(format, args...))
}

// Label formats "  <key>:  <value>\n" with the key in cyan/bold when color is on.
// It uses a fixed-width left column (width chars, left-aligned).
func Label(w io.Writer, width int, key, value string) {
	keyFmt := fmt.Sprintf("%-*s", width, key+":")
	if IsColorEnabled(w) {
		keyFmt = ColorCyan + ColorBold + keyFmt + ColorReset
	}
	_, _ = fmt.Fprintf(w, "  %s %s\n", keyFmt, value)
}

// URL returns the url styled as an underlined green clickable link (when color is on).
func URL(w io.Writer, url string) string {
	if !IsColorEnabled(w) {
		return url
	}
	return StyleUnderlineGreenURL + url + ColorReset
}

// Path returns the path styled as blue text (when color is on).
func Path(w io.Writer, path string) string {
	return Colorize(w, ColorBlue, path)
}

// Badge returns a colored inline badge like "[watch]" used in log-style output.
func Badge(w io.Writer, color, text string) string {
	return Colorize(w, color, text)
}

// Dim returns the text styled as dim/grey (when color is on).
func Dim(w io.Writer, text string) string {
	return Colorize(w, ColorDim, text)
}

// Hint prints a dim indented hint line.
func Hint(w io.Writer, hint string) {
	_, _ = fmt.Fprintf(w, "  %s\n", Dim(w, hint))
}

// PrintLogo prints the tld ASCII logo and version line.
func PrintLogo(w io.Writer, version string) {
	logo := `
   ░██    ░██ ░███████
   ░██    ░██ ░██   ░██
░████████ ░██ ░██    ░██
   ░██    ░██ ░██    ░██
   ░██    ░██ ░██   ░██
    ░████ ░██ ░███████
`
	_, _ = fmt.Fprintln(w, logo)
	Label(w, DefaultLabelWidth, "Version", version)
}

// Separator prints a blank line.
func Separator(w io.Writer) {
	_, _ = fmt.Fprintln(w)
}
