package core

import (
	"context"
	"github.com/olyamironova/exchange-engine/internal/port"
)

func withTx(ctx context.Context, repo port.Repository, fn func(port.Tx) error) error {
	tx, err := repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
