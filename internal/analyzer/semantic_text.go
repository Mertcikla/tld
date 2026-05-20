package analyzer

import (
	"regexp"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

var docMarkupRE = regexp.MustCompile(`(?m)^\s*@(?:param|return|returns|throws)\b.*$|^\s*:(?:param|returns?|rtype|raises)\b.*$|<[^>]+>`)

func declarationSignature(node *gotreesitter.Node, source []byte) string {
	text := strings.TrimSpace(nodeText(node, source))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "{"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line != "" {
				out = append(out, line)
			}
			break
		}
		out = append(out, line)
		if strings.HasSuffix(line, ":") || strings.HasSuffix(line, ";") || strings.HasSuffix(line, ")") {
			break
		}
	}
	return stripSignatureBoilerplate(strings.Join(out, " "))
}

func stripSignatureBoilerplate(signature string) string {
	signature = strings.TrimSpace(signature)
	signature = strings.NewReplacer("{", "", "}", "", ";", "").Replace(signature)
	for _, token := range []string{"public ", "private ", "protected ", "static ", "final ", "abstract ", "async "} {
		signature = strings.ReplaceAll(signature, token, "")
	}
	return strings.Join(strings.Fields(signature), " ")
}

func cleanDocText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		lines[i] = line
	}
	text = strings.Join(lines, "\n")
	text = docMarkupRE.ReplaceAllString(text, "")
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func pythonDocstring(node *gotreesitter.Node, source []byte) string {
	text := nodeText(node, source)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return ""
	}
	body := strings.Join(lines[1:], "\n")
	for _, quote := range []string{`"""`, `'''`} {
		start := strings.Index(body, quote)
		if start < 0 {
			continue
		}
		rest := body[start+len(quote):]
		end := strings.Index(rest, quote)
		if end < 0 {
			continue
		}
		return cleanDocText(rest[:end])
	}
	return ""
}

func leadingComment(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if node == nil {
		return ""
	}
	prev := prevNamedSibling(node)
	if prev == nil || nodeKind(prev, lang) != "comment" {
		return ""
	}
	if node.StartPoint().Row-prev.EndPoint().Row > 1 {
		return ""
	}
	return cleanComment(nodeText(prev, source))
}

func leadingLineComment(source []byte, line int) string {
	if line <= 1 {
		return ""
	}
	lines := strings.Split(string(source), "\n")
	idx := line - 2
	before := strings.TrimSpace(strings.Join(lines[:line-1], "\n"))
	if strings.HasSuffix(before, "*/") {
		if start := strings.LastIndex(before, "/*"); start >= 0 {
			return cleanComment(before[start:])
		}
	}
	var comments []string
	for idx >= 0 {
		text := strings.TrimSpace(lines[idx])
		if text == "" {
			break
		}
		if strings.HasPrefix(text, "//") || strings.HasPrefix(text, "///") || strings.HasPrefix(text, "//!") || strings.HasPrefix(text, "#") {
			comments = append([]string{cleanComment(text)}, comments...)
			idx--
			continue
		}
		break
	}
	return cleanDocText(strings.Join(comments, " "))
}

func cleanComment(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	text = strings.TrimPrefix(text, "///")
	text = strings.TrimPrefix(text, "//!")
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "#")
	return cleanDocText(text)
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
