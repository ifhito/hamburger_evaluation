package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// newStatsWorker は、フェイクの UnitOfWork と統計のフェイクで、DB なしに動く StatsWorker を返す。
func newStatsWorker(stats *uowtest.Stats, clock usecase.Clock, cfg usecase.StatsWorkerConfig) (*usecase.StatsWorker, *uowtest.UoW) {
	uow := &uowtest.UoW{Stats: stats}
	return usecase.NewStatsWorker(stats, uow, usecase.NewBurgerStatsRecalculator(clock), clock, cfg), uow
}

// dueOf は、決まった依頼の一覧を返す Due である。
func dueOf(requests ...domain.RecalcRequest) func(context.Context, time.Time, int, int) ([]domain.RecalcRequest, error) {
	return func(context.Context, time.Time, int, int) ([]domain.RecalcRequest, error) { return requests, nil }
}

// captureLogs は、テストの間だけ、slog の出力を集める(終わると元に戻す)。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

var defaultWorkerConfig = usecase.StatsWorkerConfig{Batch: 20, MaxAttempts: 5}

func TestStatsWorkerRunOnce(t *testing.T) {
	ctx := context.Background()

	t.Run("依頼がなければ、何も再計算せず 0 件を返す", func(t *testing.T) {
		stats := &uowtest.Stats{}
		worker, uow := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 0 {
			t.Fatalf("RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if want := []string{"due"}; !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作 = %v, want 依頼の一覧の読み取りだけ %v", stats.Ops, want)
		}
		if uow.Commits != 0 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 0/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("依頼の一覧の読み取りに、現在時刻・失敗の上限・1 回の上限件数を渡す", func(t *testing.T) {
		clockTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		var gotNow time.Time
		var gotMax, gotBatch int
		stats := &uowtest.Stats{Due: func(_ context.Context, now time.Time, maxAttempts, batch int) ([]domain.RecalcRequest, error) {
			gotNow, gotMax, gotBatch = now, maxAttempts, batch
			return nil, nil
		}}
		worker, _ := newStatsWorker(stats, uowtest.Clock{T: clockTime}, usecase.StatsWorkerConfig{Batch: 7, MaxAttempts: 3})
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if !gotNow.Equal(clockTime) || gotMax != 3 || gotBatch != 7 {
			t.Errorf("読み取りの引数 = (%v, %d, %d), want (%v, 3, 7)", gotNow, gotMax, gotBatch, clockTime)
		}
	})

	t.Run("依頼ごとに別のトランザクションで、ロック → 元データの読み取り → 保存 → 依頼の削除(version つき)を行う", func(t *testing.T) {
		stats := &uowtest.Stats{Due: dueOf(
			domain.RecalcRequest{BurgerID: uid.N(3), Version: 11},
			domain.RecalcRequest{BurgerID: uid.N(5), Version: 12},
		)}
		worker, uow := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 2 {
			t.Fatalf("RunOnce = (%d, %v), want (2, nil)", n, err)
		}
		want := []string{
			"due",
			"lock:" + uid.N(3), "facts:" + uid.N(3), "save:" + uid.N(3), "complete:" + uid.N(3) + "@11",
			"lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5), "complete:" + uid.N(5) + "@12",
		}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 2 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 2/0（依頼ごとに別のトランザクション）", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("保存する統計は、Clock の時刻(マイクロ秒に切り詰め)と、domain の計算の結果に一致する", func(t *testing.T) {
		// マイクロ秒より細かい部分は切り詰められる（timestamptz の精度）。
		clockTime := time.Date(2024, 6, 1, 12, 0, 0, 123456789, time.UTC)
		facts := []domain.ReviewFact{
			{Rating: 5, CreatedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{5, 3, 4}}},
			{Rating: 3, CreatedAt: time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC), ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{3}}},
		}
		stats := &uowtest.Stats{
			Due:   dueOf(domain.RecalcRequest{BurgerID: uid.N(5), Version: 1}),
			Facts: func(context.Context, string) ([]domain.ReviewFact, error) { return facts, nil },
		}
		worker, _ := newStatsWorker(stats, uowtest.Clock{T: clockTime}, defaultWorkerConfig)
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if len(stats.Saved) != 1 {
			t.Fatalf("保存された統計 = %d 件, want 1", len(stats.Saved))
		}
		want := domain.CalculateBurgerStat(uid.N(5), facts, clockTime.Truncate(time.Microsecond))
		if got := stats.Saved[0]; got != want {
			t.Errorf("保存された統計 = %+v, want %+v", got, want)
		}
	})

	t.Run("依頼を消せなくても(再計算の最中に新しい書き込みが入った)、失敗ではなく、依頼は残って次に処理される", func(t *testing.T) {
		stats := &uowtest.Stats{
			Due:              dueOf(domain.RecalcRequest{BurgerID: uid.N(5), Version: 1}),
			CompleteRejected: true,
		}
		logs := captureLogs(t)
		worker, uow := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if uow.Commits != 1 || len(stats.Failures) != 0 {
			t.Errorf("commits = %d, failures = %v, want 統計は保存して commit し、失敗は記録しない", uow.Commits, stats.Failures)
		}
		if logs.Len() != 0 {
			t.Errorf("通常のサイクルでログが出た: %s", logs)
		}
	})

	t.Run("1 つのバーガーの再計算が失敗しても、ほかのバーガーは処理し、失敗した依頼に失敗を記録する", func(t *testing.T) {
		boom := errors.New("元データを読めない")
		clockTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		stats := &uowtest.Stats{
			Due: dueOf(
				domain.RecalcRequest{BurgerID: uid.N(3), Version: 21, Attempts: 2},
				domain.RecalcRequest{BurgerID: uid.N(5), Version: 22},
			),
			Facts: func(_ context.Context, burgerID string) ([]domain.ReviewFact, error) {
				if burgerID == uid.N(3) {
					return nil, boom
				}
				return nil, nil
			},
		}
		logs := captureLogs(t)
		worker, uow := newStatsWorker(stats, uowtest.Clock{T: clockTime}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)（失敗したバーガーは数えない）", n, err)
		}
		if len(stats.Saved) != 1 || stats.Saved[0].BurgerID != uid.N(5) {
			t.Errorf("保存された統計 = %+v, want 失敗しなかったバーガー %s だけ", stats.Saved, uid.N(5))
		}
		if len(stats.Failures) != 1 {
			t.Fatalf("記録された失敗 = %+v, want 1 件", stats.Failures)
		}
		got := stats.Failures[0]
		// 失敗はこれで 3 回目(それまでの 2 回 + 今回)。待ち時間は 2 秒 × 2^(3-1) = 8 秒。
		if got.BurgerID != uid.N(3) || got.Version != 21 {
			t.Errorf("失敗の対象 = (%s, %d), want 取り出した version の依頼 (%s, 21)", got.BurgerID, got.Version, uid.N(3))
		}
		if want := clockTime.Add(8 * time.Second); !got.Failure.NextAttemptAt.Equal(want) {
			t.Errorf("次の再試行 = %v, want %v", got.Failure.NextAttemptAt, want)
		}
		if !strings.Contains(got.Failure.Reason, "元データを読めない") {
			t.Errorf("理由 = %q, want 原因の文言を含む", got.Failure.Reason)
		}
		if uow.Rollbacks != 1 || uow.Commits != 2 {
			t.Errorf("commit/rollback = %d/%d, want 2/1（失敗した再計算は rollback、失敗の記録と成功した再計算は commit）", uow.Commits, uow.Rollbacks)
		}
		if !strings.Contains(logs.String(), "burger stats recalculation failed") || !strings.Contains(logs.String(), uid.N(3)) {
			t.Errorf("error のログ = %q, want 失敗したバーガーの id を含む", logs)
		}
		if strings.Contains(logs.String(), "gave up") {
			t.Errorf("まだ上限に達していないのに、打ち切りのログが出た: %s", logs)
		}
	})

	t.Run("失敗の回数が上限に達したら、打ち切りとして error のログを出す", func(t *testing.T) {
		stats := &uowtest.Stats{
			Due:   dueOf(domain.RecalcRequest{BurgerID: uid.N(3), Version: 1, Attempts: 4}),
			Facts: func(context.Context, string) ([]domain.ReviewFact, error) { return nil, errors.New("boom") },
		}
		logs := captureLogs(t)
		worker, _ := newStatsWorker(stats, uowtest.Clock{}, usecase.StatsWorkerConfig{Batch: 20, MaxAttempts: 5})
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if len(stats.Failures) != 1 {
			t.Fatalf("記録された失敗 = %+v, want 1 件（行は残す）", stats.Failures)
		}
		if !strings.Contains(logs.String(), "gave up") {
			t.Errorf("ログ = %q, want 打ち切りの error を含む", logs)
		}
	})

	t.Run("失敗の記録そのものが失敗しても、ほかのバーガーの処理は続ける", func(t *testing.T) {
		stats := &uowtest.Stats{
			Due: dueOf(
				domain.RecalcRequest{BurgerID: uid.N(3), Version: 1},
				domain.RecalcRequest{BurgerID: uid.N(5), Version: 2},
			),
			Facts: func(_ context.Context, burgerID string) ([]domain.ReviewFact, error) {
				if burgerID == uid.N(3) {
					return nil, errors.New("boom")
				}
				return nil, nil
			},
			FailureErr: errors.New("記録できない"),
		}
		logs := captureLogs(t)
		worker, _ := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if !strings.Contains(logs.String(), "could not be recorded") {
			t.Errorf("ログ = %q, want 失敗を記録できなかったことを含む", logs)
		}
	})

	t.Run("依頼の一覧を読めなければ、エラーを返す", func(t *testing.T) {
		boom := errors.New("db down")
		stats := &uowtest.Stats{Due: func(context.Context, time.Time, int, int) ([]domain.RecalcRequest, error) { return nil, boom }}
		worker, _ := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		if _, err := worker.RunOnce(ctx); !errors.Is(err, boom) {
			t.Fatalf("RunOnce error = %v, want wrapped %v", err, boom)
		}
	})

	t.Run("停止(ctx の取り消し)を受けたら、そこで止め、失敗としては記録しない", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		stats := &uowtest.Stats{
			Due: dueOf(
				domain.RecalcRequest{BurgerID: uid.N(3), Version: 1},
				domain.RecalcRequest{BurgerID: uid.N(5), Version: 2},
			),
			// 1 つ目の再計算の途中で、停止が要求される。
			Facts: func(_ context.Context, burgerID string) ([]domain.ReviewFact, error) {
				cancel()
				return nil, cancelCtx.Err()
			},
		}
		logs := captureLogs(t)
		worker, _ := newStatsWorker(stats, uowtest.Clock{}, defaultWorkerConfig)
		n, err := worker.RunOnce(cancelCtx)
		if !errors.Is(err, context.Canceled) || n != 0 {
			t.Fatalf("RunOnce = (%d, %v), want (0, context.Canceled)", n, err)
		}
		if len(stats.Failures) != 0 {
			t.Errorf("停止による中断を失敗として記録した: %+v", stats.Failures)
		}
		if strings.Contains(logs.String(), "failed") {
			t.Errorf("停止による中断で error のログが出た: %s", logs)
		}
		for _, op := range stats.Ops {
			if op == "lock:"+uid.N(5) {
				t.Errorf("停止のあとで、次のバーガーの再計算を始めた: %v", stats.Ops)
			}
		}
	})
}
