package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
)

// beginnerDBTX is the connection dependency of the transactional
// repositories (reviews, users): the sqlc query surface plus Begin, so
// each write can wrap the write and the burger_stats recalculation in one
// transaction. Both *pgxpool.Pool and *pgx.Conn satisfy it.
type beginnerDBTX interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// withTx runs fn with a tx-scoped Queries inside a single transaction:
// Begin, fn, Commit, with a rollback on any failure. op prefixes the
// begin/commit errors ("<op>: begin: ..."); fn wraps its own errors.
func withTx(ctx context.Context, db beginnerDBTX, op string, fn func(q *sqlcgen.Queries) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(sqlcgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}
