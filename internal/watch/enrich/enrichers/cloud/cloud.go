package cloud

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.aws_sdk_v3", "TypeScript AWS SDK v3", "typescript", "@aws-sdk/client-s3", "S3Client", "cloud.bucket", "reads_from"),
		spec("ts.google_cloud", "TypeScript Google Cloud clients", "typescript", "@google-cloud/storage", "Storage(", "cloud.bucket", "reads_from"),
		spec("ts.azure_sdk", "TypeScript Azure SDK", "typescript", "@azure/storage-blob", "BlobServiceClient", "cloud.resource", "reads_from"),
		spec("go.aws_sdk_v2", "Go AWS SDK v2", "go", "github.com/aws/aws-sdk-go-v2", "aws.Config", "cloud.resource", "reads_from"),
		spec("go.google_cloud", "Google Cloud Go", "go", "cloud.google.com/go", "cloud.google.com/go", "cloud.resource", "reads_from"),
		spec("go.azure_sdk", "Azure SDK for Go", "go", "github.com/Azure/azure-sdk-for-go", "azidentity", "cloud.resource", "reads_from"),
		spec("python.boto3", "Python boto3", "python", "boto3", "boto3.client", "cloud.resource", "reads_from"),
		spec("python.google_cloud", "Python google-cloud clients", "python", "google-cloud", "google.cloud", "cloud.resource", "reads_from"),
		spec("python.azure_sdk", "Python Azure SDK", "python", "azure-", "azure.storage", "cloud.resource", "reads_from"),
		spec("java.aws_sdk_v2", "Java AWS SDK v2", "java", "software.amazon.awssdk", "software.amazon.awssdk", "cloud.resource", "reads_from"),
		spec("java.google_cloud", "Google Cloud Java", "java", "com.google.cloud", "com.google.cloud", "cloud.resource", "reads_from"),
		spec("java.azure_sdk", "Azure SDK Java", "java", "com.azure", "com.azure", "cloud.resource", "reads_from"),
		spec("rust.aws_sdk", "AWS SDK Rust", "rust", "aws-sdk", "aws_sdk_", "cloud.resource", "reads_from"),
		spec("rust.google_cloud", "Google Cloud Rust clients", "rust", "google-cloud", "google_cloud", "cloud.resource", "reads_from"),
		spec("rust.azure_sdk", "Azure SDK Rust", "rust", "azure_", "azure_", "cloud.resource", "reads_from"),
		spec("cpp.aws_sdk", "AWS SDK C++", "cpp", "aws-sdk-cpp", "Aws::", "cloud.resource", "reads_from"),
		spec("cpp.google_cloud", "Google Cloud C++", "cpp", "google-cloud-cpp", "google::cloud", "cloud.resource", "reads_from"),
		spec("cpp.azure_sdk", "Azure SDK C++", "cpp", "azure-sdk-for-cpp", "Azure::", "cloud.resource", "reads_from"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "cloud",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"cloud:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
