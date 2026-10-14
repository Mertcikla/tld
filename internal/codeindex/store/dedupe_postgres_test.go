//go:build integration

package store

import "testing"

func TestPostgresExactRowReuse(t *testing.T) {
	testExactRowReuse(t, openPostgresTestStore(t))
}

func TestPostgresExactEdgeReusePreservesRepeatedObservations(t *testing.T) {
	testExactEdgeReusePreservesRepeatedObservations(t, openPostgresTestStore(t))
}

func TestPostgresRowReuseUpgradePreservesSnapshots(t *testing.T) {
	testRowReuseUpgradePreservesSnapshots(t, openPostgresTestStore(t), "migrations/postgres")
}

func TestPostgresPackageVersionReuse(t *testing.T) {
	testPackageVersionReuse(t, openPostgresTestStore(t))
}
