// Package sourcelink parses and formats the source-link anchors embedded in an
// element's file_path, e.g. "grpc/server.go#L18" or
// "grpc/server.go#function_declaration:Listen". It mirrors the encoding used by
// the frontend (frontend/src/utils/sourceLinks.ts) so CLI-authored links render
// identically in the UI.
package sourcelink

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// AnchorKind identifies the type of a source anchor.
type AnchorKind int

const (
	AnchorNone AnchorKind = iota
	AnchorLine
	AnchorSymbol
)

// Anchor is the optional fragment appended to a file path.
type Anchor struct {
	Kind      AnchorKind
	NodeType  string
	Symbol    string
	StartLine int
	EndLine   int
}

// Parsed is a source link split into its file path and optional anchor.
type Parsed struct {
	BasePath string
	Anchor   Anchor
}

var (
	lineAnchorPattern   = regexp.MustCompile(`^L([1-9]\d*)(?:-L?([1-9]\d*))?$`)
	symbolAnchorPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):(.+)$`)
)

// BasePath returns the file path portion of a source link, dropping any anchor.
func BasePath(link string) string {
	if idx := strings.IndexByte(link, '#'); idx >= 0 {
		return link[:idx]
	}
	return link
}

// Parse splits a source link into its base path and anchor.
func Parse(link string) Parsed {
	idx := strings.IndexByte(link, '#')
	if idx < 0 {
		return Parsed{BasePath: link, Anchor: Anchor{Kind: AnchorNone}}
	}
	return Parsed{
		BasePath: link[:idx],
		Anchor:   parseAnchor(link[idx+1:]),
	}
}

func parseAnchor(raw string) Anchor {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Anchor{Kind: AnchorNone}
	}
	if match := lineAnchorPattern.FindStringSubmatch(text); match != nil {
		start, err := strconv.Atoi(match[1])
		if err == nil {
			end := start
			if match[2] != "" {
				if parsed, err := strconv.Atoi(match[2]); err == nil && parsed >= start {
					end = parsed
				}
			}
			return Anchor{Kind: AnchorLine, StartLine: start, EndLine: end}
		}
	}
	if match := symbolAnchorPattern.FindStringSubmatch(text); match != nil {
		nodeType := strings.TrimSpace(match[1])
		symbol := decodeURIComponent(strings.TrimSpace(match[2]))
		if nodeType != "" && symbol != "" {
			return Anchor{Kind: AnchorSymbol, NodeType: nodeType, Symbol: symbol}
		}
	}
	var legacy struct {
		StartLine *float64 `json:"startLine"`
		EndLine   *float64 `json:"endLine"`
		Name      string   `json:"name"`
		Type      string   `json:"type"`
	}
	if err := json.Unmarshal([]byte(text), &legacy); err == nil {
		if legacy.StartLine != nil && *legacy.StartLine > 0 {
			start := int(*legacy.StartLine)
			end := start
			if legacy.EndLine != nil && *legacy.EndLine >= float64(start) {
				end = int(*legacy.EndLine)
			}
			return Anchor{Kind: AnchorLine, StartLine: start, EndLine: end}
		}
		if legacy.Name != "" && legacy.Type != "" {
			return Anchor{Kind: AnchorSymbol, NodeType: legacy.Type, Symbol: legacy.Name}
		}
	}
	return Anchor{Kind: AnchorNone}
}

// FormatLine returns filePath with a line anchor, replacing any existing anchor.
func FormatLine(filePath string, line int) string {
	base := strings.TrimSpace(BasePath(filePath))
	if base == "" || line <= 0 {
		return base
	}
	return base + "#L" + strconv.Itoa(line)
}

// FormatSymbol returns filePath with a symbol anchor, replacing any existing
// anchor.
func FormatSymbol(filePath, nodeType, symbol string) string {
	base := strings.TrimSpace(BasePath(filePath))
	nodeType = strings.TrimSpace(nodeType)
	symbol = strings.TrimSpace(symbol)
	if base == "" || nodeType == "" || symbol == "" {
		return base
	}
	return base + "#" + nodeType + ":" + encodeURIComponent(symbol)
}

// Label returns a short display label for an anchor.
func Label(anchor Anchor) string {
	switch anchor.Kind {
	case AnchorLine:
		return "L" + strconv.Itoa(anchor.StartLine)
	case AnchorSymbol:
		return anchor.Symbol
	default:
		return ""
	}
}

// encodeURIComponent percent-encodes bytes the way JavaScript's
// encodeURIComponent does.
func encodeURIComponent(value string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// decodeURIComponent reverses percent-encoding, leaving malformed sequences
// untouched (matching the safe fallback used by the frontend).
func decodeURIComponent(value string) string {
	if !strings.ContainsRune(value, '%') {
		return value
	}
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			hi := fromHex(value[i+1])
			lo := fromHex(value[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func fromHex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}
