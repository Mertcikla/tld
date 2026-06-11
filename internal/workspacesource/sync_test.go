package workspacesource

import (
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestPlanImportUsesNumericConnectorRefAsExistingID(t *testing.T) {
	desired := &desiredWorkspace{
		Connectors: map[string]*desiredConnector{
			"9": {
				Ref:       "9",
				ViewRef:   "platform",
				SourceRef: "api",
				TargetRef: "db",
				Label:     "reads",
			},
		},
		ConnectorOrder: []string{"9"},
	}
	state := &sqliteState{
		connectorsByID: map[int32]*diagv1.Connector{
			9: {Id: 9},
		},
	}
	lock := &workspace.WorkspaceSourceLock{
		ManagedConnectors: map[string]*workspace.ResourceMetadata{},
	}

	plan := planImport(desired, state, lock)

	if plan.connectors.Created != 0 {
		t.Fatalf("created connectors = %d, want 0", plan.connectors.Created)
	}
	if plan.connectors.Updated != 1 {
		t.Fatalf("updated connectors = %d, want 1", plan.connectors.Updated)
	}
}
