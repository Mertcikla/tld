package analyze

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/internal/analyzer"
	"github.com/mertcikla/tld/internal/workspace"
)

func buildAnalyzeConnectorsForRef(
	ctx context.Context,
	resolver analyzeDefinitionResolver,
	ref analyzer.Ref,
	ws *workspace.Workspace,
	symbols []analyzer.Symbol,
	symbolRefs map[analyzeElementLookupKey]string,
	symbolRefsByName map[string][]string,
	fileRefs map[string]string,
	folderRefs map[string]string,
	symbolFiles map[string]string,
	repoRef string,
	elementRoot string,
	modulePath string,
) []*workspace.Connector {
	kind := strings.TrimSpace(ref.Kind)
	if kind == "import" {
		return buildAnalyzeImportConnectors(ref, ws, fileRefs, folderRefs, repoRef, elementRoot, modulePath)
	}
	return buildAnalyzeReferenceConnectors(ctx, resolver, ref, ws, symbols, symbolRefs, symbolRefsByName, fileRefs, folderRefs, symbolFiles, repoRef)
}

func buildAnalyzeReferenceConnectors(
	ctx context.Context,
	resolver analyzeDefinitionResolver,
	ref analyzer.Ref,
	ws *workspace.Workspace,
	symbols []analyzer.Symbol,
	symbolRefs map[analyzeElementLookupKey]string,
	symbolRefsByName map[string][]string,
	fileRefs map[string]string,
	folderRefs map[string]string,
	symbolFiles map[string]string,
	repoRef string,
) []*workspace.Connector {
	toRef := resolveAnalyzeTargetRef(ctx, resolver, ref, symbols, symbolRefs, symbolRefsByName)
	if toRef == "" {
		return nil
	}

	fromRef := refByFileAndLine(ref.FilePath, ref.Line, symbolRefs, symbols)
	if fromRef == "" || fromRef == toRef {
		return nil
	}

	connectors := []*workspace.Connector{{
		View:         analyzeCommonConnectorView(ws, fromRef, toRef, repoRef),
		Source:       fromRef,
		Target:       toRef,
		Label:        "calls",
		Relationship: "uses",
		Direction:    "forward",
	}}

	sourceFile := symbolFiles[fromRef]
	targetFile := symbolFiles[toRef]
	if sourceFile == "" || targetFile == "" || sourceFile == targetFile {
		return connectors
	}

	sourceFileRef := fileRefs[sourceFile]
	targetFileRef := fileRefs[targetFile]
	if sourceFileRef != "" && targetFileRef != "" && sourceFileRef != targetFileRef {
		connectors = append(connectors, &workspace.Connector{
			View:         analyzeCommonConnectorView(ws, sourceFileRef, targetFileRef, repoRef),
			Source:       sourceFileRef,
			Target:       targetFileRef,
			Label:        analyzeDependencyLabelReference,
			Relationship: "depends_on",
			Direction:    "forward",
		})
	}

	sourceFolderRef := analyzeFolderRefForFile(sourceFile, folderRefs, repoRef)
	targetFolderRef := analyzeFolderRefForFile(targetFile, folderRefs, repoRef)
	if sourceFolderRef != "" && targetFolderRef != "" && sourceFolderRef != targetFolderRef {
		connectors = append(connectors, &workspace.Connector{
			View:         analyzeCommonConnectorView(ws, sourceFolderRef, targetFolderRef, repoRef),
			Source:       sourceFolderRef,
			Target:       targetFolderRef,
			Label:        analyzeDependencyLabelReference,
			Relationship: "depends_on",
			Direction:    "forward",
		})
	}

	return connectors
}

func buildAnalyzeImportConnectors(
	ref analyzer.Ref,
	ws *workspace.Workspace,
	fileRefs map[string]string,
	folderRefs map[string]string,
	repoRef string,
	elementRoot string,
	modulePath string,
) []*workspace.Connector {
	targetDir := analyzeRepoRelativeImportDir(ref.TargetPath, modulePath)
	if targetDir == "" && strings.HasSuffix(ref.FilePath, ".py") {
		targetDir = resolvePythonImportDir(ref.FilePath, ref.TargetPath, elementRoot)
	}

	if targetDir == "" {
		return nil
	}

	sourceFile := analyzeRelativeFilePath(ref.FilePath, elementRoot)
	sourceFileRef := fileRefs[sourceFile]
	targetFolderRef := analyzeFolderRefForDir(targetDir, folderRefs, repoRef)
	if sourceFileRef == "" || targetFolderRef == "" || sourceFileRef == targetFolderRef {
		return nil
	}

	connectors := []*workspace.Connector{{
		View:         analyzeCommonConnectorView(ws, sourceFileRef, targetFolderRef, repoRef),
		Source:       sourceFileRef,
		Target:       targetFolderRef,
		Label:        analyzeDependencyLabelImport,
		Relationship: "depends_on",
		Direction:    "forward",
	}}

	sourceFolderRef := analyzeFolderRefForFile(sourceFile, folderRefs, repoRef)
	if sourceFolderRef != "" && sourceFolderRef != targetFolderRef {
		connectors = append(connectors, &workspace.Connector{
			View:         analyzeCommonConnectorView(ws, sourceFolderRef, targetFolderRef, repoRef),
			Source:       sourceFolderRef,
			Target:       targetFolderRef,
			Label:        analyzeDependencyLabelImport,
			Relationship: "depends_on",
			Direction:    "forward",
		})
	}

	return connectors
}

func resolvePythonImportDir(filePath, targetPath, elementRoot string) string {
	if strings.HasPrefix(targetPath, ".") {
		return resolvePythonRelativeImport(filePath, targetPath, elementRoot)
	}
	return resolvePythonAbsoluteImport(targetPath, elementRoot)
}

func resolvePythonRelativeImport(filePath, targetPath, elementRoot string) string {
	dir := filepath.Dir(filePath)
	dots := 0
	for strings.HasPrefix(targetPath[dots:], ".") {
		dots++
	}
	for i := 0; i < dots-1; i++ {
		dir = filepath.Dir(dir)
	}
	importPath := strings.ReplaceAll(targetPath[dots:], ".", "/")
	fullPath := filepath.Join(dir, importPath)
	rel, err := filepath.Rel(elementRoot, fullPath)
	if err != nil {
		return ""
	}
	return filepath.Clean(rel)
}

func resolvePythonAbsoluteImport(targetPath, elementRoot string) string {
	importPath := strings.ReplaceAll(targetPath, ".", "/")
	fullPath := filepath.Join(elementRoot, importPath)

	// Check if it's a directory (package)
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return importPath
	}
	// Check if it's a file (module)
	if info, err := os.Stat(fullPath + ".py"); err == nil && !info.IsDir() {
		return filepath.Dir(importPath)
	}

	return ""
}

func analyzeFolderRefForFile(filePath string, folderRefs map[string]string, repoRef string) string {
	return analyzeFolderRefForDir(filepath.Dir(filePath), folderRefs, repoRef)
}

func analyzeFolderRefForDir(dir string, folderRefs map[string]string, repoRef string) string {
	cleanDir := filepath.Clean(dir)
	if cleanDir == "." || cleanDir == string(os.PathSeparator) || cleanDir == "" {
		return repoRef
	}
	if ref := folderRefs[cleanDir]; ref != "" {
		return ref
	}
	return ""
}

func analyzeRepoRelativeImportDir(importPath, modulePath string) string {
	cleanImportPath := strings.TrimSpace(importPath)
	cleanModulePath := strings.TrimSpace(modulePath)
	if cleanImportPath == "" || cleanModulePath == "" {
		return ""
	}
	if cleanImportPath == cleanModulePath {
		return "."
	}
	prefix := cleanModulePath + "/"
	if !strings.HasPrefix(cleanImportPath, prefix) {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(strings.TrimPrefix(cleanImportPath, prefix)))
}

func analyzeModulePath(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if !strings.HasPrefix(trimmed, "module ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			return ""
		}
		return strings.Trim(fields[1], "\"")
	}
	return ""
}

const (
	analyzeDependencyLabelImport    = "imports"
	analyzeDependencyLabelReference = "references"
	analyzeDependencyLabelBoth      = "depends_on"
)

func analyzeDependencyKindsForLabel(label string) (hasImport bool, hasReference bool) {
	switch strings.TrimSpace(label) {
	case analyzeDependencyLabelImport:
		return true, false
	case analyzeDependencyLabelReference:
		return false, true
	case analyzeDependencyLabelBoth:
		return true, true
	default:
		return false, false
	}
}

func analyzeMergeDependencyLabels(left, right string) string {
	leftImport, leftReference := analyzeDependencyKindsForLabel(left)
	rightImport, rightReference := analyzeDependencyKindsForLabel(right)
	hasImport := leftImport || rightImport
	hasReference := leftReference || rightReference
	switch {
	case hasImport && hasReference:
		return analyzeDependencyLabelBoth
	case hasImport:
		return analyzeDependencyLabelImport
	case hasReference:
		return analyzeDependencyLabelReference
	default:
		return left
	}
}

func analyzeCommonConnectorView(ws *workspace.Workspace, fromRef, toRef, fallback string) string {
	if fallback == "" {
		fallback = "root"
	}
	if ws == nil {
		return fallback
	}
	fromAncestors := analyzeAncestorDepths(ws, fromRef)
	toAncestors := analyzeAncestorDepths(ws, toRef)
	bestRef := fallback
	bestScore := int(^uint(0) >> 1)
	for ref, fromDepth := range fromAncestors {
		toDepth, ok := toAncestors[ref]
		if !ok {
			continue
		}
		score := fromDepth + toDepth
		if score < bestScore || (score == bestScore && bestRef == "root" && ref != "root") {
			bestRef = ref
			bestScore = score
		}
	}
	return bestRef
}

func analyzeAncestorDepths(ws *workspace.Workspace, ref string) map[string]int {
	depths := map[string]int{"root": 1 << 20}
	type queueItem struct {
		ref   string
		depth int
	}
	queue := make([]queueItem, 0, 4)
	seedParents := []string{"root"}
	if element := ws.Elements[ref]; element != nil && len(element.Placements) > 0 {
		seedParents = seedParents[:0]
		for _, placement := range element.Placements {
			parentRef := placement.ParentRef
			if parentRef == "" {
				parentRef = "root"
			}
			seedParents = append(seedParents, parentRef)
		}
	}
	for _, parentRef := range seedParents {
		queue = append(queue, queueItem{ref: parentRef, depth: 0})
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if existingDepth, ok := depths[current.ref]; ok && existingDepth <= current.depth {
			continue
		}
		depths[current.ref] = current.depth
		if current.ref == "root" {
			continue
		}
		element := ws.Elements[current.ref]
		if element == nil || len(element.Placements) == 0 {
			queue = append(queue, queueItem{ref: "root", depth: current.depth + 1})
			continue
		}
		for _, placement := range element.Placements {
			parentRef := placement.ParentRef
			if parentRef == "" {
				parentRef = "root"
			}
			queue = append(queue, queueItem{ref: parentRef, depth: current.depth + 1})
		}
	}
	if _, ok := depths["root"]; !ok {
		depths["root"] = 0
	}
	return depths
}
