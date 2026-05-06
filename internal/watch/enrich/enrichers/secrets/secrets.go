package secrets

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	providers := []struct {
		id    string
		name  string
		token string
	}{
		{"aws_secrets_manager", "AWS Secrets Manager", "secretsmanager"},
		{"aws_ssm", "AWS SSM Parameter Store", "ssm.get_parameter"},
		{"gcp_secret_manager", "GCP Secret Manager", "google.cloud.secretmanager"},
		{"azure_key_vault", "Azure Key Vault", "vault.azure.net"},
		{"kubernetes_secrets", "Kubernetes Secrets", "secretKeyRef"},
		{"vault", "Vault", "vault.hashicorp.com"},
		{"doppler", "Doppler", "DOPPLER_TOKEN"},
		{"onepassword", "1Password Secrets Automation", "OP_SERVICE_ACCOUNT_TOKEN"},
	}
	var specs []pattern.Spec
	for _, provider := range providers {
		specs = append(specs,
			spec("secrets.code."+provider.id, provider.name+" code reference", "go", provider.token),
			spec("secrets.config."+provider.id, provider.name+" config reference", "yaml", provider.token),
			spec("secrets.iac."+provider.id, provider.name+" IaC reference", "hcl", provider.token),
		)
	}
	return specs
}

func spec(id, name, language, token string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "secrets",
		Languages:    []string{language},
		Mode:         enrich.ActivationAlways,
		FactType:     "secret.provider",
		Relationship: "uses_secret",
		SourceTokens: []string{token},
		Tags:         []string{"secrets:" + id},
		Attributes:   map[string]string{"surface": language},
	}
}
