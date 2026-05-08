package iac

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("iac.kubernetes", "Kubernetes YAML", "yaml", []string{"kind: Deployment", "apiVersion: apps/v1", "kind: StatefulSet", "kind: DaemonSet"}, nil, "runtime.service", "deploys"),
		spec("iac.helm", "Helm values", "yaml", []string{"helm.sh/chart", "{{ .Values", "{{ .Release"}, []string{"chart.yaml", "values.yaml"}, "runtime.service", "deploys"),
		spec("iac.terraform", "Terraform", "terraform", []string{"resource \"", "module \""}, []string{".tf"}, "cloud.resource", "provisions"),
		spec("iac.pulumi", "Pulumi", "typescript", []string{"new aws.", "pulumi."}, []string{"pulumi.yaml", "pulumi.yml"}, "cloud.resource", "provisions"),
		spec("iac.serverless", "Serverless Framework", "yaml", nil, []string{"serverless.yml", "serverless.yaml"}, "runtime.service", "deploys"),
		spec("iac.aws_cdk", "AWS CDK", "typescript", []string{"aws-cdk-lib", "new cdk.Stack"}, []string{"cdk.json"}, "cloud.resource", "provisions"),
		spec("iac.github_actions_deploy", "GitHub Actions deployment configs", "yaml", nil, []string{".github/workflows/"}, "deployment.workflow", "deploys"),
	}
}

func spec(id, name, language string, tokens, pathTokens []string, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "iac",
		Languages:    []string{language},
		Mode:         enrich.ActivationAlways,
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: tokens,
		PathTokens:   pathTokens,
		Tags:         []string{"iac:" + id},
		Attributes:   map[string]string{"language": language},
	}
}
