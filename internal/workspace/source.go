package workspace

import (
	"strings"

	"github.com/mertcikla/tld/v2/internal/sourcelink"
)

// SourceFile returns the file path and optional symbol to verify for this
// element. It resolves anchors embedded in FilePath (e.g. "api.go#L10" or
// "api.go#function:Handle"), falling back to the legacy Symbol field. Symbol is
// empty when the element has only a file- or line-level link.
func (e *Element) SourceFile() (string, string) {
	if e == nil {
		return "", ""
	}
	parsed := sourcelink.Parse(e.FilePath)
	symbol := strings.TrimSpace(e.Symbol)
	if symbol == "" && parsed.Anchor.Kind == sourcelink.AnchorSymbol {
		symbol = parsed.Anchor.Symbol
	}
	return parsed.BasePath, symbol
}
