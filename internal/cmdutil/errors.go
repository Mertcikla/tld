package cmdutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// CommandError keeps recovery instructions separate from the underlying error.
// Applied is omitted unless the command knows that the target was changed.
type CommandError struct {
	Cause   error  `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Applied bool   `json:"applied,omitempty"`
}

func (e *CommandError) Error() string { return fmt.Sprintf("%v\n  Hint: %s", e.Cause, e.Hint) }
func (e *CommandError) Unwrap() error { return e.Cause }

type reportedError struct{ error }

func (e *reportedError) Unwrap() error { return e.error }

func ErrorReported(err error) bool {
	var reported *reportedError
	return errors.As(err, &reported)
}

// CacheError distinguishes a committed target write from a failed cache refresh.
func CacheError(err error) error {
	return &CommandError{Cause: fmt.Errorf("target updated, but updating the YAML cache failed: %w", err),
		Code: "cache_update_failed", Applied: true, Hint: "Run 'tld pull' to refresh the workspace before continuing."}
}

func recoveryError(command string, err error) *CommandError {
	var detail *CommandError
	if errors.As(err, &detail) {
		copy := *detail
		copy.Message = detail.Cause.Error()
		return &copy
	}
	message := err.Error()
	code, hint := "command_failed", "Run '"+command+" --help' for usage and examples."
	switch {
	case strings.Contains(message, "not found") && strings.Contains(message, "connector"):
		code, hint = "not_found", "Run 'tld list connectors' and use a listed key or ID."
	case strings.Contains(message, "not found") && (strings.Contains(message, "element") || strings.Contains(message, "parent")):
		code, hint = "not_found", "Run 'tld list elements' and use a listed ref or ID; create missing elements with 'tld add'."
	case strings.Contains(message, "not found") && strings.Contains(message, "view"):
		code, hint = "not_found", "Run 'tld list views --tree' and use a listed view ref."
	case strings.Contains(message, "arg(s)") || strings.Contains(message, "flag") || strings.HasPrefix(message, "unknown command"):
		code = "invalid_arguments"
	case strings.Contains(message, "ambiguous"):
		code, hint = "ambiguous_selector", "Use an exact ref or numeric ID from 'tld list'."
	}
	return &CommandError{Cause: err, Code: code, Message: message, Hint: hint}
}

// Execute handles errors at the command boundary, including Cobra parsing
// failures that never reach RunE. Commands that already emitted JSON are kept.
func Execute(root *cobra.Command, args []string) error {
	root.SetArgs(args)
	command, err := root.ExecuteC()
	if err == nil || ErrorReported(err) {
		return err
	}
	if command == nil {
		command = root
	}
	detail := recoveryError(command.CommandPath(), err)
	if detail.Code == "invalid_arguments" {
		detail.Hint = "Usage: " + command.UseLine() + ". Run '" + command.CommandPath() + " --help' for examples."
	}
	jsonOutput := WantsJSONFromCmd(command)
	// Unknown commands/flags can stop Cobra before it parses the output flag.
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--format=json" || (arg == "--format" && i+1 < len(args) && args[i+1] == "json") {
			jsonOutput = true
		}
	}
	if jsonOutput {
		return WriteCommandError(root.OutOrStdout(), CompactFromCmd(command), strings.TrimPrefix(command.CommandPath(), root.Name()+" "), detail)
	}
	return detail
}
