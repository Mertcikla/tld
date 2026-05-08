package generic

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput

type detector struct {
	ID           string
	Name         string
	Category     string
	FactType     string
	Relationship string
	ObjectKind   string
	Tags         []string
	Tokens       []string
	PathTokens   []string
	Attrs        map[string]string
}

var tokenCleanupRE = regexp.MustCompile(`(?m)(^|[^:])//.*$|#.*$|/\*[\s\S]*?\*/|<!--[\s\S]*?-->`)

// ArchitectureGlue detects common framework/library entrypoints and integration
// glue from imports, manifests, and configuration files.
func ArchitectureGlue() Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{ID: "generic.architecture_glue", Name: "Generic architecture glue", Mode: enrich.ActivationAlways},
		func(input FileInput) bool {
			return !ignoredPath(input.RelPath)
		},
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emitGenericFacts(input, emit)
		},
	)
}

func emitGenericFacts(input FileInput, emit FactEmitter) error {
	source := string(input.Source)
	scannable := tokenCleanupRE.ReplaceAllString(source, "$1")
	seen := map[string]struct{}{}
	for _, det := range detectors {
		line := detectorLine(input.RelPath, scannable, det)
		if line == 0 {
			continue
		}
		key := det.FactType + ":" + det.ID + ":" + input.RelPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		attrs := map[string]string{
			"category":   det.Category,
			"technology": det.Name,
			"detector":   det.ID,
		}
		maps.Copy(attrs, det.Attrs)
		tags := append([]string{"arch:glue", "category:" + tagValue(det.Category), "technology:" + tagValue(det.Name)}, det.Tags...)
		if err := emit.EmitFact(Fact{
			Type:         det.FactType,
			StableKey:    key,
			Subject:      enrich.SubjectForLine(input, line),
			Object:       enrich.SubjectRef{Kind: firstNonEmpty(det.ObjectKind, det.FactType), StableKey: det.FactType + ":" + det.ID, FilePath: input.RelPath, Name: det.Name},
			Relationship: det.Relationship,
			Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
			Confidence:   0.72,
			Name:         det.Name,
			Tags:         tags,
			Attributes:   attrs,
			VisibilityHints: map[string]float64{
				"high_signal": 0.6,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func detectorLine(relPath, source string, det detector) int {
	rel := strings.ToLower(filepath.ToSlash(relPath))
	for _, token := range det.PathTokens {
		if strings.Contains(rel, strings.ToLower(token)) {
			return 1
		}
	}
	lower := strings.ToLower(source)
	for _, token := range det.Tokens {
		idx := strings.Index(lower, strings.ToLower(token))
		if idx >= 0 {
			return enrich.LineForOffset(source, idx)
		}
	}
	return 0
}

func ignoredPath(relPath string) bool {
	parts := strings.SplitSeq(filepath.ToSlash(relPath), "/")
	for part := range parts {
		switch strings.ToLower(part) {
		case ".git", "node_modules", "vendor", "dist", "build", "coverage", "generated", "gen":
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func tagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" / ", "-", "/", "-", " ", "-", "&", "and", ".", "").Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func d(id, name, category, factType, relationship string, tokens ...string) detector {
	return detector{
		ID:           id,
		Name:         name,
		Category:     category,
		FactType:     factType,
		Relationship: relationship,
		Tags:         []string{fmt.Sprintf("%s:%s", tagValue(category), tagValue(name))},
		Tokens:       tokens,
		Attrs:        map[string]string{"framework": id},
	}
}

var detectors = []detector{
	// Observability / telemetry.
	d("opentelemetry", "OpenTelemetry", "observability", "telemetry.project", "reports_to", "@opentelemetry/", "go.opentelemetry.io/otel", "opentelemetry", "io.opentelemetry", "opentelemetry-cpp"),
	d("prometheus", "Prometheus", "observability", "telemetry.metric", "emits_metric", "prom-client", "github.com/prometheus/client_golang", "prometheus_client", "micrometer-registry-prometheus", "prometheus-cpp"),
	d("sentry", "Sentry", "observability", "telemetry.project", "reports_to", "@sentry/", "github.com/getsentry/sentry-go", "sentry_sdk", "io.sentry", "sentry-native"),
	d("datadog", "Datadog", "observability", "telemetry.project", "reports_to", "dd-trace", "gopkg.in/DataDog/dd-trace-go", "ddtrace", "com.datadoghq", "datadog"),
	d("micrometer", "Micrometer", "observability", "telemetry.metric", "emits_metric", "io.micrometer", "micrometer-registry"),
	d("rust-tracing", "Rust tracing", "observability", "telemetry.span", "creates_span", "tracing::", "tracing ="),
	d("rust-metrics", "Rust metrics", "observability", "telemetry.metric", "emits_metric", "metrics::", "metrics ="),

	// Auth / identity.
	d("auth0", "Auth0", "auth", "auth.provider", "uses_identity_provider", "@auth0/", "auth0.com", "github.com/auth0", "com.auth0"),
	d("cognito", "Cognito", "auth", "auth.provider", "uses_identity_provider", "amazon-cognito", "cognitoidentityprovider", "cognito-idp", "software.amazon.awssdk.services.cognitoidentityprovider"),
	d("firebase-auth", "Firebase Auth", "auth", "auth.provider", "uses_identity_provider", "firebase/auth", "firebase_admin.auth", "firebaseauth"),
	d("clerk", "Clerk", "auth", "auth.provider", "uses_identity_provider", "@clerk/", "clerk.com"),
	d("nextauth", "NextAuth", "auth", "auth.provider", "uses_identity_provider", "next-auth"),
	d("jwt", "JWT validation", "auth", "auth.issuer", "trusts_issuer", "github.com/golang-jwt/jwt", "jsonwebtoken", "jwt-cpp", "jwtvalidator"),
	d("oidc", "OIDC", "auth", "auth.issuer", "trusts_issuer", "coreos/go-oidc", "openidconnect", "openid-client", "oidc"),
	d("pyjwt", "PyJWT", "auth", "auth.issuer", "trusts_issuer", "pyjwt", "import jwt"),
	d("authlib", "Authlib", "auth", "auth.provider", "uses_identity_provider", "authlib"),
	d("django-auth", "Django auth", "auth", "auth.provider", "authenticates_with", "django.contrib.auth"),
	d("fastapi-security", "FastAPI security", "auth", "auth.provider", "authenticates_with", "fastapi.security"),
	d("spring-security", "Spring Security OAuth/OIDC", "auth", "auth.provider", "uses_identity_provider", "spring-boot-starter-oauth2-client", "spring-security-oauth2", "@enablewebsecurity"),
	d("keycloak", "Keycloak", "auth", "auth.provider", "uses_identity_provider", "keycloak"),
	d("rust-oauth2", "Rust oauth2", "auth", "auth.provider", "uses_identity_provider", "oauth2 ="),

	// Background jobs / schedulers.
	d("bullmq", "BullMQ", "jobs", "job.queue", "enqueues", "bullmq"),
	d("agenda", "Agenda", "jobs", "job.schedule", "runs_on_schedule", "agenda"),
	d("node-cron", "node-cron", "jobs", "job.schedule", "runs_on_schedule", "node-cron", "cron.schedule"),
	d("robfig-cron", "robfig/cron", "jobs", "job.schedule", "runs_on_schedule", "github.com/robfig/cron"),
	d("asynq", "asynq", "jobs", "job.queue", "consumes", "github.com/hibiken/asynq"),
	d("machinery", "machinery", "jobs", "job.queue", "consumes", "github.com/RichardKnop/machinery"),
	d("celery", "Celery", "jobs", "job.worker", "handles_job", "celery", "@shared_task"),
	d("rq", "RQ", "jobs", "job.queue", "consumes", "from rq", "import rq"),
	d("apscheduler", "APScheduler", "jobs", "job.schedule", "runs_on_schedule", "apscheduler"),
	d("spring-scheduling", "Spring Scheduling", "jobs", "job.schedule", "runs_on_schedule", "@scheduled", "@enablescheduling"),
	d("quartz", "Quartz", "jobs", "job.schedule", "runs_on_schedule", "org.quartz"),
	d("tokio-cron-scheduler", "tokio-cron-scheduler", "jobs", "job.schedule", "runs_on_schedule", "tokio_cron_scheduler"),
	d("apalis", "apalis", "jobs", "job.queue", "consumes", "apalis"),

	// API specs / schema files.
	{ID: "openapi", Name: "OpenAPI / Swagger", Category: "api-spec", FactType: "api.spec", Relationship: "documents", PathTokens: []string{"openapi", "swagger"}, Tokens: []string{"openapi:", "\"openapi\"", "swagger:"}, Tags: []string{"api-spec:openapi"}, Attrs: map[string]string{"format": "openapi"}},
	{ID: "asyncapi", Name: "AsyncAPI", Category: "api-spec", FactType: "api.spec", Relationship: "documents", PathTokens: []string{"asyncapi"}, Tokens: []string{"asyncapi:", "\"asyncapi\""}, Tags: []string{"api-spec:asyncapi"}, Attrs: map[string]string{"format": "asyncapi"}},
	{ID: "graphql-schema", Name: "GraphQL schema", Category: "api-spec", FactType: "api.schema", Relationship: "declares", PathTokens: []string{".graphql", ".gql"}, Tokens: []string{"type Query", "schema {"}, Tags: []string{"api-spec:graphql"}, Attrs: map[string]string{"format": "graphql"}},
	{ID: "protobuf", Name: "Protocol Buffers", Category: "api-spec", FactType: "rpc.service", Relationship: "exposes", PathTokens: []string{".proto"}, Tokens: []string{"syntax = \"proto", "service "}, Tags: []string{"api-spec:protobuf", "protocol:grpc"}, Attrs: map[string]string{"format": "protobuf"}},
	{ID: "avro", Name: "Avro", Category: "api-spec", FactType: "api.schema", Relationship: "declares", PathTokens: []string{".avsc", ".avdl"}, Tokens: []string{"\"type\":\"record\"", "\"namespace\""}, Tags: []string{"api-spec:avro"}, Attrs: map[string]string{"format": "avro"}},
	{ID: "json-schema", Name: "JSON Schema", Category: "api-spec", FactType: "api.schema", Relationship: "declares", PathTokens: []string{"schema.json"}, Tokens: []string{"\"$schema\"", "json-schema.org"}, Tags: []string{"api-spec:json-schema"}, Attrs: map[string]string{"format": "json-schema"}},

	// CI/CD and deployment.
	{ID: "github-actions", Name: "GitHub Actions", Category: "deployment", FactType: "deployment.workflow", Relationship: "builds", PathTokens: []string{".github/workflows/"}, Tokens: []string{"runs-on:", "uses: actions/"}, Tags: []string{"deployment:github-actions"}, Attrs: map[string]string{"provider": "github-actions"}},
	{ID: "gitlab-ci", Name: "GitLab CI", Category: "deployment", FactType: "deployment.workflow", Relationship: "builds", PathTokens: []string{".gitlab-ci.yml"}, Tokens: []string{"gitlab-ci", "stages:"}, Tags: []string{"deployment:gitlab-ci"}, Attrs: map[string]string{"provider": "gitlab-ci"}},
	{ID: "circleci", Name: "CircleCI", Category: "deployment", FactType: "deployment.workflow", Relationship: "builds", PathTokens: []string{".circleci/config.yml"}, Tokens: []string{"circleci", "orbs:"}, Tags: []string{"deployment:circleci"}, Attrs: map[string]string{"provider": "circleci"}},
	{ID: "jenkinsfile", Name: "Jenkinsfile", Category: "deployment", FactType: "deployment.workflow", Relationship: "builds", PathTokens: []string{"jenkinsfile"}, Tokens: []string{"pipeline {"}, Tags: []string{"deployment:jenkinsfile"}, Attrs: map[string]string{"provider": "jenkins"}},
	{ID: "buildkite", Name: "Buildkite", Category: "deployment", FactType: "deployment.workflow", Relationship: "builds", PathTokens: []string{".buildkite/"}, Tokens: []string{"buildkite", "plugins:"}, Tags: []string{"deployment:buildkite"}, Attrs: map[string]string{"provider": "buildkite"}},
	{ID: "argo-cd", Name: "Argo CD", Category: "deployment", FactType: "deployment.target", Relationship: "deploys_to", Tokens: []string{"argoproj.io", "kind: Application"}, Tags: []string{"deployment:argo-cd"}, Attrs: map[string]string{"provider": "argo-cd"}},
	{ID: "flux", Name: "Flux", Category: "deployment", FactType: "deployment.target", Relationship: "deploys_to", Tokens: []string{"toolkit.fluxcd.io", "kind: Kustomization"}, Tags: []string{"deployment:flux"}, Attrs: map[string]string{"provider": "flux"}},

	// Secrets / credentials.
	d("aws-secrets-manager", "AWS Secrets Manager", "secrets", "secret.provider", "uses_secret", "secretsmanager", "aws_secretsmanager_secret", "software.amazon.awssdk.services.secretsmanager"),
	d("aws-ssm", "AWS SSM Parameter Store", "secrets", "secret.provider", "reads_config", "ssm:GetParameter", "aws_ssm_parameter", "ssm.get_parameter"),
	d("gcp-secret-manager", "GCP Secret Manager", "secrets", "secret.provider", "uses_secret", "secretmanager.googleapis.com", "google.cloud.secretmanager"),
	d("azure-key-vault", "Azure Key Vault", "secrets", "secret.provider", "uses_secret", "azure.keyvault", "azurerm_key_vault", "vault.azure.net"),
	d("kubernetes-secrets", "Kubernetes Secrets", "secrets", "secret.provider", "uses_secret", "kind: Secret", "secretKeyRef"),
	d("vault", "Vault", "secrets", "secret.provider", "uses_secret", "hashicorp/vault", "vault kv", "vault.hashicorp.com"),
	d("doppler", "Doppler", "secrets", "secret.provider", "uses_secret", "doppler", "DOPPLER_TOKEN"),
	d("onepassword", "1Password Secrets Automation", "secrets", "secret.provider", "uses_secret", "1password", "op://", "OP_SERVICE_ACCOUNT_TOKEN"),

	// Monorepo / package boundaries.
	d("nx", "Nx", "workspace", "workspace.package", "contains", "nx.json", "@nrwl/", "@nx/"),
	d("turborepo", "Turborepo", "workspace", "workspace.package", "builds", "turbo.json", "turbo run"),
	d("pnpm-workspaces", "pnpm workspaces", "workspace", "workspace.package", "contains", "pnpm-workspace.yaml"),
	d("yarn-workspaces", "Yarn workspaces", "workspace", "workspace.package", "contains", "\"workspaces\""),
	d("bazel", "Bazel", "workspace", "module.boundary", "builds", "WORKSPACE", "BUILD.bazel", "bazel_dep("),
	d("gradle-multiproject", "Gradle multi-project", "workspace", "module.boundary", "contains", "settings.gradle", "include("),
	d("maven-modules", "Maven modules", "workspace", "module.boundary", "contains", "<modules>", "<module>"),
	d("cargo-workspace", "Cargo workspace", "workspace", "workspace.package", "contains", "[workspace]"),
	d("go-workspace", "Go workspaces", "workspace", "workspace.package", "contains", "go.work", "use ("),

	// AI / ML operations and LLMs.
	d("pinecone", "Pinecone", "ai", "ai.vector_index", "queries_index", "pinecone"),
	d("milvus", "Milvus", "ai", "ai.vector_index", "queries_index", "milvus"),
	d("qdrant", "Qdrant", "ai", "ai.vector_index", "queries_index", "qdrant"),
	d("chroma", "Chroma", "ai", "ai.vector_index", "queries_index", "chromadb", "chroma_client"),
	d("weaviate", "Weaviate", "ai", "ai.vector_index", "queries_index", "weaviate"),
	d("huggingface", "Hugging Face", "ai", "ai.model_id", "loads_model", "huggingface_hub", "transformers"),
	d("mlflow", "MLflow", "ai", "ai.experiment_tracker", "tracks_metrics_to", "mlflow"),
	d("wandb", "Weights & Biases", "ai", "ai.experiment_tracker", "tracks_metrics_to", "wandb"),
	d("openai", "OpenAI SDK", "ai", "ai.llm_endpoint", "calls_llm", "openai", "@openai/"),
	d("anthropic", "Anthropic SDK", "ai", "ai.llm_endpoint", "calls_llm", "anthropic", "@anthropic-ai/"),
	d("langchain", "LangChain", "ai", "ai.llm_endpoint", "calls_llm", "langchain"),
	d("llamaindex", "LlamaIndex", "ai", "ai.llm_endpoint", "calls_llm", "llama_index", "llamaindex"),

	// Embedded systems and IoT messaging.
	d("mqtt", "MQTT", "iot", "iot.mqtt_topic", "publishes_to_device", "mqtt", "paho.mqtt", "mosquitto"),
	d("coap", "CoAP", "iot", "iot.broker", "publishes_to_device", "coap"),
	d("i2c", "I2C", "iot", "hardware.bus_address", "communicates_via_i2c", "i2c_init", "i2c_open", "i2c_transfer", "i2c_read", "i2c_write", "SMBus"),
	d("spi", "SPI", "iot", "hardware.bus_address", "communicates_via_i2c", "spi_init", "spi_open", "spi_transfer", "spi_mode", "SPIDevice", "spidev"),
	d("uart", "UART", "iot", "hardware.pin", "communicates_via_i2c", "uart_init", "uart_open", "uart_write", "uart_read", "uart_puts", "serialport"),
	d("can-bus", "CAN Bus", "iot", "hardware.bus_address", "communicates_via_i2c", "canbus", "socketcan"),

	// Kernel, systems, and local IPC.
	d("unix-socket", "Unix Domain Sockets", "ipc", "ipc.socket_path", "connects_to_socket", "unix://", "AF_UNIX"),
	d("dbus", "D-Bus", "ipc", "ipc.dbus_interface", "exposes_dbus_service", "dbus", "org.freedesktop"),
	d("named-pipes", "Named Pipes", "ipc", "ipc.socket_path", "connects_to_socket", `\\.\pipe\`, "mkfifo"),
	d("grpc-uds", "gRPC over UDS", "ipc", "ipc.socket_path", "connects_to_socket", "unix:", "grpc.WithContextDialer"),
	d("sysfs", "sysfs / procfs", "kernel", "kernel.device_node", "reads_device", "/sys/", "/proc/"),
	d("ebpf", "eBPF", "kernel", "kernel.device_node", "reads_device", "kprobe", "uprobe", "tracepoint", "libbpf", "bcc"),

	// Data engineering and orchestration.
	d("airflow", "Apache Airflow", "data", "data.pipeline_id", "depends_on_task", "airflow", "DAG("),
	d("prefect", "Prefect", "data", "data.pipeline_id", "depends_on_task", "prefect", "@flow"),
	d("dagster", "Dagster", "data", "data.pipeline_id", "depends_on_task", "dagster", "@asset"),
	d("spark", "Apache Spark", "data", "data.dataset_uri", "reads_dataset", "pyspark", "spark.sql", "org.apache.spark"),
	d("ray", "Ray", "data", "data.pipeline_id", "depends_on_task", "ray.init", "import ray"),

	// Web3 / blockchain.
	d("ethers-js", "ethers.js", "web3", "web3.rpc_endpoint", "connects_to_chain", "ethers"),
	d("web3-js", "web3.js", "web3", "web3.rpc_endpoint", "connects_to_chain", "web3.js", "new Web3"),
	d("web3-py", "web3.py", "web3", "web3.rpc_endpoint", "connects_to_chain", "from web3 import Web3", "web3.py"),
	d("foundry", "Foundry", "web3", "web3.chain_id", "connects_to_chain", "foundry.toml", "forge-std"),
	d("hardhat", "Hardhat", "web3", "web3.chain_id", "connects_to_chain", "hardhat.config", "hardhat"),

	// Desktop / mobile OS integration.
	{ID: "uri-schemes", Name: "Custom URI schemes", Category: "os-integration", FactType: "os.uri_scheme", Relationship: "handles_deep_link", PathTokens: []string{"info.plist", "androidmanifest.xml", "electron"}, Tokens: []string{"CFBundleURLSchemes", "android.intent.action.VIEW"}, Tags: []string{"os-integration:uri-schemes"}, Attrs: map[string]string{"platform": "desktop-mobile"}},
	{ID: "android-intents", Name: "Android Intents", Category: "os-integration", FactType: "os.intent", Relationship: "broadcasts_intent", PathTokens: []string{"androidmanifest.xml"}, Tokens: []string{"<intent-filter", "sendbroadcast", "android.intent.action"}, Tags: []string{"os-integration:android-intents"}, Attrs: map[string]string{"platform": "android"}},
}

