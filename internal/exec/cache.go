package exec

import "context"

type cacheKey struct{}

// WithCache controls whether workspace cache writes (YAML files and .tld.lock)
// are performed by exec helpers. Cache writes are enabled by default so
// existing workspace-first callers keep their behavior; DB-only sessions
// disable them so commands operate purely on the target store.
func WithCache(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, cacheKey{}, enabled)
}

// cacheEnabled reports whether workspace cache writes are allowed. A context
// without an explicit decision keeps the historic write-through behavior.
func cacheEnabled(ctx context.Context) bool {
	enabled, ok := ctx.Value(cacheKey{}).(bool)
	return !ok || enabled
}
