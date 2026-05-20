package config

import (
	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher {
	return append(pattern.FromSpecs(Specs()), ConcreteBindings())
}

func Specs() []pattern.Spec {
	return []pattern.Spec{
		native("ts.process_env", "TypeScript process.env", "typescript", "process.env", "process.env"),
		lib("ts.dotenv", "TypeScript dotenv", "typescript", "dotenv", "dotenv.config"),
		lib("ts.next_env", "Next.js env", "typescript", "next", "NEXT_PUBLIC_"),
		lib("ts.vite_env", "Vite env", "typescript", "vite", "import.meta.env"),
		native("go.os_getenv", "Go os.Getenv", "go", "os.Getenv", "os"),
		lib("go.viper", "Go Viper", "go", "github.com/spf13/viper", "viper.Get"),
		lib("go.envconfig", "Go envconfig", "go", "github.com/kelseyhightower/envconfig", "envconfig.Process"),
		native("python.os_environ", "Python os.environ", "python", "os.environ", "os"),
		lib("python.pydantic_settings", "Python Pydantic Settings", "python", "pydantic-settings", "BaseSettings"),
		lib("python.dynaconf", "Python Dynaconf", "python", "dynaconf", "Dynaconf"),
		lib("python.django_settings", "Django settings", "python", "django", "django.conf.settings"),
		native("java.system_getenv", "Java System.getenv", "java", "System.getenv", "java.lang.System"),
		lib("java.spring_value", "Spring @Value", "java", "org.springframework.beans.factory.annotation.Value", "@Value"),
		lib("java.spring_configuration_properties", "Spring @ConfigurationProperties", "java", "org.springframework.boot.context.properties.ConfigurationProperties", "@ConfigurationProperties"),
		lib("java.microprofile_config", "MicroProfile Config", "java", "org.eclipse.microprofile.config", "ConfigProvider"),
		native("rust.std_env_var", "Rust std::env::var", "rust", "std::env::var", "std::env"),
		lib("rust.dotenvy", "Rust dotenvy", "rust", "dotenvy", "dotenvy::"),
		lib("rust.config", "Rust config", "rust", "config", "Config::builder"),
		lib("rust.figment", "Rust figment", "rust", "figment", "Figment::"),
		native("cpp.getenv", "C++ std::getenv", "cpp", "std::getenv", "cstdlib"),
		lib("cpp.yaml_cpp", "C++ yaml-cpp", "cpp", "yaml-cpp", "YAML::Load"),
		lib("cpp.nlohmann_json", "C++ nlohmann_json", "cpp", "nlohmann_json", "nlohmann::json"),
		lib("cpp.tomlplusplus", "C++ tomlplusplus", "cpp", "tomlplusplus", "toml::parse"),
	}
}

func native(id, name, language, token, dependency string) pattern.Spec {
	spec := base(id, name, language, dependency, token)
	spec.Mode = enrich.ActivationAlways
	spec.Triggers = nil
	return spec
}

func lib(id, name, language, dependency, token string) pattern.Spec {
	return base(id, name, language, dependency, token)
}

func base(id, name, language, dependency, token string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "config",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "config.env",
		Relationship: "reads_config",
		SourceTokens: []string{token},
		Tags:         []string{"config:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
