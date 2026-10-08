package cmdutil

import (
	"context"
	"fmt"
	"os"

	"github.com/mertcikla/tld/v2/internal/codeindex/symbolcheck"
	"github.com/mertcikla/tld/v2/internal/ignore"
	"github.com/mertcikla/tld/v2/internal/workspace"
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
		found, err := symbolcheck.HasSymbol(ctx, absPath, element.Symbol)
		if err != nil {
			if symbolcheck.IsUnsupported(err) {
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
