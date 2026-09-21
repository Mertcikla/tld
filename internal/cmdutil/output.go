package cmdutil

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func WantsJSON(format string) bool {
	return strings.EqualFold(format, "json")
}

func WriteJSON(w io.Writer, compact bool, payload JSONOutput) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}

func WriteCommandError(w io.Writer, compact bool, command string, err error) error {
	return WriteJSON(w, compact, JSONOutput{
		Command: command,
		Status:  "error",
		Errors:  []string{err.Error()},
	})
}

func WriteMutation(w io.Writer, compact bool, command, action, ref string) error {
	return WriteJSON(w, compact, JSONOutput{
		Command: command,
		Status:  "ok",
		Items: []JSONItem{
			{
				Action: action,
				Ref:    ref,
			},
		},
	})
}

func IncludedElementRefs(ws *workspace.Workspace) map[string]bool {
	included := make(map[string]bool, len(ws.Elements))
	for ref, element := range ws.Elements {
		if ws.ActiveRepo != "" && element.Owner != "" && element.Owner != ws.ActiveRepo {
			continue
		}
		included[ref] = true
	}
	return included
}
