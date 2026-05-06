package auth

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.auth0", "TypeScript Auth0", "typescript", "@auth0/auth0-react", "@auth0/", "auth.provider", "uses_identity_provider"),
		spec("ts.cognito", "TypeScript Cognito", "typescript", "@aws-sdk/client-cognito-identity-provider", "CognitoIdentityProviderClient", "auth.provider", "uses_identity_provider"),
		spec("ts.firebase_auth", "TypeScript Firebase Auth", "typescript", "firebase", "firebase/auth", "auth.provider", "uses_identity_provider"),
		spec("ts.clerk", "TypeScript Clerk", "typescript", "@clerk/nextjs", "@clerk/", "auth.provider", "uses_identity_provider"),
		spec("ts.nextauth", "TypeScript NextAuth", "typescript", "next-auth", "next-auth", "auth.provider", "uses_identity_provider"),
		spec("go.jwt", "Go JWT middleware", "go", "github.com/golang-jwt/jwt", "jwt.Parse", "auth.issuer", "trusts_issuer"),
		spec("go.oidc", "Go OIDC clients", "go", "github.com/coreos/go-oidc", "oidc.NewProvider", "auth.issuer", "trusts_issuer"),
		spec("go.auth0_cognito", "Go Auth0/Cognito SDK", "go", "github.com/auth0/go-auth0", "auth0", "auth.provider", "uses_identity_provider"),
		spec("python.pyjwt", "Python PyJWT", "python", "PyJWT", "jwt.decode", "auth.issuer", "trusts_issuer"),
		spec("python.authlib", "Python Authlib", "python", "Authlib", "authlib", "auth.provider", "uses_identity_provider"),
		spec("python.django_auth", "Python Django auth", "python", "Django", "django.contrib.auth", "auth.provider", "authenticates_with"),
		spec("python.fastapi_security", "Python FastAPI security", "python", "fastapi", "fastapi.security", "auth.provider", "authenticates_with"),
		spec("java.spring_security", "Java Spring Security OAuth/OIDC", "java", "spring-security-oauth2-client", "@EnableWebSecurity", "auth.provider", "uses_identity_provider"),
		spec("java.keycloak", "Java Keycloak", "java", "keycloak", "Keycloak", "auth.provider", "uses_identity_provider"),
		spec("java.cognito", "Java Cognito", "java", "software.amazon.awssdk.services.cognitoidentityprovider", "CognitoIdentityProviderClient", "auth.provider", "uses_identity_provider"),
		spec("java.oidc", "Java OIDC", "java", "spring-security-oauth2-jose", "issuer-uri", "auth.issuer", "trusts_issuer"),
		spec("rust.jsonwebtoken", "Rust jsonwebtoken", "rust", "jsonwebtoken", "jsonwebtoken::decode", "auth.issuer", "trusts_issuer"),
		spec("rust.oauth2", "Rust oauth2", "rust", "oauth2", "oauth2::", "auth.provider", "uses_identity_provider"),
		spec("rust.oidc", "Rust OIDC crates", "rust", "openidconnect", "openidconnect::", "auth.issuer", "trusts_issuer"),
		spec("cpp.jwt", "C++ JWT validation", "cpp", "jwt-cpp", "jwt::decode", "auth.issuer", "trusts_issuer"),
		spec("cpp.oidc_jwks", "C++ OIDC/JWKS config", "cpp", "oidc", "jwks_uri", "auth.jwks_endpoint", "trusts_issuer"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "auth",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"auth:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
