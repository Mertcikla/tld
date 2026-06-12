package mermaid

import "testing"

func TestParseCompatibleMermaidFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		source            string
		minElements       int
		minConnectors     int
		wantDirection     Direction
		allowWarnings     bool
		wantFirstRef      string
		wantFirstName     string
		wantFirstConnText string
	}{
		{
			name: "flowchart",
			source: `flowchart TB
  A[API] -->|reads| B[Database]`,
			minElements:       2,
			minConnectors:     1,
			wantDirection:     DirectionTB,
			wantFirstRef:      "A",
			wantFirstName:     "API",
			wantFirstConnText: "reads",
		},
		{
			name: "sequence",
			source: `sequenceDiagram
  participant User
  participant API
  User->>API: Request`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "Request",
		},
		{
			name: "class",
			source: `classDiagram
  class Order
  class Customer
  Customer --> Order : places`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "places",
		},
		{
			name: "er",
			source: `erDiagram
  CUSTOMER ||--o{ ORDER : places`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "places",
		},
		{
			name: "state",
			source: `stateDiagram-v2
  [*] --> Idle
  Idle --> Running: start`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "start",
		},
		{
			name: "requirement",
			source: `requirementDiagram
  requirement login {
    id: 1
    text: Users can log in
  }
  element api {
    type: service
  }
  api - satisfies -> login`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "satisfies",
		},
		{
			name: "architecture beta",
			source: `architecture-beta
  service api(server)[API]
  service db(database)[DB]
  api:R --> L:db: reads`,
			minElements:       2,
			minConnectors:     1,
			wantFirstConnText: "reads",
		},
		{
			name: "pie",
			source: `pie title Traffic
  "API" : 70
  "UI" : 30`,
			minElements: 2,
		},
		{
			name: "sankey",
			source: `sankey-beta
  API,DB,10`,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name: "timeline",
			source: `timeline
  title Releases
  2026 : Backend Mermaid`,
			minElements: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !tt.allowWarnings && len(parsed.Warnings) > 0 {
				t.Fatalf("Parse() warnings = %v", parsed.Warnings)
			}
			if len(parsed.Elements) < tt.minElements {
				t.Fatalf("Parse() elements = %d, want at least %d", len(parsed.Elements), tt.minElements)
			}
			if len(parsed.Connectors) < tt.minConnectors {
				t.Fatalf("Parse() connectors = %d, want at least %d", len(parsed.Connectors), tt.minConnectors)
			}
			if tt.wantDirection != "" && parsed.Direction != tt.wantDirection {
				t.Fatalf("Parse() direction = %q, want %q", parsed.Direction, tt.wantDirection)
			}
			if tt.wantFirstRef != "" && parsed.Elements[0].GetRef() != tt.wantFirstRef {
				t.Fatalf("first ref = %q, want %q", parsed.Elements[0].GetRef(), tt.wantFirstRef)
			}
			if tt.wantFirstName != "" && parsed.Elements[0].GetName() != tt.wantFirstName {
				t.Fatalf("first name = %q, want %q", parsed.Elements[0].GetName(), tt.wantFirstName)
			}
			if tt.wantFirstConnText != "" && !hasConnectorText(parsed, tt.wantFirstConnText) {
				t.Fatalf("connector label/relationship does not contain %q: %v", tt.wantFirstConnText, connectorTexts(parsed))
			}
		})
	}
}

func hasConnectorText(parsed *ParsedDiagram, text string) bool {
	for _, connector := range parsed.Connectors {
		if connector.GetLabel() == text || connector.GetRelationship() == text {
			return true
		}
	}
	return false
}

func connectorTexts(parsed *ParsedDiagram) []string {
	out := make([]string, 0, len(parsed.Connectors))
	for _, connector := range parsed.Connectors {
		out = append(out, connector.GetLabel()+"/"+connector.GetRelationship())
	}
	return out
}

func TestParseAppliesTldMetadataRefs(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart LR
%% tld/v1 view=42
  A["Imported Auth"]
%% tld-element ref=node_57 kind=service x=120 y=80 tags=backend
  B["Database"]
%% tld-element ref=B
  A --> B
%% tld-connector ref=99 source=node_57 target=B rel=reads`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}
	if got := parsed.Elements[0].GetRef(); got != "node_57" {
		t.Fatalf("metadata ref = %q, want node_57", got)
	}
	if got := parsed.Connectors[0].GetSourceElementRef(); got != "node_57" {
		t.Fatalf("connector source ref = %q, want node_57", got)
	}
	if got := parsed.Connectors[0].GetRelationship(); got != "reads" {
		t.Fatalf("connector relationship = %q, want reads", got)
	}
}

func TestParseLimits(t *testing.T) {
	t.Parallel()

	oversized := make([]byte, MaxSourceBytes+1)
	for index := range oversized {
		oversized[index] = 'x'
	}
	if _, err := Parse(string(oversized)); err == nil {
		t.Fatal("Parse(oversized) error = nil")
	}
}
