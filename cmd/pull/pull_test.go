package pull_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"os"
	"path/filepath"
	"testing"

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
