package mermaid

import (
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

func TestExtractMermaidCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{name: "raw", text: "flowchart LR\n  A --> B", want: "flowchart LR\n  A --> B", ok: true},
		{name: "architecture raw", text: "architecture-beta\n  service api(server)[API]", want: "architecture-beta\n  service api(server)[API]", ok: true},
		{name: "fenced", text: "```mermaid\nflowchart TB\n  A --> B\n```\n", want: "flowchart TB\n  A --> B", ok: true},
		{name: "wrong fence", text: "```mmd\nflowchart LR\n  A --> B\n```", ok: false},
		{name: "plain text", text: "not a diagram", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractMermaidCode(tt.text)
			if ok != tt.ok {
				t.Fatalf("ExtractMermaidCode() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("ExtractMermaidCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkdownBlocksAndSyncStatus(t *testing.T) {
	t.Parallel()

	code := "flowchart LR\n%% tld/v1 view=42\n  A[API] --> B[DB]\n"
	staleCode := "flowchart LR\n%% tld/v1 view=42\n  A[Old API] --> B[DB]\n"
	otherCode := "flowchart LR\n%% tld/v1 view=99\n  X --> Y\n"
	markdown := "# Notes\n\n```mermaid\n" + staleCode + "```\n\n```mermaid\n" + otherCode + "```\n"

	blocks := FindMarkdownBlocks(markdown)
	if len(blocks) != 2 {
		t.Fatalf("FindMarkdownBlocks() len = %d, want 2", len(blocks))
	}
	if blocks[0].ViewID == nil || *blocks[0].ViewID != 42 {
		t.Fatalf("first block ViewID = %v, want 42", blocks[0].ViewID)
	}
	if blocks[0].Preview != "flowchart LR" {
		t.Fatalf("first block Preview = %q, want flowchart LR", blocks[0].Preview)
	}
	if got := SyncStatus(markdown, 42, code); got != diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_STALE {
		t.Fatalf("SyncStatus stale = %v", got)
	}
	if got := BlockSyncStatus(blocks[1], ptrInt32(42), code); got != diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_OTHER_VIEW {
		t.Fatalf("BlockSyncStatus other view = %v", got)
	}

	updated := UpsertMarkdownBlock(markdown, 42, code)
	if got := SyncStatus(updated, 42, code); got != diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_SYNCED {
		t.Fatalf("SyncStatus updated = %v", got)
	}

	appended := UpsertMarkdownBlock("# Empty\n", 7, "flowchart LR\n%% tld/v1 view=7\n  A --> B")
	if got := SyncStatus(appended, 7, "flowchart LR\n%% tld/v1 view=7\n  A --> B"); got != diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_SYNCED {
		t.Fatalf("SyncStatus appended = %v", got)
	}
}

func ptrInt32(value int32) *int32 {
	return &value
}
