package tech

import "testing"

func TestValidateAcceptsContainerAsDockerAlias(t *testing.T) {
	if missing := Validate("Container"); len(missing) != 0 {
		t.Fatalf("Validate(%q) missing = %v, want none", "Container", missing)
	}
}
