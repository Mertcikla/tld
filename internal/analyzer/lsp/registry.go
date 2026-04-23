package lsp

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mertcikla/diag/tld/internal/analyzer"
)

type LookPathFunc func(file string) (string, error)

type ServerCommand struct {
	Executable string
	Args       []string
}

type ResolvedCommand struct {
	Path string
	Args []string
}

type ErrServerNotConfigured struct {
	Language analyzer.Language
}

func (e ErrServerNotConfigured) Error() string {
	return fmt.Sprintf("no LSP server configured for %s", e.Language)
}

type ErrServerNotFound struct {
	Language   analyzer.Language
	Candidates []ServerCommand
}

func (e ErrServerNotFound) Error() string {
	parts := make([]string, 0, len(e.Candidates))
	for _, candidate := range e.Candidates {
		parts = append(parts, candidate.Display())
	}
	return fmt.Sprintf("no installed LSP server found for %s (tried: %s)", e.Language, strings.Join(parts, ", "))
}

func (c ServerCommand) Display() string {
	if len(c.Args) == 0 {
		return c.Executable
	}
	return c.Executable + " " + strings.Join(c.Args, " ")
}

func DefaultCommands(language analyzer.Language) []ServerCommand {
	commands, ok := defaultCommands[language]
	if !ok {
		return nil
	}
	cloned := make([]ServerCommand, 0, len(commands))
	for _, command := range commands {
		cloned = append(cloned, ServerCommand{
			Executable: command.Executable,
			Args:       append([]string{}, command.Args...),
		})
	}
	return cloned
}

func ResolveServerCommand(language analyzer.Language) (ResolvedCommand, error) {
	return ResolveServerCommandWithLookPath(language, exec.LookPath)
}

func ResolveServerCommandWithLookPath(language analyzer.Language, lookPath LookPathFunc) (ResolvedCommand, error) {
	commands := DefaultCommands(language)
	if len(commands) == 0 {
		return ResolvedCommand{}, ErrServerNotConfigured{Language: language}
	}
	for _, command := range commands {
		path, err := lookPath(command.Executable)
		if err == nil {
			return ResolvedCommand{Path: path, Args: append([]string{}, command.Args...)}, nil
		}
	}
	return ResolvedCommand{}, ErrServerNotFound{Language: language, Candidates: commands}
}

var defaultCommands = map[analyzer.Language][]ServerCommand{
	analyzer.LanguageGo: {
		{Executable: "gopls"},
	},
	analyzer.LanguagePython: {
		{Executable: "pyright-langserver", Args: []string{"--stdio"}},
	},
	analyzer.LanguageRust: {
		{Executable: "rust-analyzer"},
	},
	analyzer.LanguageJava: {
		{Executable: "jdtls"},
	},
	analyzer.LanguageTypeScript: {
		{Executable: "typescript-language-server", Args: []string{"--stdio"}},
	},
	analyzer.LanguageJavaScript: {
		{Executable: "typescript-language-server", Args: []string{"--stdio"}},
	},
	analyzer.LanguageC: {
		{Executable: "clangd"},
	},
	analyzer.LanguageCPP: {
		{Executable: "clangd"},
	},
}
