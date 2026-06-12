package mermaid

import (
	"regexp"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

type MarkdownBlock struct {
	Index          int
	Start          int
	End            int
	CodeStart      int
	CodeEnd        int
	LineStart      int
	LineEnd        int
	Fence          string
	Code           string
	ViewID         *int32
	HasTldMetadata bool
	Preview        string
}

var (
	openingFencePattern     = regexp.MustCompile("^[ \\t]*(`{3,}|~{3,})[ \\t]*([A-Za-z0-9_-]+)?(?:[ \\t].*)?$")
	tldMarkerPattern        = regexp.MustCompile(`^%%[ \t]+tld/v1(?:[ \t]+(.*))?$`)
	tldMarkerViewPattern    = regexp.MustCompile(`(?:^|\s)(?:view|viewId)=([0-9]+)`)
	supportedMermaidStartRe = regexp.MustCompile(`(?i)^(?:flowchart|graph|sequenceDiagram|classDiagram|erDiagram|stateDiagram(?:-v2)?|requirementDiagram|sankey-beta|pie|gitGraph|quadrantChart|mindmap|journey|gantt|timeline|xychart-beta|architecture-beta|C4Context|C4Container|C4Component|C4Dynamic|C4Deployment)\b`)
	mermaidMarkdownFenceRe  = regexp.MustCompile("(?is)^```mermaid[ \\t]*\\r?\\n([\\s\\S]*?)```$")
)

func ExtractMermaidCode(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	if match := mermaidMarkdownFenceRe.FindStringSubmatch(trimmed); match != nil && strings.TrimSpace(match[1]) != "" {
		return strings.TrimSpace(match[1]), true
	}
	if isSupportedMermaidStart(trimmed) {
		return trimmed, true
	}
	return "", false
}

func FindMarkdownBlocks(markdown string) []MarkdownBlock {
	lines := splitLinesKeepTerminator(markdown)
	blocks := []MarkdownBlock{}
	offset := 0
	lineNumber := 1

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if line == "" {
			break
		}
		fence := openingFence(line)
		if fence == "" {
			offset += len(line)
			lineNumber++
			continue
		}

		start := offset
		codeStart := offset + len(line)
		startLine := lineNumber
		offset += len(line)
		lineNumber++
		codeEnd := len(markdown)
		end := len(markdown)
		endLine := lineNumber

		for index++; index < len(lines); index++ {
			nextLine := lines[index]
			if nextLine == "" {
				break
			}
			if isClosingFence(nextLine, fence) {
				codeEnd = offset
				end = offset + len(nextLine)
				endLine = lineNumber
				offset = end
				lineNumber++
				break
			}
			offset += len(nextLine)
			lineNumber++
			endLine = lineNumber
		}

		code := strings.TrimSuffix(strings.TrimSuffix(markdown[codeStart:codeEnd], "\n"), "\r")
		blocks = append(blocks, MarkdownBlock{
			Index:          len(blocks),
			Start:          start,
			End:            end,
			CodeStart:      codeStart,
			CodeEnd:        codeEnd,
			LineStart:      startLine,
			LineEnd:        endLine,
			Fence:          fence,
			Code:           code,
			ViewID:         ExtractTldViewID(code),
			HasTldMetadata: HasTldMetadata(code),
			Preview:        Preview(code),
		})
	}

	return blocks
}

func ExtractTldViewID(code string) *int32 {
	for _, line := range strings.Split(code, "\n") {
		match := tldMarkerPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		if len(match) < 2 {
			return nil
		}
		raw := tldMarkerViewPattern.FindStringSubmatch(match[1])
		if raw == nil {
			return nil
		}
		id, err := strconv.ParseInt(raw[1], 10, 32)
		if err != nil || id <= 0 {
			return nil
		}
		value := int32(id)
		return &value
	}
	return nil
}

func HasTldMetadata(code string) bool {
	for _, line := range strings.Split(code, "\n") {
		if tldMarkerPattern.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

func Preview(code string) string {
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		if len(trimmed) > 120 {
			return trimmed[:117] + "..."
		}
		return trimmed
	}
	return ""
}

func FindBlockForView(markdown string, viewID int32) *MarkdownBlock {
	for _, block := range FindMarkdownBlocks(markdown) {
		if block.ViewID != nil && *block.ViewID == viewID {
			blockCopy := block
			return &blockCopy
		}
	}
	return nil
}

func MermaidBlock(code string) string {
	return "```mermaid\n" + strings.TrimSpace(strings.ReplaceAll(code, "\r\n", "\n")) + "\n```\n"
}

func UpsertMarkdownBlock(markdown string, viewID int32, code string) string {
	block := FindBlockForView(markdown, viewID)
	nextBlock := MermaidBlock(code)
	if block != nil {
		return markdown[:block.Start] + nextBlock + markdown[block.End:]
	}
	trimmedEnd := strings.TrimRight(markdown, " \t\r\n")
	if trimmedEnd == "" {
		return nextBlock
	}
	return trimmedEnd + "\n\n" + nextBlock
}

func SyncStatus(markdown string, viewID int32, code string) diagv1.MermaidMarkdownSyncStatus {
	block := FindBlockForView(markdown, viewID)
	if block == nil {
		return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_MISSING
	}
	if CodeEquals(block.Code, code) {
		return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_SYNCED
	}
	return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_STALE
}

func BlockSyncStatus(block MarkdownBlock, currentViewID *int32, currentCode string) diagv1.MermaidMarkdownSyncStatus {
	if block.ViewID == nil {
		return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_UNLINKED
	}
	if currentViewID == nil || *block.ViewID != *currentViewID {
		return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_OTHER_VIEW
	}
	if currentCode != "" && CodeEquals(block.Code, currentCode) {
		return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_SYNCED
	}
	return diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_STALE
}

func CodeEquals(left, right string) bool {
	return normalizeCode(left) == normalizeCode(right)
}

func normalizeCode(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}

func StripCommentLines(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "%%") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isSupportedMermaidStart(code string) bool {
	lines := strings.Split(strings.ReplaceAll(code, "\r\n", "\n"), "\n")
	start := firstMermaidBodyLineIndex(lines)
	return start >= 0 && supportedMermaidStartRe.MatchString(strings.TrimSpace(lines[start]))
}

func firstMermaidBodyLineIndex(lines []string) int {
	index := 0
	for index < len(lines) {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			index++
			continue
		}
		if line == "---" {
			index++
			for index < len(lines) && strings.TrimSpace(lines[index]) != "---" {
				index++
			}
			if index < len(lines) {
				index++
			}
			continue
		}
		if strings.HasPrefix(line, "%%") {
			index++
			continue
		}
		return index
	}
	return -1
}

func openingFence(line string) string {
	match := openingFencePattern.FindStringSubmatch(lineWithoutTerminator(line))
	if match == nil {
		return ""
	}
	if strings.ToLower(match[2]) != "mermaid" {
		return ""
	}
	return match[1]
}

func isClosingFence(line, fence string) bool {
	trimmed := strings.TrimSpace(lineWithoutTerminator(line))
	return trimmed == fence || strings.HasPrefix(trimmed, fence)
}

func lineWithoutTerminator(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

func splitLinesKeepTerminator(value string) []string {
	if value == "" {
		return []string{""}
	}
	lines := []string{}
	start := 0
	for index := 0; index < len(value); index++ {
		if value[index] != '\n' {
			continue
		}
		lines = append(lines, value[start:index+1])
		start = index + 1
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	} else {
		lines = append(lines, "")
	}
	return lines
}
