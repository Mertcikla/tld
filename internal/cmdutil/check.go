package cmdutil

import (
	"context"
	"fmt"
	"os"

	"github.com/mertcikla/diag/tld/internal/analyzer"
	"github.com/mertcikla/diag/tld/internal/git"
	"github.com/mertcikla/diag/tld/internal/ignore"
	"github.com/mertcikla/diag/tld/internal/workspace"
)

func CheckSymbols(ctx context.Context, ws *workspace.Workspace, repoCtx RepoScope, rules *ignore.Rules) []string {
	var failures []string
	for ref, element := range ws.Elements {
		if element.FilePath == "" || element.Symbol == "" {
			continue
		}
		if !repoCtx.MatchesElement(element) {
			continue
		}
		if rules != nil && (rules.ShouldIgnorePath(element.FilePath) || rules.ShouldIgnoreSymbol(element.Symbol)) {
			continue
		}
		absPath := repoCtx.ResolvePath(element.FilePath)
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		found, err := analyzer.HasSymbol(ctx, absPath, element.Symbol)
		if err != nil {
			if analyzer.IsUnsupportedLanguage(err) {
				continue
			}
			failures = append(failures, fmt.Sprintf("elements.yaml[%s]: %v", ref, err))
			continue
		}
		if !found {
			failures = append(failures, fmt.Sprintf(
				"elements.yaml[%s]: symbol %q not found in %s",
				ref, element.Symbol, element.FilePath,
			))
		}
	}
	return failures
}

func CheckOutdated(ws *workspace.Workspace, repoCtx RepoScope, rules *ignore.Rules) []string {
	var outdated []string

	if ws.Meta == nil || ws.Meta.Elements == nil {
		return nil
	}

	if !repoCtx.Active() {
		return nil
	}

	for ref, element := range ws.Elements {
		if element.FilePath == "" || !repoCtx.MatchesElement(element) {
			continue
		}
		if rules != nil && rules.ShouldIgnorePath(element.FilePath) {
			continue
		}
		meta, ok := ws.Meta.Elements[ref]
		if !ok || meta.UpdatedAt.IsZero() {
			continue
		}
		commitTime, err := git.FileLastCommitAt(repoCtx.Root, element.FilePath)
		if err != nil {
			continue
		}
		if commitTime.After(meta.UpdatedAt) {
			outdated = append(outdated, fmt.Sprintf(
				"elements.yaml[%s]: file %s changed %s, diagram last synced %s",
				ref,
				element.FilePath,
				commitTime.Format("2006-01-02 15:04:05"),
				meta.UpdatedAt.Format("2006-01-02 15:04:05"),
			))
		}
	}
	return outdated
}
