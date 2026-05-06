package dataeng

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("python.airflow", "Python Apache Airflow", "python", "apache-airflow", "DAG(", "data.pipeline_id", "depends_on_task"),
		spec("python.prefect", "Python Prefect", "python", "prefect", "@flow", "data.pipeline_id", "depends_on_task"),
		spec("python.dagster", "Python Dagster", "python", "dagster", "@asset", "data.pipeline_id", "depends_on_task"),
		spec("python.spark", "Python Apache Spark", "python", "pyspark", "spark.sql", "data.dataset_uri", "reads_dataset"),
		spec("java.spark", "Java Apache Spark", "java", "org.apache.spark", "SparkSession", "data.dataset_uri", "reads_dataset"),
		spec("python.ray", "Python Ray", "python", "ray", "ray.init", "data.pipeline_id", "depends_on_task"),
		spec("java.ray", "Java Ray", "java", "io.ray", "Ray.init", "data.pipeline_id", "depends_on_task"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "data",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"data:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
