package messaging

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestMessagingEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "src/messaging",
				Language: spec.Languages[0],
				Source:   []byte(spec.SourceTokens[0]),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}},
			Want:    enrichertest.Fact{Type: spec.FactType, Tag: "category:messaging", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
		})
	}
}

func TestConcreteBindingsExtractDestinations(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "go topic",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "producer.go",
				Language: "go",
				Source:   []byte(`writer := kafka.Writer{Topic: "orders.created"}`),
			},
			Want: enrichertest.Fact{Type: "messaging.topic", Name: "orders.created", Attribute: "name", AttrValue: "orders.created"},
		},
		enrichertest.Case{
			Name:     "typescript queue",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "worker.ts",
				Language: "typescript",
				Source:   []byte(`const queue = new Queue("email.send")`),
			},
			Want: enrichertest.Fact{Type: "messaging.queue", Name: "email.send", Attribute: "name", AttrValue: "email.send"},
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
