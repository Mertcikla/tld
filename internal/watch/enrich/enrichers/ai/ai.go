package ai

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.pinecone", "TypeScript Pinecone", "typescript", "@pinecone-database/pinecone", "Pinecone", "ai.vector_index", "queries_index"),
		spec("python.pinecone", "Python Pinecone", "python", "pinecone-client", "Pinecone", "ai.vector_index", "queries_index"),
		spec("ts.milvus", "TypeScript Milvus", "typescript", "@zilliz/milvus2-sdk-node", "MilvusClient", "ai.vector_index", "queries_index"),
		spec("python.milvus", "Python Milvus", "python", "pymilvus", "MilvusClient", "ai.vector_index", "queries_index"),
		spec("ts.qdrant", "TypeScript Qdrant", "typescript", "@qdrant/js-client-rest", "QdrantClient", "ai.vector_index", "queries_index"),
		spec("python.qdrant", "Python Qdrant", "python", "qdrant-client", "QdrantClient", "ai.vector_index", "queries_index"),
		spec("ts.chroma", "TypeScript Chroma", "typescript", "chromadb", "ChromaClient", "ai.vector_index", "queries_index"),
		spec("python.chroma", "Python Chroma", "python", "chromadb", "chromadb", "ai.vector_index", "queries_index"),
		spec("ts.weaviate", "TypeScript Weaviate", "typescript", "weaviate-ts-client", "weaviate.client", "ai.vector_index", "queries_index"),
		spec("python.weaviate", "Python Weaviate", "python", "weaviate-client", "weaviate.Client", "ai.vector_index", "queries_index"),
		spec("python.huggingface", "Python Hugging Face", "python", "huggingface_hub", "huggingface_hub", "ai.model_id", "loads_model"),
		spec("python.mlflow", "Python MLflow", "python", "mlflow", "mlflow.start_run", "ai.experiment_tracker", "tracks_metrics_to"),
		spec("python.wandb", "Python Weights & Biases", "python", "wandb", "wandb.init", "ai.experiment_tracker", "tracks_metrics_to"),
		spec("ts.openai", "TypeScript OpenAI SDK", "typescript", "openai", "new OpenAI", "ai.llm_endpoint", "calls_llm"),
		spec("python.openai", "Python OpenAI SDK", "python", "openai", "OpenAI(", "ai.llm_endpoint", "calls_llm"),
		spec("ts.anthropic", "TypeScript Anthropic SDK", "typescript", "@anthropic-ai/sdk", "Anthropic", "ai.llm_endpoint", "calls_llm"),
		spec("python.anthropic", "Python Anthropic SDK", "python", "anthropic", "Anthropic(", "ai.llm_endpoint", "calls_llm"),
		spec("ts.langchain", "TypeScript LangChain", "typescript", "langchain", "langchain", "ai.llm_endpoint", "calls_llm"),
		spec("python.langchain", "Python LangChain", "python", "langchain", "langchain", "ai.llm_endpoint", "calls_llm"),
		spec("ts.llamaindex", "TypeScript LlamaIndex", "typescript", "llamaindex", "llamaindex", "ai.llm_endpoint", "calls_llm"),
		spec("python.llamaindex", "Python LlamaIndex", "python", "llama-index", "llama_index", "ai.llm_endpoint", "calls_llm"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "ai",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"ai:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
