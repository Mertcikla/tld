package serve

import (
	"io"

	"github.com/mertcikla/tld/cmd/version"
	"github.com/mertcikla/tld/internal/term"
)

func PrintLogo(w io.Writer) {
	term.PrintLogo(w, version.Version)
}
