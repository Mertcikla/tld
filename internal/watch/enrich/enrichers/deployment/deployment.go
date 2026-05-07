package deployment

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("deployment.github_actions", "GitHub Actions", []string{".github/workflows/"}, []string{"runs-on:"}, "deployment.workflow", "builds"),
		spec("deployment.gitlab_ci", "GitLab CI", []string{".gitlab-ci.yml"}, []string{"stages:"}, "deployment.workflow", "builds"),
		spec("deployment.circleci", "CircleCI", []string{".circleci/config.yml"}, []string{"orbs:"}, "deployment.workflow", "builds"),
		spec("deployment.jenkinsfile", "Jenkinsfile", []string{"jenkinsfile"}, []string{"pipeline {"}, "deployment.workflow", "builds"),
		spec("deployment.buildkite", "Buildkite", []string{".buildkite/"}, []string{"plugins:"}, "deployment.workflow", "builds"),
		spec("deployment.argo_cd", "Argo CD", nil, []string{"argoproj.io"}, "deployment.target", "deploys_to"),
		spec("deployment.flux", "Flux", nil, []string{"toolkit.fluxcd.io"}, "deployment.target", "deploys_to"),
	}
}

func spec(id, name string, pathTokens, sourceTokens []string, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "deployment",
		Languages:    []string{"yaml", "groovy"},
		Mode:         enrich.ActivationAlways,
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: sourceTokens,
		PathTokens:   pathTokens,
		Tags:         []string{"deployment:" + id},
		Attributes:   map[string]string{"provider": id},
	}
}
