package enrich

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	enrichers []Enricher
}

func NewRegistry(enrichers ...Enricher) *Registry {
	r := &Registry{}
	for _, enricher := range enrichers {
		r.Register(enricher)
	}
	return r
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(
		DependencyInventory(),
		GoNetHTTP(),
		GoChi(),
		GoGin(),
		TSExpress(),
		TSNextJS(),
		TSReactRouter(),
		TSPrisma(),
	)
}

func (r *Registry) Register(enricher Enricher) {
	if r == nil || enricher == nil {
		return
	}
	r.enrichers = append(r.enrichers, enricher)
	sort.SliceStable(r.enrichers, func(i, j int) bool {
		return r.enrichers[i].Metadata().ID < r.enrichers[j].Metadata().ID
	})
}

func (r *Registry) EnrichFile(ctx context.Context, input FileInput) ([]Fact, []Warning, error) {
	if r == nil {
		return nil, nil, nil
	}
	var facts []Fact
	var warnings []Warning
	for _, enricher := range r.enrichers {
		meta := enricher.Metadata()
		if strings.TrimSpace(meta.ID) == "" {
			continue
		}
		if !r.active(meta, input.Signals) || !enricher.MatchFile(input) {
			continue
		}
		emitter := &collector{enricher: meta.ID}
		if err := enricher.EnrichFile(ctx, input, emitter); err != nil {
			return nil, nil, fmt.Errorf("%s enrich %s: %w", meta.ID, input.RelPath, err)
		}
		facts = append(facts, emitter.facts...)
		warnings = append(warnings, emitter.warnings...)
	}
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].Enricher == facts[j].Enricher {
			return facts[i].StableKey < facts[j].StableKey
		}
		return facts[i].Enricher < facts[j].Enricher
	})
	return facts, warnings, nil
}

func (r *Registry) active(meta Metadata, signals []ActivationSignal) bool {
	switch meta.Mode {
	case "", ActivationAlways:
		return true
	case ActivationImportOrDependency:
		for _, trigger := range meta.Triggers {
			for _, signal := range signals {
				if signalMatches(trigger, signal) {
					return true
				}
			}
		}
	}
	return false
}

func signalMatches(trigger, signal ActivationSignal) bool {
	if trigger.Kind != "" && trigger.Kind != signal.Kind {
		return false
	}
	triggerValue := strings.TrimSpace(trigger.Value)
	signalValue := strings.TrimSpace(signal.Value)
	if triggerValue == "" || signalValue == "" {
		return false
	}
	return signalValue == triggerValue || strings.HasPrefix(signalValue, triggerValue+"/")
}

type collector struct {
	enricher string
	facts    []Fact
	warnings []Warning
}

func (c *collector) EmitFact(fact Fact) error {
	fact.Enricher = strings.TrimSpace(fact.Enricher)
	if fact.Enricher == "" {
		fact.Enricher = c.enricher
	}
	fact.Type = strings.TrimSpace(fact.Type)
	fact.StableKey = strings.TrimSpace(fact.StableKey)
	if fact.Type == "" {
		return fmt.Errorf("fact type is required")
	}
	if fact.StableKey == "" {
		return fmt.Errorf("fact stable key is required")
	}
	if fact.Confidence <= 0 {
		fact.Confidence = 1
	}
	if fact.Attributes == nil {
		fact.Attributes = map[string]string{}
	}
	fact.Tags = normalizeTags(fact.Tags)
	c.facts = append(c.facts, fact)
	return nil
}

func (c *collector) Warn(warning Warning) {
	if warning.Enricher == "" {
		warning.Enricher = c.enricher
	}
	c.warnings = append(c.warnings, warning)
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
