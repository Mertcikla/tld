package diff_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"os"
	"path/filepath"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDiffCmd(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ExportFunc: func(_ *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			resp := &diagv1.ExportOrganizationResponse{
				Elements: []*diagv1.Element{
					{Id: 1, Name: "Server API", Kind: new("service"), UpdatedAt: timestamppb.Now()},
				},
			}
			return resp, nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	// Replace the local workspace with a different element set so diff compares current files.
	elementsYAML := "local-api:\n  name: Local API\n  kind: service\n  placements:\n    - parent: root\n"
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), []byte(elementsYAML), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "connectors.yaml")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	// Run diff
	// Note: since it calls 'git diff', it might fail in environments without git or if git is not configured.
	// But we expect it to at least run and return output if differences exist.
	_, _, err := cmd.RunCmd(t, dir, "diff")
	if err != nil {
		// git diff returns 1 if differences are found, which might be interpreted as error by Execute()
		// but our RunE returns nil even if diff found.
		t.Fatalf("diff: %v", err)
	}

	// The command succeeds if the current workspace files can be materialized for both sides.
}
