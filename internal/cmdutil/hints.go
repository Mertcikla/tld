package cmdutil

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func WithHint(err error, hint string) error {
	return &CommandError{Cause: err, Code: "command_failed", Hint: hint}
}

func LoadWorkspace(dir string) (*workspace.Workspace, error) {
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, WithHint(fmt.Errorf("load workspace: %w", err), "Run 'tld init' to create a workspace in this directory.")
	}
	return ws, nil
}

func EnsureAPIKey(apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return WithCode(errors.New("api_key is empty"), "unauthenticated", "Run 'tld login' or set the TLD_API_KEY environment variable.")
	}
	return nil
}

func WorkspaceIDRequired(message string) error {
	return WithCode(errors.New(message), "workspace_required", "Run 'tld login' to set your org-id automatically.")
}

func WithUnauthorizedHint(prefix string, err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnauthenticated {
		return WithCode(fmt.Errorf("%s: %w", prefix, err), "unauthenticated", "Run 'tld login' to refresh your credentials.")
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
