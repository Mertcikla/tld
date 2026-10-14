//go:build integration

package store

import "testing"

func TestPostgresBlobCompression(t *testing.T) {
	testBlobCompression(t, openPostgresTestStore(t))
}

func TestPostgresBlobCompressionUpgrade(t *testing.T) {
	testBlobCompressionUpgrade(t, openPostgresTestStore(t))
}
