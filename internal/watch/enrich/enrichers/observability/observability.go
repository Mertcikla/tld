package observability

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.opentelemetry", "TypeScript OpenTelemetry", "typescript", "@opentelemetry/api", "@opentelemetry/", "telemetry.span", "creates_span"),
		spec("ts.prometheus_client", "TypeScript Prometheus client", "typescript", "prom-client", "prom-client", "telemetry.metric", "emits_metric"),
		spec("ts.sentry", "TypeScript Sentry", "typescript", "@sentry/node", "@sentry/", "telemetry.project", "reports_to"),
		spec("ts.datadog", "TypeScript Datadog", "typescript", "dd-trace", "dd-trace", "telemetry.project", "reports_to"),
		spec("go.opentelemetry", "Go OpenTelemetry", "go", "go.opentelemetry.io/otel", "go.opentelemetry.io/otel", "telemetry.span", "creates_span"),
		spec("go.prometheus", "Go Prometheus", "go", "github.com/prometheus/client_golang", "prometheus.New", "telemetry.metric", "emits_metric"),
		spec("go.sentry", "Go Sentry", "go", "github.com/getsentry/sentry-go", "sentry.Init", "telemetry.project", "reports_to"),
		spec("go.datadog", "Go Datadog", "go", "gopkg.in/DataDog/dd-trace-go", "ddtrace", "telemetry.project", "reports_to"),
		spec("python.opentelemetry", "Python OpenTelemetry", "python", "opentelemetry-api", "opentelemetry", "telemetry.span", "creates_span"),
		spec("python.prometheus_client", "Python Prometheus client", "python", "prometheus-client", "prometheus_client", "telemetry.metric", "emits_metric"),
		spec("python.sentry_sdk", "Python Sentry SDK", "python", "sentry-sdk", "sentry_sdk", "telemetry.project", "reports_to"),
		spec("python.datadog_tracing", "Python Datadog tracing", "python", "ddtrace", "ddtrace", "telemetry.project", "reports_to"),
		spec("java.opentelemetry", "Java OpenTelemetry", "java", "io.opentelemetry", "io.opentelemetry", "telemetry.span", "creates_span"),
		spec("java.micrometer", "Java Micrometer", "java", "io.micrometer", "MeterRegistry", "telemetry.metric", "emits_metric"),
		spec("java.prometheus", "Java Prometheus", "java", "micrometer-registry-prometheus", "PrometheusMeterRegistry", "telemetry.metric", "emits_metric"),
		spec("java.sentry", "Java Sentry", "java", "io.sentry", "Sentry.init", "telemetry.project", "reports_to"),
		spec("java.datadog", "Java Datadog", "java", "com.datadoghq", "datadog", "telemetry.project", "reports_to"),
		spec("rust.tracing", "Rust tracing", "rust", "tracing", "tracing::", "telemetry.span", "creates_span"),
		spec("rust.opentelemetry", "Rust OpenTelemetry", "rust", "opentelemetry", "opentelemetry::", "telemetry.span", "creates_span"),
		spec("rust.metrics", "Rust metrics", "rust", "metrics", "metrics::", "telemetry.metric", "emits_metric"),
		spec("rust.sentry", "Rust Sentry", "rust", "sentry", "sentry::", "telemetry.project", "reports_to"),
		spec("cpp.opentelemetry", "C++ OpenTelemetry", "cpp", "opentelemetry-cpp", "opentelemetry", "telemetry.span", "creates_span"),
		spec("cpp.prometheus", "C++ Prometheus", "cpp", "prometheus-cpp", "prometheus::", "telemetry.metric", "emits_metric"),
		spec("cpp.sentry_native", "C++ Sentry Native", "cpp", "sentry-native", "sentry_init", "telemetry.project", "reports_to"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "observability",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"observability:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
