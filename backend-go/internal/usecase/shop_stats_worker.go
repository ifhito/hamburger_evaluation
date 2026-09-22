package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopStatsWorker は、「再計算の依頼」が溜まったショップの集計を、あとから計算し直すワーカーの、1 サイクル分の
// 手順である。バーガーの統計のワーカー(StatsWorker)と同じ仕組み(依頼の待ち行列・version の比較つき削除・
// 失敗の再試行と打ち切り)で、依頼の単位がショップである。ショップの依頼は、StatsWorker が、バーガーの統計を
// 計算し直すときに登録する。周期の実行(ticker・goroutine・停止)は、この型を呼ぶ側(infra)が担い、テストは
// RunOnce を直接呼んで、周期に頼らずに決定的に動かせる。
type ShopStatsWorker struct {
	query  ShopStatsQuery
	uow    UnitOfWork
	recalc *ShopStatsRecalculator
	clock  Clock
	cfg    StatsWorkerConfig
}

// NewShopStatsWorker は ShopStatsWorker を返す。query は、トランザクションの外で依頼の一覧を読むためのものである
// (依頼ごとの再計算は、uow の中で、そのトランザクションの読み取りを使う)。設定(Batch・MaxAttempts)は、
// バーガーの統計のワーカーと同じものを使う。
func NewShopStatsWorker(query ShopStatsQuery, uow UnitOfWork, recalc *ShopStatsRecalculator, clock Clock, cfg StatsWorkerConfig) *ShopStatsWorker {
	return &ShopStatsWorker{query: query, uow: uow, recalc: recalc, clock: clock, cfg: cfg}
}

// RunOnce は、再計算の時期が来ている依頼を、上限 Batch 件まで取り出し、ショップごとに再計算する。再計算に成功した
// ショップの数を返す。
//
// 依頼ごとに別のトランザクションで、「ショップの行をロック → 集計を計算して保存 → 依頼を消す」を行う。依頼を
// 消すのは、取り出したときの version と一致する場合だけである(比較つき削除)。再計算の最中に、同じショップの
// 新しい依頼(別のバーガーの書き込みなど)が入ると、その依頼の version が進むので、依頼は消えずに残り、次の
// RunOnce で最新の状態になる。再計算の間、依頼の行はロックしないので、依頼の登録を待たせない。
//
// 1 つのショップの再計算が失敗しても、ほかのショップは続けて処理する。失敗は、依頼に記録し(失敗の回数を増やし、
// 待ち時間を倍にしながら次の再試行の時刻を進める)、error のログを出す。失敗の回数が MaxAttempts に達したら、
// 打ち切りとして、その旨の error のログを出す(行は残す)。ctx が取り消されたときは、その時点で止め、ctx のエラーを
// 返す。
func (w *ShopStatsWorker) RunOnce(ctx context.Context) (int, error) {
	requests, err := w.query.ListDueShopRecalcRequests(ctx, w.clock.Now(), w.cfg.MaxAttempts, w.cfg.Batch)
	if err != nil {
		return 0, fmt.Errorf("shop stats worker: %w", err)
	}
	done := 0
	for _, req := range requests {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		if err := w.recalculate(ctx, req); err != nil {
			if ctx.Err() != nil {
				// 停止の要求で中断されたので、失敗としては記録しない(依頼は残り、次に処理される)。
				return done, ctx.Err()
			}
			w.recordFailure(ctx, req, err)
			continue
		}
		done++
	}
	return done, nil
}

// recalculate は、1 つの依頼を、1 つのトランザクションで再計算し、依頼を消す。
func (w *ShopStatsWorker) recalculate(ctx context.Context, req domain.ShopRecalcRequest) error {
	return w.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if err := w.recalc.Recalculate(ctx, tx, req.ShopID); err != nil {
			return err
		}
		// 消せなかった(false)のは、再計算の最中に新しい依頼が入ったということで、正常である。
		_, err := tx.ShopStats.CompleteRecalc(ctx, req.ShopID, req.Version)
		return err
	})
}

// recordFailure は、失敗した依頼に、失敗の内容を記録し、ログに出す。記録そのものが失敗しても、ほかの依頼の処理は
// 止めない(その依頼は、次のサイクルで再試行される)。
func (w *ShopStatsWorker) recordFailure(ctx context.Context, req domain.ShopRecalcRequest, cause error) {
	failures := req.Attempts + 1
	failure := domain.NewRecalcFailure(failures, cause, w.clock.Now())
	slog.Error("shop stats recalculation failed",
		"shop_id", req.ShopID, "attempts", failures, "max_attempts", w.cfg.MaxAttempts,
		"next_attempt_at", failure.NextAttemptAt, "error", cause)
	err := w.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		_, err := tx.ShopStats.RecordRecalcFailure(ctx, req.ShopID, req.Version, failure)
		return err
	})
	if err != nil {
		slog.Error("shop stats recalculation failure could not be recorded",
			"shop_id", req.ShopID, "error", err)
		return
	}
	if failures >= w.cfg.MaxAttempts {
		slog.Error("shop stats recalculation gave up",
			"shop_id", req.ShopID, "attempts", failures, "error", cause)
	}
}
