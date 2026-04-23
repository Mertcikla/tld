package analyze

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mertcikla/tld/internal/analyzer"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/ignore"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var analyzeSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var analyzeService analyzer.Service = analyzer.DefaultService()

func NewAnalyzeCmd(wdir *string) *cobra.Command {
	var deep bool
	var dryRun bool
	var changedSince string

	c := &cobra.Command{
		Use:   "analyze <path>",
		Short: "Extract symbols from source files and upsert them as workspace elements",
		Long: `Walks the given path, extracts code symbols (functions, classes, types) using
tree-sitter grammar modules, and upserts each symbol as an Element in elements.yaml.
References and imports found between files, folders, and symbols are upserted as Connectors in connectors.yaml.

By default only the given path is scanned.  Use --deep to scan the entire git repo
for cross-file call references.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scanPath := args[0]

			absPath, err := filepath.Abs(scanPath)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			if _, err := os.Stat(absPath); err != nil {
				return fmt.Errorf("path %q not found: %w", scanPath, err)
			}

			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}

			repoScopes, err := cmdutil.ResolveAnalyzeRepoScopes(ws, absPath)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			totalElements := 0
			totalConnectors := 0
			incrementalFiles := 0
			totalEntries := 0
			knownElements := buildAnalyzeElementIndex(ws)
			usedNames := buildAnalyzeElementNameOwners(ws)
			modeLabel := "shallow"
			if deep {
				modeLabel = "deep"
			}
			linePrefix := ""
			if dryRun {
				linePrefix = "[dry-run] "
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sAnalyzing %s (%s)...\n", linePrefix, scanPath, modeLabel)
			workspaceRoot, _ := filepath.Abs(ws.Dir)
			workspaceScan := cmdutil.SamePath(absPath, workspaceRoot)
			scanConfiguredRepositories := workspaceScan && ws.WorkspaceConfig != nil && len(ws.WorkspaceConfig.Repositories) > 0
			countTasks := 0
			for _, repoCtx := range repoScopes {
				countTasks++
				if deep && repoCtx.Active() && !cmdutil.SamePath(absPath, workspaceRoot) {
					countTasks++
				}
			}
			countProgress := newAnalyzeProgressBar(cmd.ErrOrStderr(), countTasks)
			if countProgress != nil {
				defer func() {
					if !countProgress.IsFinished() {
						_ = countProgress.Clear()
					}
				}()
				countProgress.Describe(fmt.Sprintf("%s Counting scan plan", analyzeSpinnerFrames[0]))
			}
			for _, repoCtx := range repoScopes {
				rules := ws.IgnoreRulesForRepository(repoCtx.Name)
				scanRoot := absPath
				if scanConfiguredRepositories {
					scanRoot = repoCtx.Root
				}
				entries, err := countAnalyzeEntries(scanRoot, rules)
				if err != nil {
					return fmt.Errorf("count entries: %w", err)
				}
				totalEntries += entries
				if countProgress != nil {
					countProgress.Describe(fmt.Sprintf("%s Counting scan plan for %s", analyzeSpinnerFrames[entries%len(analyzeSpinnerFrames)], repoCtx.Name))
					_ = countProgress.Add(1)
				}
				if deep && repoCtx.Active() && !cmdutil.SamePath(absPath, workspaceRoot) {
					deepEntries, err := countAnalyzeEntries(repoCtx.Root, rules)
					if err != nil {
						return fmt.Errorf("count deep entries: %w", err)
					}
					totalEntries += deepEntries
					if countProgress != nil {
						countProgress.Describe(fmt.Sprintf("%s Counting deep scan for %s", analyzeSpinnerFrames[deepEntries%len(analyzeSpinnerFrames)], repoCtx.Name))
						_ = countProgress.Add(1)
					}
				}
			}
			if countProgress != nil {
				_ = countProgress.Finish()
			}

			progress := newAnalyzeProgressBar(cmd.ErrOrStderr(), totalEntries)
			if progress != nil {
				defer func() {
					if !progress.IsFinished() {
						_ = progress.Clear()
					}
				}()
			}
			processedEntries := 0

			for i, repoCtx := range repoScopes {
				if progress != nil {
					progress.Describe(fmt.Sprintf("%s Scanning %s (%d/%d)", analyzeSpinnerFrames[processedEntries%len(analyzeSpinnerFrames)], repoCtx.Name, i+1, len(repoScopes)))
				}
				rules := ws.IgnoreRulesForRepository(repoCtx.Name)
				scanRoot := absPath
				if scanConfiguredRepositories {
					scanRoot = repoCtx.Root
				}

				var repoURL, branch string
				if repoCtx.Active() {
					if url, err := git.DetectRemoteURL(repoCtx.Root); err == nil {
						repoURL = url
					}
					if b, err := git.DetectBranch(repoCtx.Root); err == nil {
						branch = b
					}
				}

				scanResult, err := analyzeService.ExtractPath(ctx, scanRoot, rules, func(path string, isDir bool) {
					processedEntries++
					if progress == nil {
						return
					}
					spinner := analyzeSpinnerFrames[processedEntries%len(analyzeSpinnerFrames)]
					progress.Describe(fmt.Sprintf("%s Scanning %s (%d/%d)", spinner, repoCtx.Name, processedEntries, totalEntries))
					_ = progress.Add(1)
				})
				if err != nil {
					return fmt.Errorf("extract symbols: %w", err)
				}

				changedFileSet := map[string]struct{}{}
				if changedSince != "" && repoCtx.Active() {
					changed, err := git.FilesChangedSince(repoCtx.Root, changedSince)
					if err != nil {
						return fmt.Errorf("git changed-since: %w", err)
					}
					incrementalFiles += len(changed)
					for _, file := range changed {
						changedFileSet[filepath.Clean(file)] = struct{}{}
					}
				}

				if deep && repoCtx.Active() && !workspaceScan {
					deepResult, err := analyzeService.ExtractPath(ctx, repoCtx.Root, rules, func(path string, isDir bool) {
						processedEntries++
						if progress == nil {
							return
						}
						spinner := analyzeSpinnerFrames[processedEntries%len(analyzeSpinnerFrames)]
						progress.Describe(fmt.Sprintf("%s Scanning %s (%d/%d)", spinner, repoCtx.Name, processedEntries, totalEntries))
						_ = progress.Add(1)
					})
					if err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: deep scan failed: %v\n", err)
					} else {
						scanResult.Refs = append(scanResult.Refs, deepResult.Refs...)
					}
				}

				filtered := filterSymbols(scanResult.Symbols, rules)
				if len(changedFileSet) > 0 {
					filtered = filterSymbolsByFiles(filtered, changedFileSet)
					scanResult.Refs = filterRefsByFiles(scanResult.Refs, changedFileSet)
				}

				if len(filtered) == 0 {
					continue
				}

				if progress != nil && !dryRun {
					_ = progress.Finish()
				}

				elementRoot := analyzeElementRoot(scanRoot, repoCtx.Root, repoCtx.Active())
				filePaths := uniqueFilePaths(filtered, elementRoot)
				folderPaths := uniqueFolderPaths(filePaths)
				plannedElementWrites := 1 + len(folderPaths) + len(filePaths) + len(filtered)
				writeProgress := newAnalyzeProgressBar(cmd.ErrOrStderr(), plannedElementWrites)
				elementWriteAttempts := 0

				usedRefs := make(map[string]struct{}, len(ws.Elements))
				for ref := range ws.Elements {
					usedRefs[ref] = struct{}{}
				}

				repoName := filepath.Base(repoCtx.Root)
				repoRef, err := ensureAnalyzeElement(*wdir, dryRun, ws, knownElements, usedRefs, usedNames, analyzeElementSpec{
					Name:       repoName,
					Kind:       "repository",
					Owner:      repoCtx.Name,
					Repo:       repoURL,
					Branch:     branch,
					HasView:    len(filtered) > 0,
					Technology: "Git Repository",
					ViewLabel:  repoName,
					ParentRef:  "root",
					Identity: analyzeElementIdentity{
						Repo:     repoURL,
						Branch:   branch,
						FilePath: "",
						Symbol:   "",
						Kind:     "repository",
						Name:     repoName,
					},
				})
				if err != nil {
					return fmt.Errorf("ensure repository element: %w", err)
				}
				if writeProgress != nil {
					elementWriteAttempts++
					advanceAnalyzeWriteProgress(writeProgress, "elements.yaml", elementWriteAttempts, plannedElementWrites)
				}

				folderRefs := make(map[string]string)
				fileRefs := make(map[string]string)
				symbolRefs := make(map[analyzeElementLookupKey]string)
				symbolRefsByName := make(map[string][]string)
				symbolFiles := make(map[string]string)
				repoElements := 1

				for _, relPath := range folderPaths {
					folderName := filepath.Base(relPath)
					parentRef := repoRef
					if parentPath := filepath.Dir(relPath); parentPath != "." {
						if existingParentRef := folderRefs[parentPath]; existingParentRef != "" {
							parentRef = existingParentRef
						}
					}

					folderRef, err := ensureAnalyzeElement(*wdir, dryRun, ws, knownElements, usedRefs, usedNames, analyzeElementSpec{
						Name:       folderName,
						Kind:       "folder",
						Owner:      repoCtx.Name,
						Repo:       repoURL,
						Branch:     branch,
						FilePath:   relPath,
						Technology: "Folder",
						ParentRef:  parentRef,
						Identity: analyzeElementIdentity{
							Repo:     repoURL,
							Branch:   branch,
							FilePath: relPath,
							Kind:     "folder",
							Name:     folderName,
						},
					})
					if err != nil {
						return fmt.Errorf("ensure folder element %q: %w", relPath, err)
					}
					if writeProgress != nil {
						elementWriteAttempts++
						advanceAnalyzeWriteProgress(writeProgress, "elements.yaml", elementWriteAttempts, plannedElementWrites)
					}
					folderRefs[relPath] = folderRef
					repoElements++
				}

				for _, relPath := range filePaths {
					fileName := filepath.Base(relPath)
					parentRef := repoRef
					if parentPath := filepath.Dir(relPath); parentPath != "." {
						if folderRef := folderRefs[parentPath]; folderRef != "" {
							parentRef = folderRef
						}
					}
					fileTech := ""
					if lang, ok := analyzer.DetectLanguage(relPath); ok {
						fileTech = string(lang)
					}
					hasFileSymbols := false
					for _, sym := range filtered {
						if filepath.Clean(sym.FilePath) == filepath.Clean(relPath) {
							hasFileSymbols = true
							break
						}
					}
					fileRef, err := ensureAnalyzeElement(*wdir, dryRun, ws, knownElements, usedRefs, usedNames, analyzeElementSpec{
						Name:       fileName,
						Kind:       "file",
						Owner:      repoCtx.Name,
						Repo:       repoURL,
						Branch:     branch,
						FilePath:   relPath,
						HasView:    hasFileSymbols,
						ViewLabel:  fileName,
						Technology: fileTech,
						ParentRef:  parentRef,
						Identity: analyzeElementIdentity{
							Repo:     repoURL,
							Branch:   branch,
							FilePath: relPath,
							Symbol:   "",
							Kind:     "file",
							Name:     fileName,
						},
					})
					if err != nil {
						return fmt.Errorf("ensure file element %q: %w", relPath, err)
					}
					if writeProgress != nil {
						elementWriteAttempts++
						advanceAnalyzeWriteProgress(writeProgress, "elements.yaml", elementWriteAttempts, plannedElementWrites)
					}
					fileRefs[relPath] = fileRef
					repoElements++
				}

				for _, sym := range filtered {
					relPath := analyzeRelativeFilePath(sym.FilePath, elementRoot)
					fileRef := fileRefs[relPath]
					if fileRef == "" {
						continue
					}

					parentRef := fileRef
					if sym.Parent != "" {
						if refs := symbolRefsByName[sym.Parent]; len(refs) == 1 {
							p := refs[0]
							parentRef = p
						}
					}

					ref, err := ensureAnalyzeElement(*wdir, dryRun, ws, knownElements, usedRefs, usedNames, analyzeElementSpec{
						Name:        sym.Name,
						Kind:        sym.Kind,
						Owner:       repoCtx.Name,
						Repo:        repoURL,
						Branch:      branch,
						FilePath:    relPath,
						Symbol:      sym.Name,
						ParentName:  sym.Parent,
						Technology:  sym.Technology,
						Description: sym.Description,
						ParentRef:   parentRef,
						Identity: analyzeElementIdentity{
							Repo:     repoURL,
							Branch:   branch,
							FilePath: relPath,
							Symbol:   sym.Name,
							Kind:     sym.Kind,
							Name:     sym.Name,
						},
					})
					if progress != nil && !dryRun {
						elementWriteAttempts++
						advanceAnalyzeWriteProgress(progress, "elements.yaml", elementWriteAttempts, plannedElementWrites)
					}
					if err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: upsert element %q: %v\n", sym.Name, err)
						continue
					}
					symbolRefs[analyzeSymbolLookupKey(sym)] = ref
					symbolRefsByName[sym.Name] = append(symbolRefsByName[sym.Name], ref)
					symbolFiles[ref] = relPath
					repoElements++
				}

				if !dryRun && repoElements > 0 {
					if err := workspace.Save(ws); err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: save elements: %v\n", err)
					}
				}

				resolverRoot := scanRoot
				if repoCtx.Active() {
					resolverRoot = repoCtx.Root
				}
				resolver := newAnalyzeLSPResolver(resolverRoot)
				modulePath := analyzeModulePath(resolverRoot)

				plannedResolutionSteps := len(scanResult.Refs)
				if writeProgress != nil && plannedResolutionSteps > 0 {
					writeProgress.AddMax(plannedResolutionSteps)
					describeAnalyzeResolutionProgress(writeProgress, repoCtx.DisplayName(), 0, plannedResolutionSteps)
				}
				resolvedSteps := 0
				plannedConnectors := make([]*workspace.Connector, 0, len(scanResult.Refs))
				for _, ref := range scanResult.Refs {
					resolvedSteps++
					if writeProgress != nil && plannedResolutionSteps > 0 {
						describeAnalyzeResolutionProgress(writeProgress, repoCtx.DisplayName(), resolvedSteps, plannedResolutionSteps)
					}
					if ref.Kind != "import" && rules.ShouldIgnoreSymbol(ref.Name) {
						if writeProgress != nil && plannedResolutionSteps > 0 {
							_ = writeProgress.Add(1)
						}
						continue
					}
					plannedConnectors = append(plannedConnectors, buildAnalyzeConnectorsForRef(
						ctx,
						resolver,
						ref,
						ws,
						filtered,
						symbolRefs,
						symbolRefsByName,
						fileRefs,
						folderRefs,
						symbolFiles,
						repoRef,
						elementRoot,
						modulePath,
					)...)
					if writeProgress != nil && plannedResolutionSteps > 0 {
						_ = writeProgress.Add(1)
					}
				}
				_ = resolver.Close()

				plannedConnectors = uniqueAnalyzeConnectors(plannedConnectors)

				repoConnectors := 0
				if writeProgress != nil && len(plannedConnectors) > 0 {
					writeProgress.AddMax(len(plannedConnectors))
				}

				if !dryRun {
					if err := workspace.AppendConnectors(*wdir, plannedConnectors); err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: write connectors: %v\n", err)
					} else {
						repoConnectors = len(plannedConnectors)
					}
					if writeProgress != nil {
						advanceAnalyzeWriteProgress(writeProgress, "connectors.yaml", len(plannedConnectors), len(plannedConnectors))
					}
				} else {
					repoConnectors = len(plannedConnectors)
				}

				totalElements += repoElements
				totalConnectors += repoConnectors
			}
			if progress != nil {
				_ = progress.Finish()
			}
			if !dryRun {
				if err := ensureAnalyzeRepositoriesRegistered(ws, repoScopes); err != nil {
					return fmt.Errorf("register analyzed repositories: %w", err)
				}
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  %d elements written to elements.yaml\n", linePrefix, totalElements)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  %d connectors written to connectors.yaml\n", linePrefix, totalConnectors)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  %d repositories scanned\n", linePrefix, len(repoScopes))
			if changedSince != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  Incremental scan: %d files changed since %s\n", linePrefix, incrementalFiles, changedSince)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			if dryRun {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sNo files written. Remove --dry-run to apply.\n", linePrefix)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Tip: run `tld plan` to preview what will be applied.")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&deep, "deep", false, "scan entire git repo for cross-file references (slower)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written without modifying workspace")
	c.Flags().StringVar(&changedSince, "changed-since", "", "only re-analyse files changed since this git SHA")
	return c
}

func ensureAnalyzeRepositoriesRegistered(ws *workspace.Workspace, repoScopes []cmdutil.RepoScope) error {
	if ws == nil {
		return nil
	}

	config := ws.WorkspaceConfig
	if config == nil {
		config = &workspace.WorkspaceConfig{}
	}
	if config.Repositories == nil {
		config.Repositories = make(map[string]workspace.Repository)
	}

	workspaceRoot, err := filepath.Abs(ws.Dir)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}

	changed := false
	for _, repoScope := range repoScopes {
		if repoScope.Name == "" {
			continue
		}
		if _, exists := config.Repositories[repoScope.Name]; exists {
			continue
		}
		config.Repositories[repoScope.Name] = workspace.Repository{
			URL:      repoScope.RemoteURL,
			LocalDir: analyzeRepositoryLocalDir(workspaceRoot, repoScope.Root),
		}
		changed = true
	}
	if !changed {
		return nil
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal workspace config: %w", err)
	}
	if err := os.WriteFile(workspace.WorkspaceConfigPath(ws.Dir), data, 0600); err != nil {
		return fmt.Errorf("write .tld.yaml: %w", err)
	}
	ws.WorkspaceConfig = config
	return nil
}

func analyzeRepositoryLocalDir(workspaceRoot, repoRoot string) string {
	if workspaceRoot == "" || repoRoot == "" {
		return repoRoot
	}
	parent := filepath.Dir(workspaceRoot)
	if cmdutil.SamePath(repoRoot, parent) {
		return ""
	}
	if rel, err := filepath.Rel(workspaceRoot, repoRoot); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return rel
	}
	return repoRoot
}

func newAnalyzeProgressBar(out io.Writer, total int) *progressbar.ProgressBar {
	if total <= 0 || !term.IsTerminal(out) {
		return nil
	}
	return progressbar.NewOptions(total,
		progressbar.OptionSetWriter(out),
		progressbar.OptionSetVisibility(true),
		progressbar.OptionSetDescription("⠋ Scanning"),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetWidth(12),
		progressbar.OptionFullWidth(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionThrottle(60*time.Millisecond),
	)
}

func advanceAnalyzeWriteProgress(progress *progressbar.ProgressBar, fileName string, completed, total int) {
	if progress == nil || total <= 0 {
		return
	}
	spinner := analyzeSpinnerFrames[completed%len(analyzeSpinnerFrames)]
	progress.Describe(fmt.Sprintf("%s Writing %s (%d/%d)", spinner, fileName, completed, total))
	_ = progress.Add(1)
}

func describeAnalyzeResolutionProgress(progress *progressbar.ProgressBar, repoName string, completed, total int) {
	if progress == nil || total <= 0 {
		return
	}
	spinner := analyzeSpinnerFrames[completed%len(analyzeSpinnerFrames)]
	progress.Describe(fmt.Sprintf("%s Resolving symbols via LSP in %s (%d/%d)", spinner, repoName, completed, total))
}

func countAnalyzeEntries(path string, rules *ignore.Rules) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		if rules.ShouldIgnoreFile(path) {
			return 0, nil
		}
		return 1, nil
	}

	count := 0
	err = filepath.WalkDir(path, func(currentPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(path, currentPath)
			if rules.ShouldIgnorePath(rel) || rules.ShouldIgnorePath(d.Name()) {
				return filepath.SkipDir
			}
			count++
			return nil
		}
		if rules.ShouldIgnorePath(currentPath) {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func filterSymbols(symbols []analyzer.Symbol, rules *ignore.Rules) []analyzer.Symbol {
	var out []analyzer.Symbol
	for _, s := range symbols {
		if rules.ShouldIgnoreSymbol(s.Name) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func filterSymbolsByFiles(symbols []analyzer.Symbol, changedFiles map[string]struct{}) []analyzer.Symbol {
	var out []analyzer.Symbol
	for _, sym := range symbols {
		if _, ok := changedFiles[filepath.Clean(sym.FilePath)]; ok {
			out = append(out, sym)
		}
	}
	return out
}

func filterRefsByFiles(refs []analyzer.Ref, changedFiles map[string]struct{}) []analyzer.Ref {
	var out []analyzer.Ref
	for _, ref := range refs {
		if _, ok := changedFiles[filepath.Clean(ref.FilePath)]; ok {
			out = append(out, ref)
		}
	}
	return out
}

func refByFileAndLine(filePath string, line int, refMap map[analyzeElementLookupKey]string, symbols []analyzer.Symbol) string {
	symbol, ok := symbolByFileAndLine(filePath, line, symbols)
	if !ok {
		return ""
	}
	return refMap[analyzeSymbolLookupKey(symbol)]
}

type analyzeElementIdentity struct {
	Repo     string
	Branch   string
	FilePath string
	Symbol   string
	Kind     string
	Name     string
}

type analyzeElementLookupKey struct {
	Branch   string
	FilePath string
	Symbol   string
	Kind     string
}

type analyzeElementSpec struct {
	Name        string
	Kind        string
	Owner       string
	Repo        string
	Branch      string
	FilePath    string
	Symbol      string
	ParentName  string
	HasView     bool
	ViewLabel   string
	ParentRef   string
	Technology  string
	Description string
	Identity    analyzeElementIdentity
}

func buildAnalyzeElementIndex(ws *workspace.Workspace) map[analyzeElementLookupKey]string {
	index := make(map[analyzeElementLookupKey]string, len(ws.Elements))
	for ref, element := range ws.Elements {
		if element == nil {
			continue
		}
		index[analyzeElementLookupKey{
			Branch:   element.Branch,
			FilePath: normalizeAnalyzePath(element.FilePath),
			Symbol:   element.Symbol,
			Kind:     element.Kind,
		}] = ref
	}
	return index
}

func buildAnalyzeElementNameOwners(ws *workspace.Workspace) map[string]map[string]struct{} {
	owners := make(map[string]map[string]struct{}, len(ws.Elements))
	for ref, element := range ws.Elements {
		if element == nil || element.Name == "" {
			continue
		}
		if owners[element.Name] == nil {
			owners[element.Name] = make(map[string]struct{})
		}
		owners[element.Name][ref] = struct{}{}
	}
	return owners
}

func normalizeAnalyzePath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}

func normalizeAnalyzeElementLookupKey(identity analyzeElementIdentity) analyzeElementLookupKey {
	return analyzeElementLookupKey{
		Branch:   identity.Branch,
		FilePath: normalizeAnalyzePath(identity.FilePath),
		Symbol:   identity.Symbol,
		Kind:     identity.Kind,
	}
}

func ensureAnalyzeElement(wdir string, dryRun bool, ws *workspace.Workspace, known map[analyzeElementLookupKey]string, usedRefs map[string]struct{}, usedNames map[string]map[string]struct{}, spec analyzeElementSpec) (string, error) {
	identity := normalizeAnalyzeElementLookupKey(spec.Identity)
	ref := ""
	if knownRef, ok := known[identity]; ok {
		ref = knownRef
	} else if existingRef, ok := findAnalyzeElementRef(ws, analyzeElementIdentity{
		Branch:   identity.Branch,
		FilePath: identity.FilePath,
		Symbol:   identity.Symbol,
		Kind:     identity.Kind,
	}); ok {
		ref = existingRef
		known[identity] = ref
	} else {
		ref = uniqueAnalyzeRef(spec.Name, spec.FilePath, usedRefs)
		usedRefs[ref] = struct{}{}
		known[identity] = ref
	}
	if ref == "" {
		ref = uniqueAnalyzeRef(spec.Name, spec.FilePath, usedRefs)
		usedRefs[ref] = struct{}{}
		known[identity] = ref
	}

	if ws.Elements != nil {
		if existing := ws.Elements[ref]; existing != nil && existing.Name != "" {
			releaseAnalyzeElementName(usedNames, existing.Name, ref)
		}
	}
	spec.Name = uniqueAnalyzeElementName(ref, spec, usedNames)
	claimAnalyzeElementName(usedNames, spec.Name, ref)
	if dryRun {
		return ref, nil
	}
	elementSpec := analyzeElementToWorkspaceElement(spec)
	if existing := ws.Elements[ref]; existing != nil {
		if elementSpec.Description == "" {
			elementSpec.Description = existing.Description
		}
		if elementSpec.Technology == "" {
			elementSpec.Technology = existing.Technology
		}
		if elementSpec.URL == "" {
			elementSpec.URL = existing.URL
		}
	}
	if ws.Elements == nil {
		ws.Elements = make(map[string]*workspace.Element)
	}
	ws.Elements[ref] = elementSpec
	return ref, nil
}

func analyzeSymbolLookupKey(symbol analyzer.Symbol) analyzeElementLookupKey {
	return analyzeElementLookupKey{
		FilePath: filepath.Clean(symbol.FilePath),
		Symbol:   symbol.Name,
		Kind:     symbol.Kind,
	}
}

func analyzeElementToWorkspaceElement(spec analyzeElementSpec) *workspace.Element {
	return &workspace.Element{
		Name:        spec.Name,
		Kind:        spec.Kind,
		Owner:       spec.Owner,
		Repo:        spec.Repo,
		Branch:      spec.Branch,
		FilePath:    spec.FilePath,
		Symbol:      spec.Symbol,
		HasView:     spec.HasView,
		ViewLabel:   spec.ViewLabel,
		Technology:  spec.Technology,
		Description: spec.Description,
		Placements: []workspace.ViewPlacement{{
			ParentRef: spec.ParentRef,
		}},
	}
}

func findAnalyzeElementRef(ws *workspace.Workspace, identity analyzeElementIdentity) (string, bool) {
	targetPath := normalizeAnalyzePath(identity.FilePath)

	// 1. Try exact match including branch
	for ref, element := range ws.Elements {
		if element == nil {
			continue
		}
		if element.Kind == identity.Kind &&
			normalizeAnalyzePath(element.FilePath) == targetPath &&
			element.Symbol == identity.Symbol &&
			element.Branch == identity.Branch {
			return ref, true
		}
	}

	// 2. Try lenient match excluding branch
	for ref, element := range ws.Elements {
		if element == nil {
			continue
		}
		if element.Kind == identity.Kind &&
			normalizeAnalyzePath(element.FilePath) == targetPath &&
			element.Symbol == identity.Symbol {
			return ref, true
		}
	}

	return "", false
}

func uniqueAnalyzeElementName(ref string, spec analyzeElementSpec, usedNames map[string]map[string]struct{}) string {
	for _, candidate := range analyzeElementNameCandidates(spec) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if owners := usedNames[candidate]; len(owners) == 0 || (len(owners) == 1 && containsAnalyzeNameOwner(owners, ref)) {
			return candidate
		}
	}
	base := spec.Name
	if candidates := analyzeElementNameCandidates(spec); len(candidates) > 0 {
		base = candidates[len(candidates)-1]
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)", base, i)
		if owners := usedNames[candidate]; len(owners) == 0 || (len(owners) == 1 && containsAnalyzeNameOwner(owners, ref)) {
			return candidate
		}
	}
}

func analyzeElementNameCandidates(spec analyzeElementSpec) []string {
	rawName := strings.TrimSpace(spec.Name)
	filePath := analyzeQualifiedElementPath(spec.Owner, spec.FilePath)
	qualifiedSymbol := rawName
	if spec.ParentName != "" {
		qualifiedSymbol = spec.ParentName + "." + rawName
	}

	candidates := []string{rawName}
	switch {
	case spec.Kind == "repository":
		if spec.Owner != "" {
			candidates = append(candidates, spec.Owner)
		}
	case spec.Symbol != "":
		if qualifiedSymbol != rawName {
			candidates = append(candidates, qualifiedSymbol)
		}
		if spec.FilePath != "" {
			candidates = append(candidates, filepath.ToSlash(filepath.Clean(spec.FilePath))+"::"+qualifiedSymbol)
		}
		if filePath != "" {
			candidates = append(candidates, filePath+"::"+qualifiedSymbol)
		}
	case spec.FilePath != "":
		candidates = append(candidates, filepath.ToSlash(filepath.Clean(spec.FilePath)))
		if filePath != "" {
			candidates = append(candidates, filePath)
		}
	}
	return candidates
}

func analyzeQualifiedElementPath(owner, path string) string {
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	if cleanPath == "" || cleanPath == "." {
		return strings.TrimSpace(owner)
	}
	if owner == "" {
		return cleanPath
	}
	return owner + "/" + cleanPath
}

func claimAnalyzeElementName(usedNames map[string]map[string]struct{}, name, ref string) {
	if name == "" || ref == "" {
		return
	}
	if usedNames[name] == nil {
		usedNames[name] = make(map[string]struct{})
	}
	usedNames[name][ref] = struct{}{}
}

func releaseAnalyzeElementName(usedNames map[string]map[string]struct{}, name, ref string) {
	owners := usedNames[name]
	if len(owners) == 0 {
		return
	}
	delete(owners, ref)
	if len(owners) == 0 {
		delete(usedNames, name)
	}
}

func containsAnalyzeNameOwner(owners map[string]struct{}, ref string) bool {
	_, ok := owners[ref]
	return ok
}

func uniqueAnalyzeRef(name, filePath string, used map[string]struct{}) string {
	base := workspace.Slugify(name)
	if base == "" {
		base = "element"
	}
	if _, taken := used[base]; !taken {
		return base
	}
	fileBase := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	candidate := workspace.Slugify(fileBase + "-" + name)
	if candidate == "" {
		candidate = base
	}
	if _, taken := used[candidate]; !taken {
		return candidate
	}
	for i := 2; ; i++ {
		withSuffix := fmt.Sprintf("%s-%d", candidate, i)
		if _, taken := used[withSuffix]; !taken {
			return withSuffix
		}
	}
}

func uniqueFilePaths(symbols []analyzer.Symbol, root string) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		relPath := analyzeRelativeFilePath(sym.FilePath, root)
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}
		paths = append(paths, relPath)
	}
	return paths
}

func uniqueFolderPaths(filePaths []string) []string {
	seen := make(map[string]struct{})
	folders := make([]string, 0, len(filePaths))
	for _, filePath := range filePaths {
		for dir := filepath.Dir(filePath); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
			dir = filepath.Clean(dir)
			if _, ok := seen[dir]; ok {
				if next := filepath.Dir(dir); next == dir {
					break
				}
				continue
			}
			seen[dir] = struct{}{}
			folders = append(folders, dir)
		}
	}
	sort.Slice(folders, func(i, j int) bool {
		leftDepth := strings.Count(filepath.ToSlash(folders[i]), "/")
		rightDepth := strings.Count(filepath.ToSlash(folders[j]), "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return folders[i] < folders[j]
	})
	return folders
}

func analyzeElementRoot(scanRoot, repoRoot string, activeRepo bool) string {
	cleanScanRoot := filepath.Clean(scanRoot)
	if activeRepo && cmdutil.PathWithin(cleanScanRoot, filepath.Clean(repoRoot)) {
		return filepath.Clean(repoRoot)
	}
	info, err := os.Stat(cleanScanRoot)
	if err == nil && !info.IsDir() {
		return filepath.Dir(cleanScanRoot)
	}
	return cleanScanRoot
}

func analyzeRelativeFilePath(path, root string) string {
	cleanPath := filepath.Clean(path)
	if root == "" || cleanPath == "" || !filepath.IsAbs(cleanPath) {
		return normalizeAnalyzePath(cleanPath)
	}
	if relPath, ok := analyzePathWithinRoot(root, cleanPath); ok {
		return normalizeAnalyzePath(relPath)
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	resolvedPath, pathErr := filepath.EvalSymlinks(cleanPath)
	if rootErr == nil && pathErr == nil {
		if relPath, ok := analyzePathWithinRoot(resolvedRoot, resolvedPath); ok {
			return normalizeAnalyzePath(relPath)
		}
	}
	return normalizeAnalyzePath(cleanPath)
}

func analyzePathWithinRoot(root, path string) (string, bool) {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	relPath = filepath.Clean(relPath)
	if relPath == "." {
		return relPath, true
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return relPath, true
}

func uniqueAnalyzeConnectors(connectors []*workspace.Connector) []*workspace.Connector {
	seen := make(map[string]*workspace.Connector, len(connectors))
	unique := make([]*workspace.Connector, 0, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		if merged := analyzeMergeDuplicateConnector(seen, connector); merged {
			continue
		}
		key := workspace.ConnectorKey(connector)
		reverseKey := analyzeReverseConnectorKey(connector)
		if existing, ok := seen[reverseKey]; ok {
			existing.Direction = "both"
			continue
		}
		seen[key] = connector
		unique = append(unique, connector)
	}
	return unique
}

func analyzeMergeDuplicateConnector(seen map[string]*workspace.Connector, connector *workspace.Connector) bool {
	if connector == nil {
		return false
	}
	key := workspace.ConnectorKey(connector)
	if existing, ok := seen[key]; ok {
		if existing.Relationship == "depends_on" && connector.Relationship == "depends_on" {
			existing.Label = analyzeMergeDependencyLabels(existing.Label, connector.Label)
			delete(seen, key)
			seen[workspace.ConnectorKey(existing)] = existing
		}
		return true
	}
	mergeKey := analyzeDependencyMergeKey(connector)
	if mergeKey == "" {
		return false
	}
	for existingKey, existing := range seen {
		if existing == nil || analyzeDependencyMergeKey(existing) != mergeKey {
			continue
		}
		existing.Label = analyzeMergeDependencyLabels(existing.Label, connector.Label)
		delete(seen, existingKey)
		seen[workspace.ConnectorKey(existing)] = existing
		return true
	}
	return false
}

func analyzeDependencyMergeKey(connector *workspace.Connector) string {
	if connector == nil || connector.Relationship != "depends_on" || connector.Direction != "forward" {
		return ""
	}
	return connector.View + ":" + connector.Source + ":" + connector.Target + ":" + connector.Relationship
}

func analyzeReverseConnectorKey(connector *workspace.Connector) string {
	if connector == nil {
		return ""
	}
	return connector.View + ":" + connector.Target + ":" + connector.Source + ":" + connector.Label
}
