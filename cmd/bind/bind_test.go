package bind_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestBindCmdSetsFilePath(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Utilities", "--ref", "utilities", "--kind", "component")

	if _, _, err := cmd.RunCmd(t, dir, "bind", "utilities", "--file", "src/utils/**", "--symbol", "normalize"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	element := ws.Elements["utilities"]
	if element == nil {
		t.Fatal("utilities element missing")
	}
	if element.FilePath != "src/utils/**" || element.Symbol != "normalize" {
		t.Fatalf("binding = %q/%q, want src/utils/**/normalize", element.FilePath, element.Symbol)
	}
	if element.Kind != "component" || element.Name != "Utilities" {
		t.Fatalf("bind changed element identity: %+v", element)
	}
}

func TestBindCmdRequiresTarget(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Utilities", "--ref", "utilities", "--kind", "component")

	if _, _, err := cmd.RunCmd(t, dir, "bind", "utilities"); err == nil {
		t.Fatal("bind without --file/--symbol should fail")
	}
}

func TestBindCmdUnknownElement(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	if _, _, err := cmd.RunCmd(t, dir, "bind", "missing", "--file", "src/**"); err == nil {
		t.Fatal("bind on unknown element should fail")
	}
}

func TestAddCmdSetsFilePath(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Utilities", "--ref", "utilities", "--kind", "component", "--file", "src/utils/**")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Elements["utilities"].FilePath != "src/utils/**" {
		t.Fatalf("file path = %q, want src/utils/**", ws.Elements["utilities"].FilePath)
	}
}
