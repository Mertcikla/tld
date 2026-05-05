package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and update the global tld configuration",
		Long:  "Inspect and update the global tld.yaml configuration file.",
	}
	cmd.AddCommand(newPathCmd(), newListCmd(), newGetCmd(), newSetCmd(), newValidateCmd())
	return cmd
}

func newPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the active global config path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := workspace.ConfigPath()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	var showSecrets bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List effective global config values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := workspace.LoadGlobalConfigState()
			if err != nil {
				return err
			}
			values := redactConfigValues(state.Values, showSecrets)
			if wantsJSON(cmd) {
				return writeJSON(cmd, values)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "KEY\tVALUE\tSOURCE\tENV\tDESCRIPTION")
			for _, value := range values {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", value.Key, value.Value, value.Source, value.Env, value.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "show secret values instead of redacting them")
	return cmd
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one effective global config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.ToLower(strings.TrimSpace(args[0]))
			if _, ok := workspace.ConfigDefinitionForKey(key); !ok {
				return fmt.Errorf("unknown global config key %q", args[0])
			}
			state, err := workspace.LoadGlobalConfigState()
			if err != nil {
				return err
			}
			for _, value := range state.Values {
				if value.Key == key {
					if wantsJSON(cmd) {
						return writeJSON(cmd, value)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), value.Value)
					return nil
				}
			}
			return fmt.Errorf("unknown global config key %q", args[0])
		},
	}
}

func newSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one global config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := workspace.SetGlobalConfigValue(args[0], args[1]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Set %s\n", strings.ToLower(strings.TrimSpace(args[0])))
			return nil
		},
	}
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the global config file and env overrides",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := workspace.LoadGlobalConfigState()
			if err != nil {
				return err
			}
			issues := workspace.ValidateGlobalConfig(state.Config)
			if wantsJSON(cmd) {
				payload := struct {
					OK     bool                              `json:"ok"`
					Issues []workspace.ConfigValidationError `json:"issues"`
				}{OK: len(issues) == 0, Issues: []workspace.ConfigValidationError(issues)}
				if err := writeJSON(cmd, payload); err != nil {
					return err
				}
			}
			if len(issues) > 0 {
				return issues
			}
			if !wantsJSON(cmd) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Global config valid.")
			}
			return nil
		},
	}
}

func redactConfigValues(values []workspace.ConfigValue, showSecrets bool) []workspace.ConfigValue {
	out := append([]workspace.ConfigValue(nil), values...)
	if showSecrets {
		return out
	}
	for i := range out {
		if out[i].Secret && out[i].Value != "" {
			out[i].Value = "********"
		}
	}
	return out
}

func wantsJSON(cmd *cobra.Command) bool {
	flag := cmd.Root().PersistentFlags().Lookup("format")
	return flag != nil && flag.Value.String() == "json"
}

func compactJSON(cmd *cobra.Command) bool {
	flag := cmd.Root().PersistentFlags().Lookup("compact")
	return flag != nil && flag.Value.String() == "true"
}

func writeJSON(cmd *cobra.Command, payload any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	if !compactJSON(cmd) {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}
