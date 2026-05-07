package storage

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.redis", "TypeScript Redis/ioredis", "typescript", "ioredis", "Redis(", "cache.key", "caches"),
		spec("ts.mongodb", "TypeScript MongoDB driver", "typescript", "mongodb", "MongoClient", "storage.collection", "reads_from"),
		spec("ts.elasticsearch", "TypeScript Elasticsearch client", "typescript", "@elastic/elasticsearch", "Client(", "storage.index", "indexes"),
		spec("ts.opensearch", "TypeScript OpenSearch client", "typescript", "@opensearch-project/opensearch", "OpenSearch", "storage.index", "indexes"),
		spec("go.redis", "Go go-redis", "go", "github.com/redis/go-redis", "redis.NewClient", "cache.key", "caches"),
		spec("go.mongodb", "MongoDB Go driver", "go", "go.mongodb.org/mongo-driver", "mongo.Connect", "storage.collection", "reads_from"),
		spec("go.elasticsearch", "Go Elasticsearch client", "go", "github.com/elastic/go-elasticsearch", "elasticsearch.NewClient", "storage.index", "indexes"),
		spec("go.opensearch", "Go OpenSearch client", "go", "github.com/opensearch-project/opensearch-go", "opensearch.NewClient", "storage.index", "indexes"),
		spec("python.redis", "Python redis-py", "python", "redis", "redis.Redis", "cache.key", "caches"),
		spec("python.pymongo", "Python PyMongo", "python", "pymongo", "MongoClient", "storage.collection", "reads_from"),
		spec("python.elasticsearch", "Python Elasticsearch", "python", "elasticsearch", "Elasticsearch(", "storage.index", "indexes"),
		spec("python.opensearch", "Python opensearch-py", "python", "opensearch-py", "OpenSearch(", "storage.index", "indexes"),
		spec("java.lettuce", "Java Lettuce", "java", "io.lettuce", "RedisClient", "cache.key", "caches"),
		spec("java.jedis", "Java Jedis", "java", "redis.clients", "Jedis", "cache.key", "caches"),
		spec("java.mongodb", "MongoDB Java driver", "java", "mongodb-driver", "MongoClient", "storage.collection", "reads_from"),
		spec("java.elasticsearch", "Elasticsearch Java client", "java", "co.elastic.clients", "ElasticsearchClient", "storage.index", "indexes"),
		spec("java.opensearch", "OpenSearch Java client", "java", "org.opensearch.client", "OpenSearchClient", "storage.index", "indexes"),
		spec("rust.redis", "Rust redis", "rust", "redis", "redis::", "cache.key", "caches"),
		spec("rust.mongodb", "Rust mongodb", "rust", "mongodb", "mongodb::", "storage.collection", "reads_from"),
		spec("rust.elasticsearch", "Rust elasticsearch", "rust", "elasticsearch", "elasticsearch::", "storage.index", "indexes"),
		spec("rust.opensearch", "Rust opensearch", "rust", "opensearch", "opensearch::", "storage.index", "indexes"),
		spec("cpp.redis_plus_plus", "C++ redis-plus-plus", "cpp", "redis-plus-plus", "sw::redis", "cache.key", "caches"),
		spec("cpp.mongodb", "MongoDB C++ driver", "cpp", "mongo-cxx-driver", "mongocxx::", "storage.collection", "reads_from"),
		spec("cpp.elasticsearch_http", "C++ Elasticsearch/OpenSearch HTTP", "cpp", "elasticsearch", "elasticsearch", "storage.index", "indexes"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "storage",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"storage:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
