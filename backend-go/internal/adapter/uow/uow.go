// Package uow は、usecase.UnitOfWork を pgx のトランザクションで実装する。
//
// トランザクションに束縛された書き込み（adapter/repository の repository を、domain の
// 書き込みオブジェクトで包んだもの）と読み取り（adapter/query の BurgerStatsQuery）を、
// 1 つのトランザクションから作って usecase に渡す。repository と query の両方を組み合わせる
// のは、このパッケージだけで、repository と query は互いに依存しない。
package uow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// beginner は、トランザクションを開始できる接続である。*pgxpool.Pool と *pgx.Conn の
// どちらもこれを満たす。
type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// UnitOfWork は、usecase.UnitOfWork の pgx による実装である。
type UnitOfWork struct {
	db beginner
}

// New は db（通常は共有の pgx pool）をラップする。
func New(db beginner) *UnitOfWork {
	return &UnitOfWork{db: db}
}

var _ usecase.UnitOfWork = (*UnitOfWork)(nil)

// Do は、トランザクションを開始して fn を実行し、fn がエラーを返したら rollback し、
// 成功したら commit する。fn に渡す Tx の書き込みと読み取りは、すべてこのトランザクションで行う。
// repository の中で複数の文をまとめる withTx は、pgx のトランザクションの中では savepoint に
// なり、全体のトランザクションの中で原子的に働く。
func (u *UnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, tx usecase.Tx) error) error {
	pgxTx, err := u.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unit of work: begin: %w", err)
	}
	defer func() { _ = pgxTx.Rollback(ctx) }()
	tx := usecase.Tx{
		Reviews:     domain.NewReviews(repository.NewReviewRepository(pgxTx)),
		Users:       domain.NewUsers(repository.NewUserRepository(pgxTx)),
		BurgerStats: domain.NewBurgerStats(repository.NewBurgerStatRepository(pgxTx)),
		Stats:       query.NewBurgerStatsQuery(pgxTx),
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := pgxTx.Commit(ctx); err != nil {
		return fmt.Errorf("unit of work: commit: %w", err)
	}
	return nil
}
