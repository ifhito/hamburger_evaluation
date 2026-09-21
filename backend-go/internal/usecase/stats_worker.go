package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// StatsWorkerConfig は、統計の再計算のワーカーの設定である。
type StatsWorkerConfig struct {
	// Batch は、1 回の RunOnce で取り出す依頼の上限の件数である(1 以上)。
	Batch int
	// MaxAttempts は、1 つの依頼を、失敗しながら再試行する上限の回数である(1 以上)。
	// 上限に達した依頼は打ち切りで、行は残るが、再計算の対象から外れる(error のログが出る)。
	MaxAttempts int
}

// StatsWorker は、「再計算の依頼」が溜まったバーガーの統計を、あとから計算し直すワーカーの、
// 1 サイクル分の手順である。書き込み(レビューの投稿・編集・削除、退会)は、統計を計算せず、依頼を
// 登録するだけなので、統計の計算はここで行う。周期の実行(ticker・goroutine・停止)は、この型を
// 呼ぶ側(infra)が担い、テストは RunOnce を直接呼んで、周期に頼らずに決定的に動かせる。
type StatsWorker struct {
	query  BurgerStatsQuery
	uow    UnitOfWork
	recalc *BurgerStatsRecalculator
	clock  Clock
	cfg    StatsWorkerConfig
}

// NewStatsWorker は StatsWorker を返す。query は、トランザクションの外で依頼の一覧を読むための
// ものである(依頼ごとの再計算は、uow の中で、そのトランザクションの読み取りを使う)。
func NewStatsWorker(query BurgerStatsQuery, uow UnitOfWork, recalc *BurgerStatsRecalculator, clock Clock, cfg StatsWorkerConfig) *StatsWorker {
	return &StatsWorker{query: query, uow: uow, recalc: recalc, clock: clock, cfg: cfg}
}

// RunOnce は、再計算の時期が来ている依頼を、上限 Batch 件まで取り出し、バーガーごとに再計算する。
// 再計算に成功したバーガーの数を返す。
//
// 依頼ごとに別のトランザクションで、「burger の行をロック → 統計を計算して保存 → 依頼を消す」を
// 行う。依頼を消すのは、取り出したときの version と一致する場合だけである(比較つき削除)。
// 再計算の最中に、同じバーガーへの新しい書き込みが入ると、その依頼の version が進むので、依頼は
// 消えずに残り、次の RunOnce で最新の状態になる。再計算の間、依頼の行はロックしないので、書き込みを
// 待たせない。
//
// 1 つのバーガーの再計算が失敗しても、ほかのバーガーは続けて処理する。失敗は、依頼に記録し(失敗の
// 回数を増やし、待ち時間を倍にしながら次の再試行の時刻を進める)、error のログを出す。失敗の回数が
// MaxAttempts に達したら、打ち切りとして、その旨の error のログを出す(行は残す)。
// ctx が取り消されたときは、その時点で止め、ctx のエラーを返す。
func (w *StatsWorker) RunOnce(ctx context.Context) (int, error) {
	requests, err := w.query.ListDueRecalcRequests(ctx, w.clock.Now(), w.cfg.MaxAttempts, w.cfg.Batch)
	if err != nil {
		return 0, fmt.Errorf("stats worker: %w", err)
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
func (w *StatsWorker) recalculate(ctx context.Context, req domain.RecalcRequest) error {
	return w.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if err := w.recalc.Recalculate(ctx, tx, req.BurgerID); err != nil {
			return err
		}
		// 消せなかった(false)のは、再計算の最中に新しい書き込みが入ったということで、正常である。
		_, err := tx.BurgerStats.CompleteRecalc(ctx, req.BurgerID, req.Version)
		return err
	})
}

// recordFailure は、失敗した依頼に、失敗の内容を記録し、ログに出す。記録そのものが失敗しても、
// ほかの依頼の処理は止めない(その依頼は、次のサイクルで再試行される)。
func (w *StatsWorker) recordFailure(ctx context.Context, req domain.RecalcRequest, cause error) {
	failures := req.Attempts + 1
	failure := domain.NewRecalcFailure(failures, cause, w.clock.Now())
	slog.Error("burger stats recalculation failed",
		"burger_id", req.BurgerID, "attempts", failures, "max_attempts", w.cfg.MaxAttempts,
		"next_attempt_at", failure.NextAttemptAt, "error", cause)
	err := w.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		_, err := tx.BurgerStats.RecordRecalcFailure(ctx, req.BurgerID, req.Version, failure)
		return err
	})
	if err != nil {
		slog.Error("burger stats recalculation failure could not be recorded",
			"burger_id", req.BurgerID, "error", err)
		return
	}
	if failures >= w.cfg.MaxAttempts {
		slog.Error("burger stats recalculation gave up",
			"burger_id", req.BurgerID, "attempts", failures, "error", cause)
	}
}
