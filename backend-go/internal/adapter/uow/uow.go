// Package uow は、usecase.UnitOfWork を pgx のトランザクションで実装する。
//
// UnitOfWork(作業のひとまとまり)は、ここからここまでの書き込みと読み取りを、まとめて 1 つの
// トランザクションにする範囲を、usecase が指定する仕組みである。途中でエラーになれば全体を取り消す
// (rollback)ので、レビューの保存と統計の再計算のような複数の書き込みが、片方だけ反映されることがない。
//
// 1 つのトランザクションに結び付けた、書き込み(adapter/repository の repository を domain の
// 書き込みオブジェクトで包んだもの)と、読み取り(adapter/query の BurgerStatsQuery)を作って
// usecase に渡す。repository と query の両方を組み合わせるのは、このパッケージだけである。
// repository と query が互いに依存すると、書き込みと読み取りを別々に差し替えたりテストしたり
// できなくなるため、両者は互いを知らないまま保ち、つなぐ役目をここに集めている。
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

// beginner は、トランザクションを開始できる接続である。本番の接続プール(*pgxpool.Pool)と、
// テストの単一の接続(*pgx.Conn)のどちらも満たす。
type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// UnitOfWork は、usecase.UnitOfWork(ここからここまでをまとめて 1 つのトランザクションにする範囲を、
// usecase が指定する仕組み)を、pgx のトランザクションで実装したものである。
type UnitOfWork struct {
	db beginner
}

// New は db（通常は共有の pgx pool）をラップする。
func New(db beginner) *UnitOfWork {
	return &UnitOfWork{db: db}
}

var _ usecase.UnitOfWork = (*UnitOfWork)(nil)

// Do は、トランザクションを開始して fn を実行し、fn がエラーを返したら rollback し、成功したら
// commit する。fn に渡す Tx の書き込みと読み取りは、すべてこのトランザクションで行われる。
//
// repository の中には、複数の文を 1 つにまとめるために、自分でもトランザクションを開始する
// 書き込みがある(写真つきのレビュー編集など)。すでに開始済みのトランザクションの中でそれを
// 呼ぶと、pgx は新しいトランザクションではなくセーブポイント(SAVEPOINT。トランザクションの
// 途中に打つ、部分的な巻き戻し用の目印)を作る。したがって、その書き込みが失敗しても、外側の
// トランザクション全体の中で矛盾なく巻き戻る。
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
		// 確認待ちの signup: 書き込みと読み取りの両方が、同じトランザクションに結び付く。
		SignupVerifications: domain.NewSignupVerifications(repository.NewSignupVerificationRepository(pgxTx)),
		PendingSignups:      query.NewSignupVerificationQuery(pgxTx),
		// OAuth の許可: 退会のとき、ユーザーの論理削除と同じトランザクションで、許可(と、発行済みのトークン)を取り消す。
		OAuthGrants: domain.NewOAuthGrants(repository.NewOAuthGrantRepository(pgxTx)),
		// 外部のサービスのアカウントとの結び付き: 外部のサービスでの新規登録で、ユーザーの作成と同じトランザクションで記録する。
		UserIdentities: domain.NewUserIdentities(repository.NewUserIdentityRepository(pgxTx)),
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := pgxTx.Commit(ctx); err != nil {
		return fmt.Errorf("unit of work: commit: %w", err)
	}
	return nil
}
