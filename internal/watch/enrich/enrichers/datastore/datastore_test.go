package datastore

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestDatastoreGlue(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "datastore glue matches redis mention",
		Enricher: DatastoreGlue(),
		Input: enrich.FileInput{
			RelPath:  "cache.go",
			Language: "go",
			Source:   []byte(`func connect() { _ = "redis://cache:6379" }`),
		},
		Want: enrichertest.Fact{Type: "datastore.dependency", Tag: "datastore:redis", Name: "redis"},
	})
}
