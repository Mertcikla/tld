package completion

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// InstallWizard runs an interactive prompt to install the completion script.
func InstallWizard(cmd *cobra.Command) error {
	shell := filepath.Base(os.Getenv("SHELL"))
	if shell == "" {
		shell = "zsh" // default fallback
	}

	fmt.Printf("Detected shell: %s\n", shell)
	fmt.Printf("Do you want to install tld autocomplete for %s? [Y/n] ", shell)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "" && response != "y" && response != "yes" {
		fmt.Println("Installation cancelled.")
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	var rcFile string
	var installCmd string
	var writeDirect bool

	switch shell {
	case "zsh":
		rcFile = filepath.Join(homeDir, ".zshrc")
		installCmd = "\n# tld completion\nif command -v tld >/dev/null 2>&1; then eval \"$(tld completion zsh)\"; fi\n"
	case "bash":
		rcFile = filepath.Join(homeDir, ".bashrc")
		installCmd = "\n# tld completion\nif command -v tld >/dev/null 2>&1; then eval \"$(tld completion bash)\"; fi\n"
	case "fish":
		fishDir := filepath.Join(homeDir, ".config", "fish", "completions")
		if err := os.MkdirAll(fishDir, 0755); err != nil {
			return fmt.Errorf("failed to create fish completions directory: %w", err)
		}
		rcFile = filepath.Join(fishDir, "tld.fish")
		writeDirect = true
	default:
		return fmt.Errorf("unsupported shell: %s. Please install manually.", shell)
	}

	if writeDirect && shell == "fish" {
		f, err := os.OpenFile(rcFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", rcFile, err)
		}
		defer f.Close()
		if err := cmd.Root().GenFishCompletion(f, true); err != nil {
			return fmt.Errorf("failed to generate fish completion: %w", err)
		}
		fmt.Printf("Successfully installed completion to %s\n", rcFile)
		fmt.Println("Please restart your shell for the changes to take effect.")
		return nil
	}

	// Append to rc file
	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", rcFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString(installCmd); err != nil {
		return fmt.Errorf("failed to write to %s: %w", rcFile, err)
	}

	fmt.Printf("Successfully appended completion command to %s\n", rcFile)
	fmt.Println("Please restart your shell or run 'source " + rcFile + "' for the changes to take effect.")
	return nil
}
