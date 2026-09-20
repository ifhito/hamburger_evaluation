package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
)

// beginnerDBTX は、トランザクションを扱う repository（reviews、users）の
// 接続への依存である。sqlc のクエリ面に Begin を加えたもので、各書き込みが
// その書き込みと burger_stats の再計算を 1 つのトランザクションで包める
// ようにする。*pgxpool.Pool と *pgx.Conn のどちらもこれを満たす。
type beginnerDBTX interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// withTx は、tx スコープの Queries を使って fn を単一のトランザクション内で
// 実行する。Begin、fn、Commit の順に行い、いずれかが失敗したら rollback
// する。op は begin/commit のエラーの接頭辞になる（"<op>: begin: ..."）。
// fn は自身のエラーを自分でラップする。
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
