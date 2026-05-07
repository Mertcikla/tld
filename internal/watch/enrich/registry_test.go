package enrich_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/analyzer"
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/defaults"
	goroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/golang"
)

type ActivationSignal = enrich.ActivationSignal
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata

const (
	ActivationAlways = enrich.ActivationAlways
	SignalDependency = enrich.SignalDependency
	SignalImport     = enrich.SignalImport
)

var (
	DefaultEnrichers   = defaults.DefaultEnrichers
	GoChi              = goroutes.GoChi
	ImportSignals      = enrich.ImportSignals
	DiscoverSignals    = enrich.DiscoverRepositorySignalsFromFiles
	NewDefaultRegistry = defaults.NewRegistry
	NewRegistry        = enrich.NewRegistry
)

func TestRegistryActivatesImportGatedEnrichers(t *testing.T) {
	source := []byte(`package main

import "github.com/go-chi/chi/v5"

func routes(r chi.Router) {
	r.Get("/users/{id}", getUser)
}

func getUser() {}
`)
	input := FileInput{
		RelPath:  "routes.go",
		Language: "go",
		Source:   source,
		Parsed: &analyzer.Result{Refs: []analyzer.Ref{{
			Kind:       "import",
			TargetPath: "github.com/go-chi/chi/v5",
			FilePath:   "routes.go",
			Line:       3,
		}}},
	}

	withoutSignals, _, err := NewRegistry(GoChi()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich without signals: %v", err)
	}
	if len(withoutSignals) != 0 {
		t.Fatalf("expected inactive chi enricher without signals, got %+v", withoutSignals)
	}

	input.Signals = ImportSignals(input.Parsed.Refs)
	withSignals, _, err := NewRegistry(GoChi()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich with signals: %v", err)
	}
	if len(withSignals) != 1 || withSignals[0].Type != "http.route" || !containsTag(withSignals[0].Tags, "framework:chi") {
		t.Fatalf("expected chi route fact, got %+v", withSignals)
	}
}

func TestRegistryRejectsInvalidFacts(t *testing.T) {
	bad := enrich.NewEnricher(
		Metadata{ID: "bad", Mode: ActivationAlways},
		nil,
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emit.EmitFact(Fact{Type: "demo.fact"})
		},
	)
	_, _, err := NewRegistry(bad).EnrichFile(context.Background(), FileInput{RelPath: "demo.go"})
	if err == nil || !strings.Contains(err.Error(), "stable key") {
		t.Fatalf("expected stable key validation error, got %v", err)
	}
}

func TestDefaultEnrichersHaveUniqueIDs(t *testing.T) {
	seen := map[string]struct{}{}
	for _, enricher := range DefaultEnrichers() {
		meta := enricher.Metadata()
		if strings.TrimSpace(meta.ID) == "" {
			t.Fatalf("default enricher has empty ID: %+v", meta)
		}
		if _, ok := seen[meta.ID]; ok {
			t.Fatalf("default enricher ID registered more than once: %s", meta.ID)
		}
		seen[meta.ID] = struct{}{}
	}
}

func TestDefaultEnrichersIncludeExpandedCatalog(t *testing.T) {
	enrichers := DefaultEnrichers()
	if len(enrichers) < 360 || len(enrichers) > 430 {
		t.Fatalf("expected expanded default catalog, got %d", len(enrichers))
	}
	want := []string{
		"ts.process_env",
		"python.httpx",
		"java.spring_web",
		"rust.axum",
		"cpp.drogon",
		"python.sqlalchemy",
		"rust.tonic",
		"ts.kafkajs",
		"go.aws_sdk_v2",
		"java.opensearch",
		"iac.terraform",
		"ts.opentelemetry",
		"go.jwt",
		"ts.bullmq",
		"apispec.openapi",
		"deployment.github_actions",
		"secrets.code.aws_secrets_manager",
		"workspace.nx",
		"python.openai",
		"go.mqtt",
		"go.unix_socket",
		"python.airflow",
		"ts.ethers",
		"os.uri_schemes",
	}
	seen := map[string]struct{}{}
	for _, enricher := range enrichers {
		seen[enricher.Metadata().ID] = struct{}{}
	}
	for _, id := range want {
		if _, ok := seen[id]; !ok {
			t.Fatalf("default catalog missing enricher %s", id)
		}
	}
	if _, ok := seen["generic.architecture_glue"]; ok {
		t.Fatalf("generic architecture glue should not be registered alongside categorized enrichers")
	}
}

func TestDiscoverRepositorySignalsFromExpandedManifests(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"requirements.txt": "fastapi==0.110.0\nhttpx>=0.27.0\n",
		"pyproject.toml":   "[project]\ndependencies = [\"sqlalchemy>=2\"]\n[tool.poetry.dependencies]\ndjango = \"^5\"\n",
		"Cargo.toml":       "[dependencies]\naxum = \"0.7\"\ntonic = \"0.11\"\n",
		"pom.xml":          `<project><dependencies><dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId></dependency></dependencies></project>`,
		"build.gradle":     `implementation "org.springframework.kafka:spring-kafka:3.1.0"`,
		"CMakeLists.txt":   "find_package(Drogon REQUIRED)\n",
		"conanfile.txt":    "requires = cpprestsdk/2.10.18\n",
		"vcpkg.json":       `{"dependencies":[{"name":"boost-beast"}]}`,
	}
	var paths []string
	for rel, data := range files {
		path := filepath.Join(root, rel)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	signals := DiscoverSignals(root, paths)
	for _, want := range []string{"fastapi", "httpx", "django", "axum", "tonic", "spring-boot-starter-web", "spring-kafka", "Drogon", "cpprestsdk", "boost-beast"} {
		if !hasSignal(signals, want) {
			t.Fatalf("missing dependency signal %q in %+v", want, signals)
		}
	}
}

func TestDefaultRegistryEmitsDemoFacts(t *testing.T) {
	tests := []struct {
		name     string
		input    FileInput
		signals  []ActivationSignal
		wantType string
		wantTag  string
	}{
		{
			name: "express route",
			input: FileInput{
				RelPath:  "server.ts",
				Language: "typescript",
				Source:   []byte(`router.get("/api/users", listUsers)`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "express"}},
			wantType: "http.route",
			wantTag:  "framework:express",
		},
		{
			name: "next page route",
			input: FileInput{
				RelPath:  "src/app/users/[id]/page.tsx",
				Language: "typescript",
				Source:   []byte(`export default function Page() { return null }`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "next"}},
			wantType: "frontend.route",
			wantTag:  "framework:nextjs",
		},
		{
			name: "prisma query",
			input: FileInput{
				RelPath:  "db.ts",
				Language: "typescript",
				Source:   []byte(`await prisma.user.findMany()`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "@prisma/client"}},
			wantType: "orm.query",
			wantTag:  "orm:prisma",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Signals = tt.signals
			facts, _, err := NewDefaultRegistry().EnrichFile(context.Background(), tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !hasFact(facts, tt.wantType, tt.wantTag) {
				t.Fatalf("missing %s/%s in %+v", tt.wantType, tt.wantTag, facts)
			}
		})
	}
}

func TestDefaultRegistryEmitsArchitectureGlueFacts(t *testing.T) {
	tests := []struct {
		name     string
		input    FileInput
		signals  []ActivationSignal
		wantType string
		wantTag  string
	}{
		{
			name: "go grpc client",
			input: FileInput{
				RelPath:  "src/frontend/rpc.go",
				Language: "go",
				Source: []byte(`package main
func f() { _ = pb.NewCartServiceClient(conn).GetCart(ctx, req) }`),
			},
			signals:  []ActivationSignal{{Kind: SignalImport, Value: "google.golang.org/grpc"}},
			wantType: "grpc.client",
			wantTag:  "grpc:client",
		},
		{
			name: "python grpc server",
			input: FileInput{
				RelPath:  "src/emailservice/email_server.py",
				Language: "python",
				Source:   []byte(`demo_pb2_grpc.add_EmailServiceServicer_to_server(service, server)`)},
			signals:  []ActivationSignal{{Kind: SignalImport, Value: "grpc"}},
			wantType: "grpc.server",
			wantTag:  "grpc:server",
		},
		{
			name: "node grpc server",
			input: FileInput{
				RelPath:  "src/paymentservice/server.js",
				Language: "javascript",
				Source:   []byte(`this.server.addService(hipsterShopPackage.PaymentService.service, { charge })`)},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "@grpc/grpc-js"}},
			wantType: "grpc.server",
			wantTag:  "grpc:server",
		},
		{
			name: "java grpc server",
			input: FileInput{
				RelPath:  "src/adservice/src/main/java/hipstershop/AdService.java",
				Language: "java",
				Source:   []byte(`class AdServiceImpl extends hipstershop.AdServiceGrpc.AdServiceImplBase {}`)},
			signals:  []ActivationSignal{{Kind: SignalImport, Value: "io.grpc"}},
			wantType: "grpc.server",
			wantTag:  "grpc:server",
		},
		{
			name: "dotnet grpc server",
			input: FileInput{
				RelPath:  "src/cartservice/src/Startup.cs",
				Language: "c-sharp",
				Source:   []byte(`endpoints.MapGrpcService<CartService>();`)},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "Grpc.AspNetCore"}},
			wantType: "grpc.server",
			wantTag:  "grpc:server",
		},
		{
			name: "protobuf contract",
			input: FileInput{
				RelPath:  "protos/demo.proto",
				Language: "protobuf",
				Source:   []byte(`service CheckoutService { rpc PlaceOrder(PlaceOrderRequest) returns (PlaceOrderResponse); }`)},
			wantType: "grpc.contract",
			wantTag:  "arch:contract",
		},
		{
			name: "runtime manifest component",
			input: FileInput{
				RelPath:  "kubernetes-manifests/frontend.yaml",
				Language: "yaml",
				Source: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
spec:
  template:
    spec:
      containers:
      - image: frontend
        env:
        - name: CART_SERVICE_ADDR
          value: cartservice:7070
`)},
			wantType: "runtime.component",
			wantTag:  "runtime:kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Signals = tt.signals
			facts, _, err := NewDefaultRegistry().EnrichFile(context.Background(), tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !hasFact(facts, tt.wantType, tt.wantTag) {
				t.Fatalf("missing %s/%s in %+v", tt.wantType, tt.wantTag, facts)
			}
		})
	}
}

func hasFact(facts []Fact, factType, tag string) bool {
	for _, fact := range facts {
		if fact.Type == factType && containsTag(fact.Tags, tag) {
			return true
		}
	}
	return false
}

func containsTag(tags []string, tag string) bool {
	return slices.Contains(tags, tag)
}

func hasSignal(signals []ActivationSignal, value string) bool {
	for _, signal := range signals {
		if signal.Kind == SignalDependency && signal.Value == value {
			return true
		}
	}
	return false
}
