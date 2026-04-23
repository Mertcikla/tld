package cmdutil

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/workspace"
)

func WithHint(err error, hint string) error {
	return fmt.Errorf("%w\n  Hint: %s", err, hint)
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
		return WithHint(errors.New("api_key is empty"), "Run 'tld login' or set the TLD_API_KEY environment variable.")
	}
	return nil
}

func WorkspaceIDRequired(message string) error {
	return WithHint(errors.New(message), "Run 'tld login' to set your org-id automatically.")
}

func WithUnauthorizedHint(prefix string, err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnauthenticated {
		return WithHint(fmt.Errorf("%s: %w", prefix, err), "Your API key may have expired. Run 'tld login' to refresh it.")
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
