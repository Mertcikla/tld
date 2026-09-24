package pull_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

func TestPullCmd_PreserveRefs(t *testing.T) {
	// 1. Setup workspace with a custom ref
	dir := t.TempDir()

	// Set TLD_CONFIG_DIR to use our temp dir for tld.yaml
	t.Setenv("TLD_CONFIG_DIR", dir)

	// Create tld.yaml in the temp dir
	err := os.WriteFile(filepath.Join(dir, "tld.yaml"), []byte("server_url: http://localhost\napi_key: test\norg_id: test-org\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create elements.yaml with a custom ref "my-custom-ref" for element named "API Service".
	// ID "5nKqb4Xa" decodes to 22 in tld-cli hashids
	elementsYAML := `
my-custom-ref:
  name: API Service
  kind: service
_meta_elements:
  my-custom-ref:
    id: "5nKqb4Xa"
    updated_at: "2023-01-01T00:00:00Z"
`
	err = os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(elementsYAML), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Setup mock server that returns "API Service" with ID matching "5nKqb4Xa"
	svc := &cmd.MockDiagramService{
		ExportFunc: func(_ *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			resp := &diagv1.ExportOrganizationResponse{
				Elements: []*diagv1.Element{
					{
						Id:          22, // This matches 5nKqb4Xa
						Name:        "API Service Updated",
						Kind:        new("service"),
						UpdatedAt:   timestamppb.Now(),
						Description: new("New description"),
					},
				},
			}
			return resp, nil
		},
	}

	serverURL := cmd.NewMockServer(t, svc)
	// Update config with mock server URL
	err = os.WriteFile(filepath.Join(dir, "tld.yaml"), []byte("server_url: "+serverURL+"\napi_key: test\norg_id: test-org\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Run pull
	_, _, err = cmd.RunCmd(t, dir, "pull", "--force")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	// 4. Verify elements.yaml still uses "my-custom-ref"
	data, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if _, ok := result["my-custom-ref"]; !ok {
		t.Errorf("elements.yaml lost custom ref 'my-custom-ref'. Keys: %v", getMapKeys(result))
	}

	obj := result["my-custom-ref"].(map[string]any)
	if obj["name"] != "API Service Updated" {
		t.Errorf("Expected name 'API Service Updated', got %q", obj["name"])
	}
	if obj["description"] != "New description" {
		t.Errorf("Expected description 'New description', got %q", obj["description"])
	}
}

func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestPullLocalTarget(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()

	// Set TLD_CONFIG_DIR and TLD_DATA_DIR to use our temp dirs
	t.Setenv("TLD_CONFIG_DIR", dir)
	t.Setenv("TLD_DATA_DIR", dataDir)

	cmd.MustInitWorkspace(t, dir)

	// 1. Add some elements to the workspace (synchronous: writes to local
	// sqlite and refreshes the YAML cache immediately)
	cmd.MustRunCmd(t, dir, "add", "API Service", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db", "--kind", "database")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "queries")

	// 3. Remove YAML files to simulate missing local state
	elementsPath := filepath.Join(dir, ".tld", "elements.yaml")
	connectorsPath := filepath.Join(dir, ".tld", "connectors.yaml")
	if err := os.Remove(elementsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(connectorsPath); err != nil {
		t.Fatal(err)
	}

	// 4. Run pull --target local --data-dir dataDir
	stdout, _, err := cmd.RunCmd(t, dir, "pull", "--force", "--target", "local", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("pull local: %v\nstdout: %s", err, stdout)
	}

	// 5. Verify YAML files were materialized/restored correctly
	if _, err := os.Stat(elementsPath); os.IsNotExist(err) {
		t.Fatalf("elements.yaml was not materialized by pull")
	}
	if _, err := os.Stat(connectorsPath); os.IsNotExist(err) {
		t.Fatalf("connectors.yaml was not materialized by pull")
	}

	// Verify content of elements.yaml
	elemData, err := os.ReadFile(elementsPath)
	if err != nil {
		t.Fatal(err)
	}
	var elemResult map[string]any
	if err := yaml.Unmarshal(elemData, &elemResult); err != nil {
		t.Fatal(err)
	}
	if _, ok := elemResult["api"]; !ok {
		t.Errorf("elements.yaml missing 'api'. Keys: %v", getMapKeys(elemResult))
	}
	if _, ok := elemResult["db"]; !ok {
		t.Errorf("elements.yaml missing 'db'. Keys: %v", getMapKeys(elemResult))
	}

	// Verify content of connectors.yaml
	connData, err := os.ReadFile(connectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var connResult []map[string]any
	if err := yaml.Unmarshal(connData, &connResult); err != nil {
		t.Fatal(err)
	}
	foundConnector := false
	for _, conn := range connResult {
		if conn["source"] == "api" && conn["target"] == "db" && conn["label"] == "queries" {
			foundConnector = true
			break
		}
	}
	if !foundConnector {
		t.Errorf("connectors.yaml missing expected connector")
	}
}
