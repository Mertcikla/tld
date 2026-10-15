//go:build integration

package store

import "testing"

func TestPostgresMembershipGroupsKeepIndependentSnapshots(t *testing.T) {
	testMembershipGroups(t, openPostgresTestStore(t))
}
