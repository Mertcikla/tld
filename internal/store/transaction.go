package store

import (
	"context"
	"fmt"

	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

var _ api.TransactionalStore = (*APIAdapter)(nil)

func (a *APIAdapter) RunInTransaction(ctx context.Context, fn func(context.Context, api.Store) error) error {
	if a == nil || a.Store == nil || a.Store.DB() == nil {
		return fmt.Errorf("transactional store is not configured")
	}
	if a.Store.Dialect() != dbrepo.DialectSQLite {
		return fmt.Errorf("transactional Mermaid import for %s: %w", a.Store.Dialect(), api.ErrUnimplemented)
	}
	if _, err := a.Store.DB().ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = a.Store.DB().ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if err := fn(ctx, a); err != nil {
		return err
	}
	if _, err := a.Store.DB().ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}
