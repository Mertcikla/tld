package inspect_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/cmd"
)

func setupInspectWorkspace(t *testing.T, dir string) {
	t.Helper()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")
}

func TestInspectElementShowsDerivedChildrenAndRelatedConnectors(t *testing.T) {
	dir := t.TempDir()
	setupInspectWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "inspect", "platform")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	for _, want := range []string{
		"Resource:",
		"element platform",
		"Has view:",
		"true",
		"Derived children:",
		"api, db",
		"In owned view:",
		"platform:api:db:reads",
		"local_db:",
		"no metadata id",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, stdout)
		}
	}
}

func TestInspectAmbiguousRefRequiresType(t *testing.T) {
	dir := t.TempDir()
	setupInspectWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Ambiguous", "--ref", "platform:api:db:reads", "--kind", "service")

	stdout, _, err := cmd.RunCmd(t, dir, "inspect", "platform:api:db:reads")
	if err != nil {
		t.Fatalf("inspect ambiguous: %v", err)
	}
	if !strings.Contains(stdout, "ambiguous ref") || !strings.Contains(stdout, "element, connector") || !strings.Contains(stdout, "--type") {
		t.Fatalf("ambiguous output unexpected:\n%s", stdout)
	}
}

func TestInspectJSONIncludesSourceStates(t *testing.T) {
	dir := t.TempDir()
	setupInspectWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "--format", "json", "inspect", "platform")
	if err != nil {
		t.Fatalf("inspect json: %v", err)
	}
	var payload struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Type    string `json:"type"`
		Element struct {
			Children []string `json:"derived_children"`
			HasView  bool     `json:"has_view"`
		} `json:"element"`
		Sources []struct {
			Source  string `json:"source"`
			Present bool   `json:"present"`
			Note    string `json:"note"`
		} `json:"sources"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse inspect json: %v\n%s", err, stdout)
	}
	if payload.Command != "inspect" || payload.Status != "ok" || payload.Type != "element" {
		t.Fatalf("unexpected json header: %+v", payload)
	}
	if !payload.Element.HasView || strings.Join(payload.Element.Children, ",") != "api,db" {
		t.Fatalf("unexpected element json: %+v", payload.Element)
	}
	if len(payload.Sources) != 2 || payload.Sources[0].Source != "yaml" || payload.Sources[1].Source != "local_db" {
		t.Fatalf("unexpected sources: %+v", payload.Sources)
	}
}

func TestInspectLocalDBUsesDataDir(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	setupInspectWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)

	stdout, _, err := cmd.RunCmd(t, dir, "inspect", "api", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("inspect local db: %v", err)
	}
	if !strings.Contains(stdout, "local_db:") || !strings.Contains(stdout, "present") || !strings.Contains(stdout, "id=") {
		t.Fatalf("local DB state missing:\n%s", stdout)
	}
}

func TestInspectCloudUsesExportWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	svc := &cmd.MockDiagramService{
		ExportFunc: func(req *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			if req.OrgId != cmd.TestWorkspaceID {
				t.Fatalf("org id = %q, want %q", req.OrgId, cmd.TestWorkspaceID)
			}
			kind := "service"
			return &diagv1.ExportOrganizationResponse{
				Elements: []*diagv1.Element{{Id: 42, Name: "API", Kind: &kind}},
			}, nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)
	cmd.WriteConfig(t, dir, serverURL, "test-key")

	stdout, _, err := cmd.RunCmd(t, dir, "inspect", "api", "--cloud")
	if err != nil {
		t.Fatalf("inspect cloud: %v", err)
	}
	if !strings.Contains(stdout, "cloud:") || !strings.Contains(stdout, "present") || !strings.Contains(stdout, "id=42") {
		t.Fatalf("cloud state missing:\n%s", stdout)
	}
}

func TestInspectCloudUnauthorizedIncludesHint(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	svc := &cmd.MockDiagramService{
		ExportFunc: func(*diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("expired"))
		},
	}
	serverURL := cmd.NewMockServer(t, svc)
	cmd.WriteConfig(t, dir, serverURL, "test-key")

	stdout, _, err := cmd.RunCmd(t, dir, "inspect", "api", "--cloud")
	if err != nil {
		t.Fatalf("inspect cloud unauthorized: %v", err)
	}
	if !strings.Contains(stdout, "cloud:") || !strings.Contains(stdout, "Run 'tld login'") {
		t.Fatalf("unauthorized hint missing:\n%s", stdout)
	}
}
