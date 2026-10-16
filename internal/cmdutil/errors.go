package cmdutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"connectrpc.com/connect"
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

func WithCode(err error, code, hint string) error {
	return &CommandError{Cause: err, Code: code, Hint: hint}
}

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
	var recovery interface {
		RecoveryCode() string
		RecoveryHint() string
	}
	if errors.As(err, &recovery) {
		return &CommandError{Cause: err, Code: recovery.RecoveryCode(), Message: message, Hint: recovery.RecoveryHint()}
	}
	code, hint := "command_failed", "Run '"+command+" --help' for usage and examples."
	var pathErr *os.PathError
	var rpcErr *connect.Error
	if errors.As(err, &pathErr) {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			code, hint = "path_not_found", "Supply an existing input path."
			if pathErr.Op == "open" && strings.Contains(message, "write output") {
				hint = "Create the output directory or choose a path in an existing directory."
			}
		case errors.Is(err, fs.ErrPermission):
			code, hint = "permission_denied", "Use a path you can access or fix its file permissions."
		default:
			code, hint = "filesystem_error", "Check the path and available disk space, then retry."
		}
		return &CommandError{Cause: err, Code: code, Message: message, Hint: hint}
	}
	if errors.As(err, &rpcErr) {
		switch rpcErr.Code() {
		case connect.CodeUnauthenticated:
			code, hint = "unauthenticated", "Run 'tld login' to refresh your credentials."
		case connect.CodePermissionDenied:
			code, hint = "permission_denied", "Select a workspace you can edit or request write access."
		case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
			code, hint = "target_unavailable", "Check the server URL and connection, then retry."
		case connect.CodeInvalidArgument:
			code, hint = "invalid_value", "Correct the field reported by the target, then retry."
		case connect.CodeFailedPrecondition:
			code, hint = "failed_precondition", "Refresh target state with 'tld pull', then retry the corrected operation."
		case connect.CodeUnimplemented:
			code, hint = "unsupported_operation", "Update the target server to a version supporting this operation."
		}
		return &CommandError{Cause: err, Code: code, Message: message, Hint: hint}
	}
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
	case strings.Contains(message, "invalid direction"):
		code, hint = "invalid_value", "Use forward, backward, both, or none for direction."
	case strings.Contains(message, "invalid style"):
		code, hint = "invalid_value", "Use bezier, straight, step, or smoothstep for style."
	}
	return &CommandError{Cause: err, Code: code, Message: message, Hint: hint}
}

// UnknownField supplies a nearby valid field when a spelling mistake is likely.
func UnknownField(resource, field string, fields []string) error {
	hint := "Run 'tld update " + resource + " --help' for accepted fields."
	best, distance := "", 3
	for _, candidate := range fields {
		row := make([]int, len(candidate)+1)
		for j := range row {
			row[j] = j
		}
		for i := range field {
			previous := row[0]
			row[0] = i + 1
			for j := range candidate {
				old, cost := row[j+1], 1
				if field[i] == candidate[j] {
					cost = 0
				}
				row[j+1] = min(row[j]+1, row[j+1]+1, previous+cost)
				previous = old
			}
		}
		if d := row[len(candidate)]; d < distance {
			best, distance = candidate, d
		}
	}
	if best != "" {
		hint = fmt.Sprintf("Use the %s field instead of %s.", best, field)
	}
	return WithCode(fmt.Errorf("unknown %s field %q; known fields: %s", resource, field, strings.Join(fields, ", ")), "unknown_field", hint)
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
