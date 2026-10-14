package workspace

import (
	"strings"
	"testing"
)

func TestCheckElementRemovalRefusesSharedTargetAliases(t *testing.T) {
	ws := &Workspace{
		Elements: map[string]*Element{"original": {Name: "Shared"}, "alias": {Name: "Shared"}},
		Meta:     &Meta{Elements: map[string]*ResourceMetadata{"original": {ID: 42}, "alias": {ID: 42}}},
	}
	if err := CheckElementRemoval(ws, "alias"); err == nil || !strings.Contains(err.Error(), "shares target ID") {
		t.Fatalf("deleting an alias must not remove the shared target: %v", err)
	}
}
