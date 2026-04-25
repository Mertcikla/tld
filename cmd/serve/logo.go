package serve

import (
	"fmt"
	"io"

	"github.com/mertcikla/tld/cmd/version"
)

func PrintLogo(w io.Writer) {
	logo := `
   ░██    ░██ ░███████
   ░██    ░██ ░██   ░██
░████████ ░██ ░██    ░██
   ░██    ░██ ░██    ░██
   ░██    ░██ ░██   ░██
    ░████ ░██ ░███████
`
	_, _ = fmt.Fprintln(w, logo)
	_, _ = fmt.Fprintf(w, "Version:             %s\n", version.Version)

}
