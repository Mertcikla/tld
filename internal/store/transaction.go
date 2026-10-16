package store

import (
	"context"
	"fmt"

	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
)

var _ api.TransactionalStore = (*APIAdapter)(nil)

func (a *APIAdapter) RunInTransaction(ctx context.Context, fn func(context.Context, api.Store) error) error {
	if a == nil || a.Store == nil || a.Store.DB() == nil {
		return fmt.Errorf("transactional store is not configured")
	}
	return a.Store.RunInTransaction(ctx, func(txCtx context.Context, txStore *app.Store) error {
		txAdapter := *a
		txAdapter.Store = txStore
		return fn(txCtx, &txAdapter)
	})
}
