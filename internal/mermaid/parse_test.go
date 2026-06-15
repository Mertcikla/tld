package mermaid

import (
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

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

func elementByRef(parsed *ParsedDiagram, ref string) *diagv1.PlanElement {
	for _, element := range parsed.Elements {
		if element.GetRef() == ref {
			return element
		}
	}
	return nil
}

func TestParseFlowchartQuotedLabelsAndInlineTargetDefinition(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart LR
  A["Checkout API Mutation"]
  B["Checkout Orchestrator"]

  C["manager"]
  A -- "Submits order" --> B
  A --> D["asd"]
  C --> D`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}

	wantNames := map[string]string{
		"A": "Checkout API Mutation",
		"B": "Checkout Orchestrator",
		"C": "manager",
		"D": "asd",
	}
	for _, element := range parsed.Elements {
		if want, ok := wantNames[element.GetRef()]; ok {
			if element.GetName() != want {
				t.Fatalf("element %s name = %q, want %q", element.GetRef(), element.GetName(), want)
			}
			delete(wantNames, element.GetRef())
		}
	}
	if len(wantNames) > 0 {
		t.Fatalf("missing elements: %v", wantNames)
	}
	if !hasConnectorText(parsed, "Submits order") {
		t.Fatalf("connector label/relationship does not contain %q: %v", "Submits order", connectorTexts(parsed))
	}
}

func TestParseFlowchartNumericIDsWithQuotedEdgeLabels(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart LR
  125["API Gateway"]
  126["Auth Service"]
  127["CDN"]
  128["User"]
  129["Web App"]

  125 --> 126
  129 -- "API calls" --> 125
  129 -- "Auth" --> 126
  129 -- "Serves via" --> 127
  128 -- "Uses" --> 129`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}
	if len(parsed.Elements) != 5 {
		t.Fatalf("Parse() elements = %d, want 5", len(parsed.Elements))
	}
	if len(parsed.Connectors) != 5 {
		t.Fatalf("Parse() connectors = %d, want 5", len(parsed.Connectors))
	}

	wantNames := map[string]string{
		"125": "API Gateway",
		"126": "Auth Service",
		"127": "CDN",
		"128": "User",
		"129": "Web App",
	}
	for _, element := range parsed.Elements {
		want, ok := wantNames[element.GetRef()]
		if !ok {
			t.Fatalf("unexpected element ref %q", element.GetRef())
		}
		if element.GetName() != want {
			t.Fatalf("element %s name = %q, want %q", element.GetRef(), element.GetName(), want)
		}
		delete(wantNames, element.GetRef())
	}
	if len(wantNames) > 0 {
		t.Fatalf("missing elements: %v", wantNames)
	}
	for _, label := range []string{"API calls", "Auth", "Serves via", "Uses"} {
		if !hasConnectorText(parsed, label) {
			t.Fatalf("connector label/relationship does not contain %q: %v", label, connectorTexts(parsed))
		}
	}
}

func TestParseFlowchartFontAwesomeNodeLabels(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart TD
    A[Christmas] -->|Get money| B(Go shopping)
    B --> C{Let me think}
    C -->|One| D[Laptop]
    C -->|Two| E[iPhone]
    C -->|Three| F[fa:fa-car Car]`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}

	element := elementByRef(parsed, "F")
	if element == nil {
		t.Fatal("missing element F")
	}
	if element.GetName() != "Car" {
		t.Fatalf("element F name = %q, want Car", element.GetName())
	}
	links := element.GetTechnologyLinks()
	if len(links) != 1 {
		t.Fatalf("element F technology links = %d, want 1", len(links))
	}
	if links[0].GetType() != "custom" || links[0].GetLabel() != "fa:fa-car" || !links[0].GetIsPrimaryIcon() {
		t.Fatalf("element F technology link = %+v, want primary custom fa:fa-car", links[0])
	}
	if element.GetTechnology() != "" {
		t.Fatalf("element F technology = %q, want empty", element.GetTechnology())
	}
	if element.GetLogoUrl() != "" {
		t.Fatalf("element F logo url = %q, want empty", element.GetLogoUrl())
	}
}

func TestParseFlowchartFontAwesomeVariantsAndFallbackName(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart LR
  A[fa:car]
  B[fas:fa-shopping-cart Cart]
  C[Some fa:fa-car Text]
  Root --> D
  D[fa:fa-server Server]`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}

	elementA := elementByRef(parsed, "A")
	if elementA == nil {
		t.Fatal("missing element A")
	}
	if elementA.GetName() != "Car" || len(elementA.GetTechnologyLinks()) != 1 || elementA.GetTechnologyLinks()[0].GetLabel() != "fa:fa-car" {
		t.Fatalf("element A = name %q links %+v, want fallback Car with fa:fa-car", elementA.GetName(), elementA.GetTechnologyLinks())
	}

	elementB := elementByRef(parsed, "B")
	if elementB == nil {
		t.Fatal("missing element B")
	}
	if elementB.GetName() != "Cart" || len(elementB.GetTechnologyLinks()) != 1 || elementB.GetTechnologyLinks()[0].GetLabel() != "fa:fa-shopping-cart" {
		t.Fatalf("element B = name %q links %+v, want Cart with fa:fa-shopping-cart", elementB.GetName(), elementB.GetTechnologyLinks())
	}

	elementC := elementByRef(parsed, "C")
	if elementC == nil {
		t.Fatal("missing element C")
	}
	if elementC.GetName() != "Some fa:fa-car Text" || len(elementC.GetTechnologyLinks()) != 0 {
		t.Fatalf("element C = name %q links %+v, want unchanged label without links", elementC.GetName(), elementC.GetTechnologyLinks())
	}

	elementD := elementByRef(parsed, "D")
	if elementD == nil {
		t.Fatal("missing element D")
	}
	if elementD.GetName() != "Server" || len(elementD.GetTechnologyLinks()) != 1 || elementD.GetTechnologyLinks()[0].GetLabel() != "fa:fa-server" {
		t.Fatalf("element D = name %q links %+v, want duplicate ref update to Server with fa:fa-server", elementD.GetName(), elementD.GetTechnologyLinks())
	}
}

func TestParseFlowchartFontAwesomeMetadataTechLinksOverride(t *testing.T) {
	t.Parallel()

	parsed, err := Parse(`flowchart LR
%% tld/v1 view=42
  F["fa:fa-car Car"]
%% tld-element ref=F techLinks=catalog:go:Go:1`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse() warnings = %v", parsed.Warnings)
	}

	element := elementByRef(parsed, "F")
	if element == nil {
		t.Fatal("missing element F")
	}
	links := element.GetTechnologyLinks()
	if len(links) != 1 {
		t.Fatalf("element F technology links = %d, want 1", len(links))
	}
	if links[0].GetType() != "catalog" || links[0].GetSlug() != "go" || links[0].GetLabel() != "Go" || !links[0].GetIsPrimaryIcon() {
		t.Fatalf("element F technology link = %+v, want metadata catalog Go override", links[0])
	}
}

func TestParseFlowchartHeaderNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		source        string
		wantDirection Direction
		minElements   int
		minConnectors int
	}{
		{
			name:          "semicolon header with inline statements",
			source:        `graph TD;A-->B;B-->C;`,
			wantDirection: DirectionTD,
			minElements:   3,
			minConnectors: 2,
		},
		{
			name: "comment before semicolon header",
			source: `%% comment
graph TD;
  A-->B;`,
			wantDirection: DirectionTD,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name:          "symbol direction right",
			source:        `graph >;A-->B;`,
			wantDirection: DirectionLR,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name:          "symbol direction left",
			source:        `graph <;A-->B;`,
			wantDirection: DirectionRL,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name:          "symbol direction up",
			source:        `graph ^;A-->B;`,
			wantDirection: DirectionBT,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name:          "lowercase down direction",
			source:        `graph v;A-->B;`,
			wantDirection: DirectionTB,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name: "bare flowchart header",
			source: `flowchart
A-->B;`,
			wantDirection: DirectionTD,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name:          "swimlane alias",
			source:        `swimlane LR;A-->B;`,
			wantDirection: DirectionLR,
			minElements:   2,
			minConnectors: 1,
		},
		{
			name: "semicolon subgraph terminator",
			source: `graph TD
subgraph myTitle
c-->d
end;`,
			wantDirection: DirectionTD,
			minElements:   2,
			minConnectors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(parsed.Warnings) > 0 {
				t.Fatalf("Parse() warnings = %v", parsed.Warnings)
			}
			if parsed.Direction != tt.wantDirection {
				t.Fatalf("Parse() direction = %q, want %q", parsed.Direction, tt.wantDirection)
			}
			if len(parsed.Elements) < tt.minElements {
				t.Fatalf("Parse() elements = %d, want at least %d", len(parsed.Elements), tt.minElements)
			}
			if len(parsed.Connectors) < tt.minConnectors {
				t.Fatalf("Parse() connectors = %d, want at least %d", len(parsed.Connectors), tt.minConnectors)
			}
		})
	}
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
