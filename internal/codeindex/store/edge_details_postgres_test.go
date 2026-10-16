//go:build integration

package store

import "testing"

func TestPostgresCompactEdgeQueries(t *testing.T) {
	testCompactEdgeQueries(t, openPostgresTestStore(t))
}
