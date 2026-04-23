package analyze

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/internal/analyzer"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestEnsureAnalyzeElement_ReusesIdentityWithinRun(t *testing.T) {
	dir := t.TempDir()
	ws := &workspace.Workspace{
		Dir:      dir,
		Elements: map[string]*workspace.Element{},
	}
	known := buildAnalyzeElementIndex(ws)
	usedRefs := map[string]struct{}{}
	usedNames := buildAnalyzeElementNameOwners(ws)
	spec := analyzeElementSpec{
		Name:      "alias_for_table",
		Kind:      "function",
		Owner:     "digitaltwin-poc",
		Repo:      "git@gitlab.btsgrp.com:ridvan.zengin/dtwin.git",
		Branch:    "main",
		FilePath:  filepath.Clean("backhaul_analysis/backhaul_analysis.py"),
		Symbol:    "alias_for_table",
		ParentRef: "backhaul-analysis-py",
		Identity: analyzeElementIdentity{
			Repo:     "git@gitlab.btsgrp.com:ridvan.zengin/dtwin.git",
			Branch:   "main",
			FilePath: "backhaul_analysis/backhaul_analysis.py",
			Symbol:   "alias_for_table",
			Kind:     "function",
			Name:     "alias_for_table",
		},
	}

	firstRef, err := ensureAnalyzeElement(dir, false, ws, known, usedRefs, usedNames, spec)
	if err != nil {
		t.Fatalf("first ensureAnalyzeElement: %v", err)
	}
	secondRef, err := ensureAnalyzeElement(dir, false, ws, known, usedRefs, usedNames, spec)
	if err != nil {
		t.Fatalf("second ensureAnalyzeElement: %v", err)
	}
	if firstRef != secondRef {
		t.Fatalf("refs differ: first=%q second=%q", firstRef, secondRef)
	}
	if len(ws.Elements) != 1 {
		t.Fatalf("elements = %d, want 1", len(ws.Elements))
	}
	if _, ok := ws.Elements[firstRef]; !ok {
		t.Fatalf("missing expected ref %q in ws.Elements", firstRef)
	}
}

func TestEnsureAnalyzeElement_DoesNotReuseNameAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	ws := &workspace.Workspace{
		Dir:      dir,
		Elements: map[string]*workspace.Element{},
	}
	known := buildAnalyzeElementIndex(ws)
	usedRefs := map[string]struct{}{}
	usedNames := buildAnalyzeElementNameOwners(ws)

	firstSpec := analyzeElementSpec{
		Name:      "Load",
		Kind:      "function",
		Owner:     "tld",
		Repo:      "git@github.com:Mertcikla/tld-cli.git",
		Branch:    "main",
		FilePath:  "workspace/loader.go",
		Symbol:    "Load",
		ParentRef: "loader-go",
		Identity: analyzeElementIdentity{
			Repo:     "git@github.com:Mertcikla/tld-cli.git",
			Branch:   "main",
			FilePath: "workspace/loader.go",
			Symbol:   "Load",
			Kind:     "function",
			Name:     "Load",
		},
	}
	secondSpec := firstSpec
	secondSpec.FilePath = "planner/loader.go"
	secondSpec.ParentRef = "planner-loader-go"
	secondSpec.Identity.FilePath = "planner/loader.go"

	firstRef, err := ensureAnalyzeElement(dir, false, ws, known, usedRefs, usedNames, firstSpec)
	if err != nil {
		t.Fatalf("first ensureAnalyzeElement: %v", err)
	}
	secondRef, err := ensureAnalyzeElement(dir, false, ws, known, usedRefs, usedNames, secondSpec)
	if err != nil {
		t.Fatalf("second ensureAnalyzeElement: %v", err)
	}
	if firstRef == secondRef {
		t.Fatalf("expected distinct refs for duplicate symbol names, got %q", firstRef)
	}
	if len(ws.Elements) != 2 {
		t.Fatalf("elements = %d, want 2", len(ws.Elements))
	}
	if ws.Elements[firstRef].Name == ws.Elements[secondRef].Name {
		t.Fatalf("expected unique element names, both were %q", ws.Elements[firstRef].Name)
	}
}

func TestResolveAnalyzeTargetRef_UsesDefinitionLocation(t *testing.T) {
	symbols := []analyzer.Symbol{
		{Name: "Load", Kind: "function", FilePath: "cmd/loader.go", Line: 10, EndLine: 20},
		{Name: "Load", Kind: "function", FilePath: "workspace/loader.go", Line: 30, EndLine: 40},
	}
	refBySymbol := map[analyzeElementLookupKey]string{
		analyzeSymbolLookupKey(symbols[0]): "cmd-load",
		analyzeSymbolLookupKey(symbols[1]): "workspace-load",
	}
	refsByName := map[string][]string{"Load": {"cmd-load", "workspace-load"}}

	resolved := resolveAnalyzeTargetRef(context.Background(), fakeAnalyzeDefinitionResolver{
		locations: []analyzeDefinitionLocation{{FilePath: "workspace/loader.go", Line: 30}},
	}, analyzer.Ref{Name: "Load", FilePath: "cmd/analyze.go", Line: 5, Column: 12}, symbols, refBySymbol, refsByName)

	if resolved != "workspace-load" {
		t.Fatalf("resolved ref = %q, want workspace-load", resolved)
	}
}

func TestResolveAnalyzeTargetRef_DropsAmbiguousFallback(t *testing.T) {
	symbols := []analyzer.Symbol{
		{Name: "Load", Kind: "function", FilePath: "cmd/loader.go", Line: 10, EndLine: 20},
		{Name: "Load", Kind: "function", FilePath: "workspace/loader.go", Line: 30, EndLine: 40},
	}
	refBySymbol := map[analyzeElementLookupKey]string{
		analyzeSymbolLookupKey(symbols[0]): "cmd-load",
		analyzeSymbolLookupKey(symbols[1]): "workspace-load",
	}
	refsByName := map[string][]string{"Load": {"cmd-load", "workspace-load"}}

	resolved := resolveAnalyzeTargetRef(context.Background(), nil, analyzer.Ref{Name: "Load", FilePath: "cmd/analyze.go", Line: 5}, symbols, refBySymbol, refsByName)
	if resolved != "" {
		t.Fatalf("resolved ref = %q, want empty", resolved)
	}
}

type fakeAnalyzeDefinitionResolver struct {
	locations []analyzeDefinitionLocation
	err       error
}

func (r fakeAnalyzeDefinitionResolver) ResolveDefinitions(context.Context, analyzer.Ref) ([]analyzeDefinitionLocation, error) {
	return r.locations, r.err
}

func (r fakeAnalyzeDefinitionResolver) Close() error {
	return nil
}
