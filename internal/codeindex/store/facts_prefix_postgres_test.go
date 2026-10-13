//go:build integration

package store

import "testing"

func TestPostgresFactsLiteralPathPrefix(t *testing.T) {
	testFactsLiteralPathPrefix(t, openPostgresTestStore(t))
}
