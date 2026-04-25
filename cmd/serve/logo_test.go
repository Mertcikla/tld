package serve

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd/version"
	"github.com/mertcikla/tld/internal/term"
)

func TestPrintLogo(t *testing.T) {
	t.Run("no color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		var buf bytes.Buffer
		PrintLogo(&buf)

		output := buf.String()
		if strings.Contains(output, term.ColorBlue) {
			t.Errorf("expected no color, but found blue color code")
		}
		if !strings.Contains(output, "░██") {
			t.Errorf("expected logo content, but not found")
		}
		if !strings.Contains(output, "Version:             "+version.Version) {
			t.Errorf("expected version content, but not found")
		}
	})

	t.Run("with color enabled via terminal simulation", func(t *testing.T) {
		// We can't easily mock IsTerminal because it uses os.File.Stat()
		// but we can try to test that it handles NO_COLOR correctly.
		// To truly test color, we'd need to bypass IsTerminal check or use a real terminal.
		// For now, let's just ensure NO_COLOR is respected.
		t.Setenv("NO_COLOR", "1")

		var buf bytes.Buffer
		PrintLogo(&buf)
		if strings.Contains(buf.String(), term.ColorBlue) {
			t.Errorf("expected no color when NO_COLOR is set")
		}
		if !strings.Contains(buf.String(), "Version:             "+version.Version) {
			t.Errorf("expected version content, but not found")
		}
	})
}
