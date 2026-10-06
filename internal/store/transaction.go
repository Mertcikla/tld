package store

import (
	"context"
	"fmt"

	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
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
	return a.Store.legacy.RunInTransaction(ctx, func(txCtx context.Context, txStore *app.Store) error {
		txAdapter := *a
		txAdapter.Store = &SQLiteStore{legacy: txStore}
		return fn(txCtx, &txAdapter)
	})
}
