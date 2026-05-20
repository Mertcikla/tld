package jobs

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestJobEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "src/jobs",
				Language: spec.Languages[0],
				Source:   []byte(spec.SourceTokens[0]),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}},
			Want:    enrichertest.Fact{Type: spec.FactType, Tag: "category:jobs", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
		})
	}
}

func TestConcreteBindingsExtractSchedules(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "typescript cron",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "jobs.ts",
				Language: "typescript",
				Source:   []byte(`cron.schedule("0 0 * * *", runDaily)`),
			},
			Want: enrichertest.Fact{Type: "job.schedule", Name: "0 0 * * *", Attribute: "schedule", AttrValue: "0 0 * * *"},
		},
		enrichertest.Case{
			Name:     "java scheduled",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "Job.java",
				Language: "java",
				Source:   []byte(`@Scheduled(cron = "0 0 * * * *") void run() {}`),
			},
			Want: enrichertest.Fact{Type: "job.schedule", Name: "0 0 * * * *", Attribute: "schedule", AttrValue: "0 0 * * * *"},
		},
	)
}

func enrichersByID() map[string]enrich.Enricher {
	out := map[string]enrich.Enricher{}
	for _, enricher := range All() {
		out[enricher.Metadata().ID] = enricher
	}
	return out
}
