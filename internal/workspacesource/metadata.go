package workspacesource

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type metadataEntry struct {
	key     string
	value   string
	encoded bool
}

var metadataKeyPattern = regexp.MustCompile(`(^|\s+)([A-Za-z][A-Za-z0-9_]*)=`)

func escapeMetadataValue(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"=", `\=`,
		",", `\,`,
		"|", `\|`,
		":", `\:`,
	)
	return replacer.Replace(value)
}

func unescapeMetadataValue(value string) (string, bool) {
	var out strings.Builder
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char != '\\' {
			out.WriteByte(char)
			continue
		}
		index++
		if index >= len(value) {
			return "", false
		}
		switch value[index] {
		case '\\', '=', ',', '|', ':':
			out.WriteByte(value[index])
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		default:
			return "", false
		}
	}
	return out.String(), true
}

func metadataComment(kind string, entries []metadataEntry) string {
	parts := []string{"%% " + kind}
	for _, entry := range entries {
		if strings.TrimSpace(entry.value) == "" && entry.key != "x" && entry.key != "y" {
			continue
		}
		value := entry.value
		if !entry.encoded {
			value = escapeMetadataValue(value)
		}
		parts = append(parts, entry.key+"="+value)
	}
	return strings.Join(parts, " ")
}

func parseMetadataPairs(text string) map[string]string {
	matches := metadataKeyPattern.FindAllStringSubmatchIndex(text, -1)
	pairs := make(map[string]string, len(matches))
	for index, match := range matches {
		key := text[match[4]:match[5]]
		valueStart := match[5] + 1
		valueEnd := len(text)
		if index+1 < len(matches) {
			valueEnd = matches[index+1][0]
		}
		pairs[key] = strings.TrimRight(text[valueStart:valueEnd], " \t")
	}
	return pairs
}

func pairString(pairs map[string]string, key string) string {
	raw, ok := pairs[key]
	if !ok {
		return ""
	}
	value, ok := unescapeMetadataValue(raw)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func pairBool(pairs map[string]string, key string) *bool {
	raw, ok := pairs[key]
	if !ok {
		return nil
	}
	value, ok := unescapeMetadataValue(raw)
	if !ok {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true":
		v := true
		return &v
	case "0", "false":
		v := false
		return &v
	default:
		return nil
	}
}

func pairFloat(pairs map[string]string, key string) (float64, bool) {
	raw, ok := pairs[key]
	if !ok {
		return 0, false
	}
	value, ok := unescapeMetadataValue(raw)
	if !ok {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func encodeStringList(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			encoded = append(encoded, escapeMetadataValue(trimmed))
		}
	}
	return strings.Join(encoded, ",")
}

func decodeStringList(value string) []string {
	if value == "" {
		return nil
	}
	parts := splitEscaped(value, ',')
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		decoded, ok := unescapeMetadataValue(part)
		if !ok {
			return nil
		}
		if trimmed := strings.TrimSpace(decoded); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func pairStringList(pairs map[string]string, key string) []string {
	raw, ok := pairs[key]
	if !ok {
		return nil
	}
	return decodeStringList(raw)
}

func splitEscaped(value string, separator byte) []string {
	var out []string
	var current strings.Builder
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '\\' && index+1 < len(value) {
			current.WriteByte(char)
			index++
			current.WriteByte(value[index])
			continue
		}
		if char == separator {
			out = append(out, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	out = append(out, current.String())
	return out
}

func compactNumber(value float64) string {
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(math.Round(value*1000)/1000, 'f', -1, 64)
}

func metadataRequired(pairs map[string]string, key, context string) (string, error) {
	value := pairString(pairs, key)
	if value == "" {
		return "", fmt.Errorf("%s metadata missing %s", context, key)
	}
	return value, nil
}
