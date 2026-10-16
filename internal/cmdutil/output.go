package cmdutil

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func WantsJSON(format string) bool {
	return strings.EqualFold(format, "json")
}

// WantsJSONFromCmd reports whether the root --format flag requests JSON. It is a
// convenience for commands that do not take a format pointer directly.
func WantsJSONFromCmd(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Root().PersistentFlags().Lookup("format")
	return flag != nil && WantsJSON(flag.Value.String())
}

// CompactFromCmd reports whether the root --compact flag is set.
func CompactFromCmd(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Root().PersistentFlags().Lookup("compact")
	return flag != nil && flag.Value.String() == "true"
}

func WriteJSON(w io.Writer, compact bool, payload JSONOutput) error {
	if payload.Status == "dry-run" {
		payload.Status, payload.DryRun = "ok", true
	}
	for _, item := range payload.Items {
		if item.Action == "dry-run" {
			payload.DryRun = true
		}
	}
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}

// WriteCommandError writes a JSON error response and returns the operation's
// failure so a successfully written response cannot make the command succeed.
func WriteCommandError(w io.Writer, compact bool, command string, err error) error {
	return WriteResult(w, compact, JSONOutput{Command: command}, err)
}

// WriteResult emits exactly one response, retaining diagnostics on failure.
func WriteResult(w io.Writer, compact bool, payload JSONOutput, err error) error {
	if err != nil {
		detail := recoveryError("tld "+strings.TrimPrefix(payload.Command, "tld "), err)
		payload.Status, payload.Error, payload.Errors = "error", detail, []string{detail.Error()}
		err = detail
	}
	if writeErr := WriteJSON(w, compact, payload); writeErr != nil {
		return errors.Join(err, writeErr)
	}
	if err != nil {
		return &reportedError{err}
	}
	return nil
}

func WriteMutation(w io.Writer, compact bool, command, action, ref string) error {
	resourceType := "element"
	if strings.Contains(command, "connector") || command == "connect" {
		resourceType = "connector"
	}
	if strings.HasPrefix(command, "view") {
		resourceType = "view"
	}
	return WriteResourceMutation(w, compact, command, action, ref, resourceType, 0)
}

func WriteResourceMutation(w io.Writer, compact bool, command, action, ref, resourceType string, id int32) error {
	return WriteJSON(w, compact, JSONOutput{
		Command: command,
		Status:  "ok",
		Items: []JSONItem{
			{
				Action:       action,
				Ref:          ref,
				ResourceType: resourceType,
				ID:           id,
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
