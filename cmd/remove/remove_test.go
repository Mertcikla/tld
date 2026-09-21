package remove_test

import (
	"fmt"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRemoveElementCmd_LocalOnly(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "element", "api")
	if err != nil {
		t.Fatalf("remove element: %v", err)
	}
	if !strings.Contains(stdout, "del: api") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}
}

func TestRemoveElementCmd_DryRunDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "element", "api", "--dry-run")
	if err != nil {
		t.Fatalf("remove element --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run: del: api") {
		t.Fatalf("missing dry-run output: %s", stdout)
	}

	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Elements["api"] == nil {
		t.Fatal("api should not be deleted during dry-run")
	}
}

func TestRemoveElementCmd_ReferencedElementFails(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	_, _, err := cmd.RunCmd(t, dir, "remove", "element", "platform")
	if err == nil {
		t.Fatal("expected referenced element removal to fail")
	}
	msg := err.Error()
	for _, want := range []string{
		`element "platform" is still referenced`,
		"elements.yaml[api].placements[0].parent",
		"connectors.yaml[platform:api:db:reads].view",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Elements["platform"] == nil {
		t.Fatal("platform should not have been deleted")
	}
}

func TestRemoveConnectorCmd(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace", "--with-view")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("remove connector: %v", err)
	}
	if !strings.Contains(stdout, "del: 1") {
		t.Errorf("stdout %q does not contain success message", stdout)
	}
}

func TestRemoveConnectorCmd_DryRunDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database", "--parent", "platform")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db", "--dry-run")
	if err != nil {
		t.Fatalf("remove connector --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run: del: 1") {
		t.Fatalf("missing dry-run delete count: %s", stdout)
	}

	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Connectors["platform:api:db:reads"] == nil {
		t.Fatal("connector should remain after dry-run")
	}
}

func TestRemoveConnectorCmd_AmbiguousRequiresLabel(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")
	cmd.MustRunCmd(t, dir, "connect", "--view", "platform", "--from", "api", "--to", "db", "--label", "writes")

	_, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db")
	if err == nil {
		t.Fatal("expected ambiguous connector removal to fail")
	}
	if !strings.Contains(err.Error(), "multiple connectors match") || !strings.Contains(err.Error(), "platform:api:db:reads") || !strings.Contains(err.Error(), "platform:api:db:writes") {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db", "--label", "reads")
	if err != nil {
		t.Fatalf("remove connector with label: %v", err)
	}
	if !strings.Contains(stdout, "del: 1") {
		t.Fatalf("missing delete count: %s", stdout)
	}
	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Connectors["platform:api:db:reads"] != nil || ws.Connectors["platform:api:db:writes"] == nil {
		t.Fatalf("unexpected connectors after delete: %+v", ws.Connectors)
	}
}

// TestRemoveElementCmd_ServerFailureKeepsYamlForRetry verifies the
// server-first ordering: when the server delete fails, the YAML cache and its
// cached IDs are left intact so the removal can be retried.
func TestRemoveElementCmd_ServerFailureKeepsYamlForRetry(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)

	cmd.MustRunCmd(t, dir, "add", "Cache", "--ref", "cache", "--kind", "database")
	if got := svc.ElementCount(); got != 1 {
		t.Fatalf("server element count = %d, want 1", got)
	}

	svc.DeleteElementFunc = func(*diagv1.DeleteElementRequest) (*diagv1.DeleteElementResponse, error) {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("server unavailable"))
	}
	_, _, err := cmd.RunCmd(t, dir, "remove", "element", "cache")
	if err == nil || !strings.Contains(err.Error(), "server delete element failed") {
		t.Fatalf("expected server delete failure, got: %v", err)
	}
	ws, loadErr := workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if ws.Elements["cache"] == nil {
		t.Fatal("cache YAML entry should be retained after a server failure")
	}
	if meta := ws.Meta.Elements["cache"]; meta == nil || meta.ID == 0 {
		t.Fatal("cache server ID should be retained after a server failure")
	}
	if got := svc.ElementCount(); got != 1 {
		t.Fatalf("server element count = %d, want 1", got)
	}

	// Retry once the server is reachable: it must converge.
	svc.DeleteElementFunc = nil
	stdout, _, err := cmd.RunCmd(t, dir, "remove", "element", "cache")
	if err != nil {
		t.Fatalf("retry remove element: %v", err)
	}
	if !strings.Contains(stdout, "del: cache") {
		t.Fatalf("stdout = %q, want delete confirmation", stdout)
	}
	ws, loadErr = workspace.Load(dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if _, ok := ws.Elements["cache"]; ok {
		t.Fatalf("cache still present after remove: %+v", ws.Elements)
	}
	if got := svc.ElementCount(); got != 0 {
		t.Fatalf("server element count = %d, want 0", got)
	}
}
