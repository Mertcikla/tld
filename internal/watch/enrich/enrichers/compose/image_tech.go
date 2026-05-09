package compose

import (
	"path"
	"strings"
)

type imageInfo struct {
	Tech string
	Kind string
}

var imageTechRegistry = map[string]imageInfo{
	"postgres":    {"PostgreSQL", "database"},
	"mysql":       {"MySQL", "database"},
	"mariadb":     {"MariaDB", "database"},
	"mongo":       {"MongoDB", "database"},
	"mongodb":     {"MongoDB", "database"},
	"couchdb":     {"CouchDB", "database"},
	"couchbase":   {"Couchbase", "database"},
	"cassandra":   {"Cassandra", "database"},
	"neo4j":       {"Neo4j", "database"},
	"arangodb":    {"ArangoDB", "database"},
	"dynamodb":    {"DynamoDB", "database"},
	"cockroachdb": {"CockroachDB", "database"},
	"rethinkdb":   {"RethinkDB", "database"},
	"influxdb":    {"InfluxDB", "database"},
	"timescaledb": {"TimescaleDB", "database"},
	"clickhouse":  {"ClickHouse", "database"},
	"sqlite":      {"SQLite", "database"},

	"redis":       {"Redis", "cache"},
	"memcached":   {"Memcached", "cache"},
	"dragonflydb": {"DragonflyDB", "cache"},
	"keydb":       {"KeyDB", "cache"},

	"celery": {"Celery", "worker"},

	"nginx":   {"Nginx", "proxy"},
	"traefik": {"Traefik", "proxy"},
	"haproxy": {"HAProxy", "proxy"},
	"envoy":   {"Envoy", "proxy"},
	"caddy":   {"Caddy", "proxy"},
	"apache":  {"Apache", "proxy"},
	"httpd":   {"Apache", "proxy"},
	"kong":    {"Kong", "proxy"},
	"apisix":  {"Apache APISIX", "proxy"},
	"bff":     {"BFF", "gateway"},
	"gateway": {"API Gateway", "gateway"},

	"kafka":    {"Apache Kafka", "queue"},
	"rabbitmq": {"RabbitMQ", "queue"},
	"nats":     {"NATS", "queue"},
	"activemq": {"ActiveMQ", "queue"},
	"pulsar":   {"Apache Pulsar", "queue"},
	"zeromq":   {"ZeroMQ", "queue"},
	"redpanda": {"Redpanda", "queue"},
	"pubsub":   {"Pub/Sub", "queue"},
	"kinesis":  {"Kinesis", "queue"},
	"amazonmq": {"Amazon MQ", "queue"},

	"elasticsearch": {"Elasticsearch", "search"},
	"opensearch":    {"OpenSearch", "search"},
	"meilisearch":   {"Meilisearch", "search"},
	"solr":          {"Solr", "search"},
	"algolia":       {"Algolia", "search"},
	"typesense":     {"Typesense", "search"},

	"minio":     {"MinIO", "storage"},
	"ceph":      {"Ceph", "storage"},
	"seaweedfs": {"SeaweedFS", "storage"},
	"glusterfs": {"GlusterFS", "storage"},

	"grafana":       {"Grafana", "monitoring"},
	"prometheus":    {"Prometheus", "monitoring"},
	"jaeger":        {"Jaeger", "observability"},
	"opentelemetry": {"OpenTelemetry", "observability"},
	"datadog":       {"Datadog", "monitoring"},
	"loki":          {"Loki", "monitoring"},
	"tempo":         {"Tempo", "observability"},
	"mimir":         {"Mimir", "monitoring"},

	"vault":     {"Vault", "security"},
	"consul":    {"Consul", "service-mesh"},
	"istio":     {"Istio", "service-mesh"},
	"linkerd":   {"Linkerd", "service-mesh"},
	"keycloak":  {"Keycloak", "auth"},
	"authentik": {"Authentik", "auth"},
	"authelia":  {"Authelia", "auth"},

	"zookeeper": {"ZooKeeper", "coordination"},
	"etcd":      {"etcd", "coordination"},

	"curlimages": {"curl", "utility"},
	"alpine":     {"Alpine", "utility"},
	"busybox":    {"BusyBox", "utility"},
	"ubuntu":     {"Ubuntu", "utility"},
	"debian":     {"Debian", "utility"},
	"golang":     {"Go", "utility"},
	"python":     {"Python", "utility"},
	"node":       {"Node.js", "utility"},
	"ruby":       {"Ruby", "utility"},
	"openjdk":    {"OpenJDK", "utility"},
	"rust":       {"Rust", "utility"},
}

func imageToTech(image string) (tech, kind string) {
	if image == "" {
		return "Container", "service"
	}
	base := strings.ToLower(path.Base(strings.Split(image, ":")[0]))
	if info, ok := imageTechRegistry[base]; ok {
		return info.Tech, info.Kind
	}
	if info, ok := imageTechRegistry[stripVersion(base)]; ok {
		return info.Tech, info.Kind
	}
	return "Container", "service"
}

func stripVersion(base string) string {
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] >= '0' && base[i] <= '9' {
			continue
		}
		if base[i] == '.' || base[i] == '-' || base[i] == '_' {
			continue
		}
		return base[:i+1]
	}
	return base
}
