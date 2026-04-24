package login

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewLoginCmd(_ *string) *cobra.Command {
	var serverURL string
	var noBrowser bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate the CLI with a tlDiagram server",
		Long: `Opens a browser window to log in to the tlDiagram server and
authorise this CLI. The resulting API key is written to .tld.yaml.

If a browser cannot be opened, you can navigate to the URL manually or
enter the displayed code at <server>/app/device.`,
		RunE: func(cmd *cobra.Command, _ []string) error {

			envServerURL := os.Getenv("TLD_SERVER_URL")
			if envServerURL != "" {
				serverURL = envServerURL
			} else {
				serverURL = "https://tldiagram.com"
			}
			serverURL = strings.TrimRight(serverURL, "/")

			// Step 1: request a device code.
			auth, err := deviceAuthorize(cmd.Context(), serverURL)
			if err != nil {
				return fmt.Errorf("request device code: %w", err)
			}

			interval := int(auth.Interval)
			if interval <= 0 {
				interval = 5
			}

			// Step 2: inform the user.
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nOpen the following URL to log in:\n\n  %s\n\n", auth.VerificationUriComplete)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Or navigate to %s and enter the code:\n\n  %s\n\n", auth.VerificationUri, auth.UserCode)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authorisation… (press Ctrl+C to cancel)")

			// Step 3: optionally open the browser.
			if !noBrowser {
				_ = cmdutil.OpenBrowser(auth.VerificationUriComplete)
			}

			// Step 4: poll until approved, denied, or expired.
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(auth.ExpiresIn)*time.Second)
			defer cancel()

			apiKey, workspaceID, err := pollDeviceToken(ctx, serverURL, auth.DeviceCode, time.Duration(interval)*time.Second)
			if err != nil {
				return err
			}

			// Step 5: write tld.yaml.
			if err := writeConfig(serverURL, apiKey, workspaceID); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			cfgPath, _ := workspace.ConfigPath()
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nAuthorised! Config written to %s\n", cfgPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "server URL (default: $TLD_SERVER_URL or https://tldiagram.com)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")

	return cmd
}

func deviceAuthorize(ctx context.Context, serverURL string) (*diagv1.DeviceAuthorizeResponse, error) {
	c := client.NewDeviceClient(serverURL, false)
	req := connect.NewRequest(&diagv1.DeviceAuthorizeRequest{
		ClientName: "tld CLI",
	})
	res, err := c.Authorize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authorize device: %w", err)
	}
	return res.Msg, nil
}

func pollDeviceToken(ctx context.Context, serverURL, deviceCode string, interval time.Duration) (apiKey, workspaceID string, err error) {
	for {
		select {
		case <-ctx.Done():
			return "", "", fmt.Errorf("timed out waiting for authorisation")
		case <-time.After(interval):
		}

		tok, pollErr := deviceToken(ctx, serverURL, deviceCode)
		if pollErr != nil {
			// Transient network error - keep polling.
			continue
		}

		switch tok.Error {
		case "":
			// Success.
			return tok.ApiKey, tok.OrgId, nil
		case "authorization_pending":
			// Keep waiting.
			continue
		case "access_denied":
			return "", "", fmt.Errorf("authorisation denied by user")
		case "expired_token":
			return "", "", fmt.Errorf("device code expired - run 'tld login' again")
		default:
			return "", "", fmt.Errorf("unexpected error from server: %s", tok.Error)
		}
	}
}

func deviceToken(ctx context.Context, serverURL, deviceCode string) (*diagv1.DevicePollTokenResponse, error) {
	c := client.NewDeviceClient(serverURL, false)
	req := connect.NewRequest(&diagv1.DevicePollTokenRequest{
		DeviceCode: deviceCode,
	})
	res, err := c.PollToken(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("poll device token: %w", err)
	}
	return res.Msg, nil
}

// writeConfig merges the auth credentials into the global tld.yaml,
// preserving any existing keys not related to auth.
func writeConfig(serverURL, apiKey, workspaceID string) error {
	cfgPath, err := workspace.ConfigPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Read existing config if present.
	var node yaml.Node
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := yaml.Unmarshal(data, &node); err != nil || node.Kind == 0 {
			node = yaml.Node{}
		}
	}

	// Ensure we have a mapping node.
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
			{Kind: yaml.MappingNode},
		}}
	}
	root := node.Content[0]

	setYAMLKey(root, "server_url", serverURL)
	setYAMLKey(root, "api_key", apiKey)
	setYAMLKey(root, "org_id", workspaceID)

	out, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, out, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// setYAMLKey sets or updates a string key in a YAML mapping node.
func setYAMLKey(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Value = value
			return
		}
	}
	// Key not found - append.
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}
