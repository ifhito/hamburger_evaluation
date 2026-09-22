// Package statsworkertest は、統計の再計算のワーカーを、DB を使うテストから決定的に動かすための補助を
// 提供する。統計の再計算は、書き込みの応答のあとでバックグラウンドのワーカーが行うので、書き込みの直後に
// 統計を確かめるテストは、確かめる前に Settle でワーカーを動かして、溜まった依頼を処理しておく。
package statsworkertest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// DB は、UnitOfWork のトランザクションと、依頼の一覧の読み取りの両方に使える接続である
// (*pgx.Conn と *pgxpool.Pool がこれを満たす)。
type DB interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// テストで使う、ワーカーの設定である。1 回で足りる大きさの上限にして、失敗した依頼を、
// 上限の回数だけ再試行できるようにする。
const (
	testBatch       = 100
	testMaxAttempts = 5
)

// NewWorker は、db に対して動く統計の再計算のワーカーを返す。clock は、統計の計算時刻と、
// 失敗した依頼の次の再試行の時刻に使う(テストで時刻を進めるときは、進められる Clock を渡す)。
//
// このワーカーは、バーガーの統計を計算し直すときに、そのバーガーが紐づくショップの集計の再計算の依頼を登録する。
// ショップの集計そのものは、NewShopWorker のワーカー(または SettleAll)が計算する。
func NewWorker(db DB, clock usecase.Clock) *usecase.StatsWorker {
	recalc := usecase.NewBurgerStatsRecalculator(clock)
	return usecase.NewStatsWorker(query.NewBurgerStatsQuery(db), uow.New(db), recalc, usecase.NewShopStatsRecalculator(clock), clock,
		usecase.StatsWorkerConfig{Batch: testBatch, MaxAttempts: testMaxAttempts})
}

// NewShopWorker は、db に対して動く、ショップの集計の再計算のワーカーを返す(clock の使い方は NewWorker と同じ)。
func NewShopWorker(db DB, clock usecase.Clock) *usecase.ShopStatsWorker {
	return usecase.NewShopStatsWorker(query.NewShopStatsQuery(db), uow.New(db), usecase.NewShopStatsRecalculator(clock), clock,
		usecase.StatsWorkerConfig{Batch: testBatch, MaxAttempts: testMaxAttempts})
}

// Settle は、再計算に成功する依頼がなくなるまで、ワーカーを繰り返し動かす。書き込みの直後に統計を
// 確かめるテストが、確かめる前に呼ぶ。再計算に失敗して待ち時間の中にある依頼は、残ったままになる。
func Settle(ctx context.Context, t *testing.T, worker *usecase.StatsWorker) {
	t.Helper()
	for i := 0; i < 100; i++ {
		n, err := worker.RunOnce(ctx)
		if err != nil {
			t.Fatalf("統計のワーカーの RunOnce が失敗した: %v", err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("統計のワーカーを 100 回動かしても、再計算の依頼がなくならない")
}

// SettleShops は、ショップの集計の依頼がなくなるまで、ショップの集計のワーカーを繰り返し動かす。
func SettleShops(ctx context.Context, t *testing.T, worker *usecase.ShopStatsWorker) {
	t.Helper()
	for i := 0; i < 100; i++ {
		n, err := worker.RunOnce(ctx)
		if err != nil {
			t.Fatalf("ショップの集計のワーカーの RunOnce が失敗した: %v", err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("ショップの集計のワーカーを 100 回動かしても、再計算の依頼がなくならない")
}

// SettleAll は、バーガーの統計の依頼を処理し(ショップの集計の依頼が登録される)、続けて、ショップの集計の依頼を
// 処理する。書き込みの直後に、バーガーの統計とショップの集計の両方を確かめるテストが、確かめる前に呼ぶ。
func SettleAll(ctx context.Context, t *testing.T, db DB, clock usecase.Clock) {
	t.Helper()
	Settle(ctx, t, NewWorker(db, clock))
	SettleShops(ctx, t, NewShopWorker(db, clock))
}
