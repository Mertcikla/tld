package compose

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func triggerSignals() []enrich.ActivationSignal {
	return []enrich.ActivationSignal{
		{Kind: enrich.SignalDependency, Value: "docker-compose"},
	}
}

func TestComposeServiceWithImage(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose service with image",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  web:
    image: nginx:latest
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Tag: "runtime:compose", Name: "web", Attribute: "image", AttrValue: "nginx:latest"},
	})
}

func TestComposeServiceImageTechnology(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose service with inferred technology",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  db:
    image: postgres:15
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Tag: "kind:database", Name: "db", Attribute: "technology", AttrValue: "PostgreSQL"},
	})
}

func TestComposeDependsOn(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose depends_on connection",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  web:
    image: nginx:latest
    depends_on:
      - db
  db:
    image: postgres:15
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.connection", Name: "web -> db", Attribute: "source", AttrValue: "web"},
	})
}

func TestComposeEnvConnection(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose env references another service",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  web:
    image: nginx:latest
    environment:
      - DATABASE_URL=http://db:5432
  db:
    image: postgres:15
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.connection", Tag: "compose:implicit", Attribute: "source", AttrValue: "web"},
	})
}

func TestComposeBuildContext(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose service with build context",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  app:
    build:
      context: ./src
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Name: "app", Attribute: "build_context", AttrValue: "src"},
	})
}

func TestComposePort(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose port exposes endpoint",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.endpoint", Attribute: "service", AttrValue: "web"},
	})
}

func TestComposeVolume(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose volume reference",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  db:
    image: postgres:15
    volumes:
      - pgdata:/var/lib/postgresql/data
volumes:
  pgdata:
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "storage.volume", Attribute: "source", AttrValue: "pgdata"},
	})
}

func TestComposeLabelKindOverride(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose label overrides kind",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.yml",
			Language: "yaml",
			Source: []byte(`services:
  worker:
    image: alpine:latest
    labels:
      tld.kind: "worker"
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Name: "worker", Attribute: "kind", AttrValue: "worker"},
	})
}

func TestComposeEmptyServices(t *testing.T) {
	registry := enrich.NewRegistry(Compose())
	facts, _, err := registry.EnrichFile(context.Background(), enrich.FileInput{
		RelPath:  "docker-compose.yml",
		Language: "yaml",
		Source:   []byte("services:\n"),
		Signals:  triggerSignals(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) > 0 {
		t.Fatalf("expected no facts for empty services, got %d: %+v", len(facts), facts)
	}
}

func TestComposeNonComposeYAML(t *testing.T) {
	registry := enrich.NewRegistry(Compose())
	facts, _, err := registry.EnrichFile(context.Background(), enrich.FileInput{
		RelPath:  "config.yaml",
		Language: "yaml",
		Source: []byte(`apiVersion: v1
kind: ConfigMap
`),
		Signals: triggerSignals(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) > 0 {
		t.Fatalf("expected no facts for non-compose yaml, got %d: %+v", len(facts), facts)
	}
}

func TestComposeComposeYAMLPath(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "compose.yaml path matches",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "compose.yaml",
			Language: "yaml",
			Source: []byte(`services:
  api:
    image: myapp:latest
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Tag: "runtime:compose", Name: "api"},
	})
}

func TestComposeOverrideFile(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "docker-compose.override.yml matches",
		Enricher: Compose(),
		Input: enrich.FileInput{
			RelPath:  "docker-compose.override.yml",
			Language: "yaml",
			Source: []byte(`services:
  web:
    ports:
      - "8080:80"
`),
		},
		Signals: triggerSignals(),
		Want:    enrichertest.Fact{Type: "runtime.component", Name: "web"},
	})
}

func TestImageToTech(t *testing.T) {
	tests := []struct {
		image    string
		wantTech string
		wantKind string
	}{
		{"", "Container", "service"},
		{"postgres:15", "PostgreSQL", "database"},
		{"mysql:8", "MySQL", "database"},
		{"redis:alpine", "Redis", "cache"},
		{"nginx:latest", "Nginx", "proxy"},
		{"rabbitmq:3-management", "RabbitMQ", "queue"},
		{"kafka:latest", "Apache Kafka", "queue"},
		{"grafana/grafana:latest", "Grafana", "monitoring"},
		{"unknown-image:1.0", "Container", "service"},
		{"registry.example.com/myapp:v1", "Container", "service"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			tech, kind := imageToTech(tt.image)
			if tech != tt.wantTech {
				t.Errorf("imageToTech(%q) tech = %q, want %q", tt.image, tech, tt.wantTech)
			}
			if kind != tt.wantKind {
				t.Errorf("imageToTech(%q) kind = %q, want %q", tt.image, kind, tt.wantKind)
			}
		})
	}
}

func TestComposePathTokens(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docker-compose.yml", true},
		{"docker-compose.yaml", true},
		{"docker-compose.override.yml", true},
		{"docker-compose.prod.yaml", true},
		{"compose.yaml", true},
		{"compose.yml", true},
		{"deploy/docker-compose.yml", true},
		{"config.yaml", false},
		{"main.go", false},
		{"docker-compose", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := composePathTokens(tt.path)
			if got != tt.want {
				t.Errorf("composePathTokens(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractServiceRef(t *testing.T) {
	names := []string{"api", "db", "redis", "web", "worker"}
	tests := []struct {
		value string
		want  string
	}{
		{"http://api:8080", "api"},
		{"db:5432", "db"},
		{"redis://redis:6379", "redis"},
		{"web", "web"},
		{"worker.internal:9000", "worker"},
		{"localhost", ""},
		{"127.0.0.1", ""},
		{"true", ""},
		{"false", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := extractServiceRef(tt.value, names)
			if got != tt.want {
				t.Errorf("extractServiceRef(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestTagValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"PostgreSQL", "postgresql"},
		{"Apache Kafka", "apache-kafka"},
		{"My App", "my-app"},
		{"  Spaces  ", "spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tagValue(tt.input)
			if got != tt.want {
				t.Errorf("tagValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
