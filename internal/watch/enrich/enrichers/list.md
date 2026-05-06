## Enricher Completion Checklist

Completed entries below have one registered enricher per framework/library or framework/library-language combination, plus package-level tests.

### 11. Observability / Telemetry

Semantic facts: `telemetry.service`, `telemetry.metric`, `telemetry.span`, `telemetry.project`, `runtime.identifier`
Relationships: `emits_metric`, `creates_span`, `reports_to`, `identifies_service_as`

- [x] TypeScript: OpenTelemetry, Prometheus client, Sentry, Datadog
- [x] Go: OpenTelemetry, Prometheus, Sentry, Datadog
- [x] Python: OpenTelemetry, Prometheus client, Sentry SDK, Datadog tracing
- [x] Java: OpenTelemetry, Micrometer, Prometheus, Sentry, Datadog
- [x] Rust: tracing, opentelemetry, metrics, Sentry
- [x] C++: OpenTelemetry C++, Prometheus C++, Sentry Native

### 12. Auth / Identity Providers

Semantic facts: `auth.issuer`, `auth.audience`, `auth.provider`, `auth.tenant`, `auth.jwks_endpoint`
Relationships: `authenticates_with`, `trusts_issuer`, `validates_audience`, `uses_identity_provider`

- [x] TypeScript: Auth0, Cognito, Firebase Auth, Clerk, NextAuth
- [x] Go: JWT middleware, OIDC clients, Auth0/Cognito SDK usage
- [x] Python: PyJWT, Authlib, Django auth integrations, FastAPI security
- [x] Java: Spring Security OAuth/OIDC, Keycloak, Cognito
- [x] Rust: jsonwebtoken, oauth2, OIDC crates
- [x] C++: JWT validation libraries, OIDC/JWKS config through HTTP/config

### 13. Background Jobs / Schedulers

Semantic facts: `job.worker`, `job.schedule`, `job.queue`, `job.handler`
Relationships: `runs_on_schedule`, `consumes`, `handles_job`, `enqueues`

- [x] TypeScript: BullMQ, Agenda, node-cron
- [x] Go: robfig/cron, asynq, machinery
- [x] Python: Celery, RQ, APScheduler
- [x] Java: Spring Scheduling, Quartz
- [x] Rust: tokio-cron-scheduler, apalis
- [x] C++: cron-like wrappers, custom schedulers, queue consumers

### 14. API Specs / Schema Files

Semantic facts: `api.spec`, `api.operation`, `api.path`, `api.schema`, `rpc.service`
Relationships: `declares`, `documents`, `generates_client_for`, `exposes`

- [x] OpenAPI / Swagger
- [x] AsyncAPI
- [x] GraphQL schema
- [x] Protocol Buffers
- [x] Avro
- [x] JSON Schema

### 15. CI/CD and Deployment Glue

Semantic facts: `deployment.workflow`, `deployment.environment`, `deployment.target`, `runtime.service`, `cloud.resource`
Relationships: `deploys_to`, `builds`, `publishes_artifact`, `uses_secret`, `targets_environment`

- [x] GitHub Actions
- [x] GitLab CI
- [x] CircleCI
- [x] Jenkinsfile
- [x] Buildkite
- [x] Argo CD
- [x] Flux

### 16. Secrets / External Credential References

Semantic facts: `secret.reference`, `secret.provider`, `config.env`, `integration.credential`
Relationships: `uses_secret`, `reads_config`, `authenticates_with`

- [x] AWS Secrets Manager
- [x] AWS SSM Parameter Store
- [x] GCP Secret Manager
- [x] Azure Key Vault
- [x] Kubernetes Secrets
- [x] Vault
- [x] Doppler
- [x] 1Password/Secrets Automation

### 17. Monorepo / Package Boundary Metadata

Semantic facts: `workspace.package`, `workspace.service`, `module.boundary`, `dependency.module`
Relationships: `contains`, `depends_on`, `owns`, `builds`

- [x] Nx
- [x] Turborepo
- [x] pnpm workspaces
- [x] Yarn workspaces
- [x] Bazel
- [x] Gradle multi-project
- [x] Maven modules
- [x] Cargo workspace
- [x] Go workspaces

### 18. Negative / Non-Match Fixtures

- [x] Commented-out matches do not emit facts
- [x] Generated/vendor paths do not emit facts
- [x] Import/dependency-gated enrichers stay inactive without activation signals

### 19. AI / ML Operations & LLMs

Semantic facts: `ai.model_id`, `ai.vector_index`, `ai.experiment_tracker`, `ai.llm_endpoint`
Relationships: `queries_index`, `loads_model`, `tracks_metrics_to`, `calls_llm`

- [x] Vector DBs: Pinecone, Milvus, Qdrant, Chroma, Weaviate
- [x] Model Hubs/Trackers: Hugging Face, MLflow, Weights & Biases
- [x] LLM APIs: OpenAI SDK, Anthropic SDK, LangChain, LlamaIndex

### 20. Embedded Systems & IoT Messaging

Semantic facts: `iot.mqtt_topic`, `iot.broker`, `hardware.bus_address`, `hardware.pin`
Relationships: `publishes_to_device`, `subscribes_to_sensor`, `communicates_via_i2c`

- [x] IoT Messaging: MQTT, CoAP
- [x] Hardware Buses: I2C, SPI, UART, CAN Bus

### 21. Kernel, Systems & Local IPC

Semantic facts: `ipc.socket_path`, `ipc.dbus_interface`, `ipc.shared_memory_name`, `kernel.device_node`
Relationships: `connects_to_socket`, `exposes_dbus_service`, `reads_device`

- [x] Local IPC: Unix Domain Sockets, D-Bus, Named Pipes, gRPC over UDS
- [x] Kernel/System: `/dev/` paths, `sysfs` / `procfs` paths
- [x] eBPF: kprobes, uprobes, tracepoints used by BCC/libbpf-style code

### 22. Data Engineering & Orchestration

Semantic facts: `data.pipeline_id`, `data.task_dependency`, `data.dataset_uri`
Relationships: `depends_on_task`, `reads_dataset`, `writes_dataset`

- [x] Orchestrators: Apache Airflow, Prefect, Dagster
- [x] Processing: Apache Spark, Ray

### 23. Web3 / Blockchain

Semantic facts: `web3.contract_address`, `web3.rpc_endpoint`, `web3.chain_id`
Relationships: `calls_contract`, `connects_to_chain`

- [x] ethers.js
- [x] web3.js
- [x] web3.py
- [x] Foundry
- [x] Hardhat

### 24. Desktop / Mobile App OS Integration

Semantic facts: `os.uri_scheme`, `os.intent`
Relationships: `handles_deep_link`, `broadcasts_intent`

- [x] Deep linking/custom URI schemes in Info.plist, AndroidManifest.xml, or Electron configs
- [x] Android Intents
