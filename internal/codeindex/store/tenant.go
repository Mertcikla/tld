package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// repositoryOwned reports whether a repository belongs to the request's
// organisation. It is always true when scope is disabled (single-tenant), so
// destructive operations keep working in the self-hosted case.
func (s *Store) repositoryOwned(ctx context.Context, repositoryID string) (bool, error) {
	t := scope(ctx)
	if !t.on {
		return true, nil
	}
	var count int
	if err := s.bun.NewRaw(`SELECT COUNT(*) FROM codeindex_repositories WHERE id = ? AND org_id = ?`, repositoryID, t.orgID).Scan(ctx, &count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// tenantScope is the organisation a codeindex request is confined to. When no
// organisation is present (the self-hosted single-tenant case) scope is
// disabled: rows are written with a NULL org_id and reads are unfiltered, which
// preserves the pre-multi-tenant behaviour.
type tenantScope struct {
	orgID uuid.UUID
	on    bool
}

// scope reads the tenant organisation injected by the API auth middleware.
func scope(ctx context.Context) tenantScope {
	id := app.TenantOrgIDFromCtx(ctx)
	if id == uuid.Nil {
		return tenantScope{}
	}
	return tenantScope{orgID: id, on: true}
}

// value returns the org_id to persist for a new row, or nil when scoped to
// nothing (single-tenant). It is passed as an INSERT argument.
func (t tenantScope) value() any {
	if !t.on {
		return nil
	}
	return t.orgID
}

// clause returns " AND <column> = ?" plus its argument so a caller can append
// it to a WHERE. It is empty when scope is disabled.
func (t tenantScope) clause(column string) (string, []any) {
	if !t.on {
		return "", nil
	}
	return " AND " + column + " = ?", []any{t.orgID}
}

// conflictWhere prevents a scoped upsert from modifying a row owned by another
// organisation, including unscoped rows. The condition is evaluated atomically
// by the database rather than relying on a separate ownership check.
func (t tenantScope) conflictWhere(column string) (string, []any) {
	if !t.on {
		return "", nil
	}
	return " WHERE " + column + " = ?", []any{t.orgID}
}
