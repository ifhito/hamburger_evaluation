package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// newShopStatsWorker は、フェイクの UnitOfWork とショップの集計のフェイクで、DB なしに動く ShopStatsWorker を返す。
func newShopStatsWorker(shopStats *uowtest.ShopStats, clock usecase.Clock, cfg usecase.StatsWorkerConfig) (*usecase.ShopStatsWorker, *uowtest.UoW) {
	uow := &uowtest.UoW{ShopStats: shopStats}
	return usecase.NewShopStatsWorker(shopStats, uow, usecase.NewShopStatsRecalculator(clock), clock, cfg), uow
}

func shopDueOf(requests ...domain.ShopRecalcRequest) func(context.Context, time.Time, int, int) ([]domain.ShopRecalcRequest, error) {
	return func(context.Context, time.Time, int, int) ([]domain.ShopRecalcRequest, error) { return requests, nil }
}

// TestShopStatsRecalculatorRequestForBurger は、バーガーの統計を計算し直したあと、そのバーガーが紐づくショップの
// 再計算の依頼が、同じトランザクションの中で、shop_id の昇順に登録されることを確かめる。
func TestShopStatsRecalculatorRequestForBurger(t *testing.T) {
	ctx := context.Background()
	request := func(shopStats *uowtest.ShopStats) error {
		uow := &uowtest.UoW{ShopStats: shopStats}
		return uow.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
			return usecase.NewShopStatsRecalculator(uowtest.Clock{}).RequestRecalculationForBurger(ctx, tx, uid.N(7))
		})
	}

	t.Run("バーガーが紐づくすべてのショップの依頼を、shop_id の昇順で登録する(読み取りが逆順でも、昇順に並べ直す)", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{BurgerShops: func(context.Context, string) ([]string, error) {
			return []string{uid.N(9), uid.N(2), uid.N(5)}, nil
		}}
		if err := request(shopStats); err != nil {
			t.Fatalf("RequestRecalculationForBurger returned error: %v", err)
		}
		want := []string{"burger-shops:" + uid.N(7), "request:" + uid.N(2), "request:" + uid.N(5), "request:" + uid.N(9)}
		if !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作 = %v, want %v", shopStats.Ops, want)
		}
	})

	t.Run("ショップに紐づかないバーガーは、依頼を登録せず、成功する", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{}
		if err := request(shopStats); err != nil {
			t.Fatalf("RequestRecalculationForBurger returned error: %v", err)
		}
		if want := []string{"burger-shops:" + uid.N(7)}; !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作 = %v, want %v", shopStats.Ops, want)
		}
	})

	t.Run("ショップの一覧を読めない・依頼を登録できないときは、エラーを返す", func(t *testing.T) {
		boom := errors.New("boom")
		if err := request(&uowtest.ShopStats{BurgerShops: func(context.Context, string) ([]string, error) { return nil, boom }}); !errors.Is(err, boom) {
			t.Errorf("読み取りの失敗: err = %v, want boom", err)
		}
		shopStats := &uowtest.ShopStats{
			BurgerShops: func(context.Context, string) ([]string, error) { return []string{uid.N(1)}, nil },
			RequestErr:  boom,
		}
		if err := request(shopStats); !errors.Is(err, boom) {
			t.Errorf("登録の失敗: err = %v, want boom", err)
		}
	})
}

// TestShopStatsRecalculatorRecalculate は、ショップの集計の再計算が「ロック → 元データの読み取り → 保存」の順で、
// domain の計算(CalculateShopStat)の結果を、Clock の時刻で保存することを確かめる。
func TestShopStatsRecalculatorRecalculate(t *testing.T) {
	ctx := context.Background()
	recalculate := func(shopStats *uowtest.ShopStats, clock usecase.Clock) error {
		uow := &uowtest.UoW{ShopStats: shopStats}
		return uow.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
			return usecase.NewShopStatsRecalculator(clock).Recalculate(ctx, tx, uid.N(3))
		})
	}

	t.Run("ロック → 元データの読み取り → 保存の順に行い、保存するのは、CalculateShopStat の結果(時刻は Clock をマイクロ秒に切り詰めたもの)である", func(t *testing.T) {
		clockTime := time.Date(2026, 9, 22, 12, 0, 0, 123456789, time.UTC)
		facts := []domain.ShopReviewFact{
			{ID: uid.N(1), Rating: 5, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), PhotoKey: strPtr("a.jpg")},
			{ID: uid.N(2), Rating: 1, CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{1, 5, 3}}},
		}
		shopStats := &uowtest.ShopStats{Facts: func(context.Context, string) ([]domain.ShopReviewFact, error) { return facts, nil }}
		if err := recalculate(shopStats, uowtest.Clock{T: clockTime}); err != nil {
			t.Fatalf("Recalculate returned error: %v", err)
		}
		if want := []string{"lock:" + uid.N(3), "facts:" + uid.N(3), "save:" + uid.N(3)}; !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", shopStats.Ops, want)
		}
		if len(shopStats.Saved) != 1 {
			t.Fatalf("保存された集計 = %d 件, want 1", len(shopStats.Saved))
		}
		want := domain.CalculateShopStat(uid.N(3), facts, clockTime.Truncate(time.Microsecond))
		if got := shopStats.Saved[0]; !reflect.DeepEqual(got, want) {
			t.Errorf("保存された集計 = %+v, want %+v", got, want)
		}
	})

	t.Run("レビューが 0 件でも、件数 0 の集計を保存する", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{}
		if err := recalculate(shopStats, uowtest.Clock{}); err != nil {
			t.Fatalf("Recalculate returned error: %v", err)
		}
		if len(shopStats.Saved) != 1 || shopStats.Saved[0].ReviewCount != 0 || shopStats.Saved[0].AverageRating != nil {
			t.Errorf("保存された集計 = %+v, want 件数 0・平均なし", shopStats.Saved)
		}
	})

	t.Run("ショップがなくなっていたときは、何も読まず・保存せず、成功する(依頼が空振りしても、失敗にならない)", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{LockErr: domain.ErrShopNotFound}
		if err := recalculate(shopStats, uowtest.Clock{}); err != nil {
			t.Fatalf("Recalculate returned error: %v", err)
		}
		if want := []string{"lock:" + uid.N(3)}; !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作 = %v, want ロックだけ %v", shopStats.Ops, want)
		}
	})

	t.Run("ロック・読み取り・保存の失敗は、エラーとして返す", func(t *testing.T) {
		boom := errors.New("boom")
		for name, shopStats := range map[string]*uowtest.ShopStats{
			"ロック":  {LockErr: boom},
			"読み取り": {Facts: func(context.Context, string) ([]domain.ShopReviewFact, error) { return nil, boom }},
			"保存":   {SaveErr: boom},
		} {
			if err := recalculate(shopStats, uowtest.Clock{}); !errors.Is(err, boom) {
				t.Errorf("%s の失敗: err = %v, want boom", name, err)
			}
		}
	})
}

func TestShopStatsWorkerRunOnce(t *testing.T) {
	ctx := context.Background()

	t.Run("依頼がなければ、何も再計算せず 0 件を返す", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{}
		worker, uow := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 0 {
			t.Fatalf("RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if want := []string{"due"}; !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作 = %v, want 依頼の一覧の読み取りだけ %v", shopStats.Ops, want)
		}
		if uow.Commits != 0 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 0/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("依頼の一覧の読み取りに、現在時刻・失敗の上限・1 回の上限件数を渡す", func(t *testing.T) {
		clockTime := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
		var gotNow time.Time
		var gotMax, gotBatch int
		shopStats := &uowtest.ShopStats{Due: func(_ context.Context, now time.Time, maxAttempts, batch int) ([]domain.ShopRecalcRequest, error) {
			gotNow, gotMax, gotBatch = now, maxAttempts, batch
			return nil, nil
		}}
		worker, _ := newShopStatsWorker(shopStats, uowtest.Clock{T: clockTime}, usecase.StatsWorkerConfig{Batch: 7, MaxAttempts: 3})
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if !gotNow.Equal(clockTime) || gotMax != 3 || gotBatch != 7 {
			t.Errorf("読み取りの引数 = (%v, %d, %d), want (%v, 3, 7)", gotNow, gotMax, gotBatch, clockTime)
		}
	})

	t.Run("依頼ごとに別のトランザクションで、ロック → 元データの読み取り → 保存 → 依頼の削除(version つき)を行う", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{Due: shopDueOf(
			domain.ShopRecalcRequest{ShopID: uid.N(3), Version: 11},
			domain.ShopRecalcRequest{ShopID: uid.N(5), Version: 12},
		)}
		worker, uow := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 2 {
			t.Fatalf("RunOnce = (%d, %v), want (2, nil)", n, err)
		}
		want := []string{
			"due",
			"lock:" + uid.N(3), "facts:" + uid.N(3), "save:" + uid.N(3), "complete:" + uid.N(3) + "@11",
			"lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5), "complete:" + uid.N(5) + "@12",
		}
		if !reflect.DeepEqual(shopStats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", shopStats.Ops, want)
		}
		if uow.Commits != 2 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 2/0（依頼ごとに別のトランザクション）", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("依頼を消せなくても(再計算の最中に、同じショップの新しい依頼が入った)、失敗ではなく、依頼は残って次に処理される", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{
			Due:              shopDueOf(domain.ShopRecalcRequest{ShopID: uid.N(5), Version: 1}),
			CompleteRejected: true,
		}
		logs := captureLogs(t)
		worker, uow := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if uow.Commits != 1 || len(shopStats.Failures) != 0 || logs.Len() != 0 {
			t.Errorf("commits = %d, failures = %v, logs = %q, want 集計は保存して commit し、失敗もログも出さない", uow.Commits, shopStats.Failures, logs)
		}
	})

	t.Run("1 つのショップの再計算が失敗しても、ほかのショップは処理し、失敗した依頼に失敗を記録する", func(t *testing.T) {
		boom := errors.New("元データを読めない")
		clockTime := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
		shopStats := &uowtest.ShopStats{
			Due: shopDueOf(
				domain.ShopRecalcRequest{ShopID: uid.N(3), Version: 21, Attempts: 2},
				domain.ShopRecalcRequest{ShopID: uid.N(5), Version: 22},
			),
			Facts: func(_ context.Context, shopID string) ([]domain.ShopReviewFact, error) {
				if shopID == uid.N(3) {
					return nil, boom
				}
				return nil, nil
			},
		}
		logs := captureLogs(t)
		worker, uow := newShopStatsWorker(shopStats, uowtest.Clock{T: clockTime}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)（失敗したショップは数えない）", n, err)
		}
		if len(shopStats.Saved) != 1 || shopStats.Saved[0].ShopID != uid.N(5) {
			t.Errorf("保存された集計 = %+v, want 失敗しなかったショップ %s だけ", shopStats.Saved, uid.N(5))
		}
		if len(shopStats.Failures) != 1 {
			t.Fatalf("記録された失敗 = %+v, want 1 件", shopStats.Failures)
		}
		got := shopStats.Failures[0]
		// 失敗はこれで 3 回目(それまでの 2 回 + 今回)。待ち時間は 2 秒 × 2^(3-1) = 8 秒。
		if got.ShopID != uid.N(3) || got.Version != 21 {
			t.Errorf("失敗の対象 = (%s, %d), want 取り出した version の依頼 (%s, 21)", got.ShopID, got.Version, uid.N(3))
		}
		if want := clockTime.Add(8 * time.Second); !got.Failure.NextAttemptAt.Equal(want) {
			t.Errorf("次の再試行 = %v, want %v", got.Failure.NextAttemptAt, want)
		}
		if !strings.Contains(got.Failure.Reason, "元データを読めない") {
			t.Errorf("理由 = %q, want 原因の文言を含む", got.Failure.Reason)
		}
		if uow.Rollbacks != 1 || uow.Commits != 2 {
			t.Errorf("commit/rollback = %d/%d, want 2/1", uow.Commits, uow.Rollbacks)
		}
		if !strings.Contains(logs.String(), "shop stats recalculation failed") || !strings.Contains(logs.String(), uid.N(3)) || strings.Contains(logs.String(), "gave up") {
			t.Errorf("error のログ = %q, want 失敗したショップの id を含み、打ち切りではない", logs)
		}
	})

	t.Run("失敗の回数が上限に達したら、打ち切りとして error のログを出す", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{
			Due:   shopDueOf(domain.ShopRecalcRequest{ShopID: uid.N(3), Version: 1, Attempts: 4}),
			Facts: func(context.Context, string) ([]domain.ShopReviewFact, error) { return nil, errors.New("boom") },
		}
		logs := captureLogs(t)
		worker, _ := newShopStatsWorker(shopStats, uowtest.Clock{}, usecase.StatsWorkerConfig{Batch: 20, MaxAttempts: 5})
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if len(shopStats.Failures) != 1 || !strings.Contains(logs.String(), "gave up") {
			t.Errorf("failures = %+v, logs = %q, want 失敗の記録(行は残す)と、打ち切りのログ", shopStats.Failures, logs)
		}
	})

	t.Run("失敗の記録そのものが失敗しても、ほかのショップの処理は続ける", func(t *testing.T) {
		shopStats := &uowtest.ShopStats{
			Due: shopDueOf(domain.ShopRecalcRequest{ShopID: uid.N(3), Version: 1}, domain.ShopRecalcRequest{ShopID: uid.N(5), Version: 2}),
			Facts: func(_ context.Context, shopID string) ([]domain.ShopReviewFact, error) {
				if shopID == uid.N(3) {
					return nil, errors.New("boom")
				}
				return nil, nil
			},
			FailureErr: errors.New("記録できない"),
		}
		logs := captureLogs(t)
		worker, _ := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 || !strings.Contains(logs.String(), "could not be recorded") {
			t.Fatalf("RunOnce = (%d, %v), logs = %q, want (1, nil) と、記録できなかったことのログ", n, err, logs)
		}
	})

	t.Run("依頼の一覧を読めなければ、エラーを返す", func(t *testing.T) {
		boom := errors.New("db down")
		shopStats := &uowtest.ShopStats{Due: func(context.Context, time.Time, int, int) ([]domain.ShopRecalcRequest, error) { return nil, boom }}
		worker, _ := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		if _, err := worker.RunOnce(ctx); !errors.Is(err, boom) {
			t.Fatalf("RunOnce error = %v, want wrapped %v", err, boom)
		}
	})

	t.Run("停止(ctx の取り消し)を受けたら、そこで止め、失敗としては記録しない", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		shopStats := &uowtest.ShopStats{
			Due: shopDueOf(domain.ShopRecalcRequest{ShopID: uid.N(3), Version: 1}, domain.ShopRecalcRequest{ShopID: uid.N(5), Version: 2}),
			Facts: func(context.Context, string) ([]domain.ShopReviewFact, error) {
				cancel()
				return nil, cancelCtx.Err()
			},
		}
		logs := captureLogs(t)
		worker, _ := newShopStatsWorker(shopStats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(cancelCtx)
		if !errors.Is(err, context.Canceled) || n != 0 || len(shopStats.Failures) != 0 || strings.Contains(logs.String(), "failed") {
			t.Fatalf("RunOnce = (%d, %v), failures = %+v, logs = %q, want (0, context.Canceled)・失敗は記録しない", n, err, shopStats.Failures, logs)
		}
		for _, op := range shopStats.Ops {
			if op == "lock:"+uid.N(5) {
				t.Errorf("停止のあとで、次のショップの再計算を始めた: %v", shopStats.Ops)
			}
		}
	})
}

// TestStatsWorkerDoesNotTouchShopStats は、バーガーの統計のワーカー(StatsWorker)が、ショップの集計に一切
// 触れないことを確かめる(依頼のレビュー指摘への対応: ワーカーからワーカーへの連鎖をやめ、書き込みの経路が
// 直接、両方の依頼を登録する形にした。バーガーの統計の再計算の成功・失敗が、ショップ側の都合(登録の失敗など)に
// 引きずられない)。
func TestStatsWorkerDoesNotTouchShopStats(t *testing.T) {
	ctx := context.Background()
	stats := &uowtest.Stats{Due: dueOf(domain.RecalcRequest{BurgerID: uid.N(7), Version: 3})}
	shopStats := &uowtest.ShopStats{RequestErr: errors.New("ショップ側が壊れていても、バーガーの再計算には無関係")}
	uow := &uowtest.UoW{Stats: stats, ShopStats: shopStats}
	worker := usecase.NewStatsWorker(stats, uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), uowtest.Clock{}, defaultWorkerConfig)
	n, err := worker.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("RunOnce = (%d, %v), want (1, nil)(ショップ側の RequestErr に影響されない)", n, err)
	}
	if len(shopStats.Ops) != 0 {
		t.Errorf("ショップの操作 = %v, want なし(バーガーのワーカーは、ショップの集計に触れない)", shopStats.Ops)
	}
	if len(stats.Failures) != 0 {
		t.Errorf("バーガーの失敗 = %+v, want なし", stats.Failures)
	}
}
