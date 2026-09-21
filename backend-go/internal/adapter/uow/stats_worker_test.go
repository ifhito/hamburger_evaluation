package uow_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/statsworkertest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// このファイルは、統計の再計算のワーカーを、実際の PostgreSQL に対して、本物の usecase・query・repository と
// つないで確かめる。書き込みは「再計算の依頼」を登録するだけで、統計は、ワーカーが RunOnce で動くまで
// 古いままであること、同じバーガーの依頼は 1 件にまとまること、再計算の最中に新しい書き込みが入っても
// 更新を取りこぼさないこと、失敗した依頼が、待ち時間を置いて再試行され、上限で打ち切られることを確かめる。

// steppingClock は、テストが進める時刻を返す usecase.Clock である。失敗した依頼の「次の再試行の時刻」を、
// 実際に待たずに過ぎさせるために使う。
type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func newSteppingClock() *steppingClock {
	return &steppingClock{now: time.Now().Truncate(time.Microsecond)}
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *steppingClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// hookedUnit は、UnitOfWork のトランザクションの中で、統計の元データを読んだ直後に hook を呼ぶ
// UnitOfWork である。元データを読んだあと、統計を保存する前という、再計算の途中の場面を、テストが
// 作り出すために使う。hook が error を返すと、その元データの読み取りは、その error で失敗する。
type hookedUnit struct {
	usecase.UnitOfWork
	hook func(ctx context.Context, burgerID string) error
}

func (h hookedUnit) Do(ctx context.Context, fn func(ctx context.Context, tx usecase.Tx) error) error {
	return h.UnitOfWork.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		tx.Stats = hookedStats{BurgerStatsQuery: tx.Stats, hook: h.hook}
		return fn(ctx, tx)
	})
}

type hookedStats struct {
	usecase.BurgerStatsQuery
	hook func(ctx context.Context, burgerID string) error
}

func (s hookedStats) ListBurgerReviewFacts(ctx context.Context, burgerID string) ([]domain.ReviewFact, error) {
	facts, err := s.BurgerStatsQuery.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return nil, err
	}
	if err := s.hook(ctx, burgerID); err != nil {
		return nil, err
	}
	return facts, nil
}

// hookedWorker は、world の接続で動き、hook を挟んだ統計のワーカーを返す。
func (w *world) hookedWorker(clock usecase.Clock, maxAttempts int, hook func(ctx context.Context, burgerID string) error) *usecase.StatsWorker {
	return usecase.NewStatsWorker(query.NewBurgerStatsQuery(w.conn), hookedUnit{UnitOfWork: w.unit, hook: hook}, w.recalc, clock,
		usecase.StatsWorkerConfig{Batch: 100, MaxAttempts: maxAttempts})
}

// pool は、別の接続(トランザクション)から書き込みや別のワーカーを動かすための接続プールを開く。
func (w *world) pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(w.ctx, w.dbURL)
	if err != nil {
		t.Fatalf("接続プールを開けなかった: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestStatsWorkerDeferredRecalculation は、書き込みの時点では統計が変わらず、ワーカーが動いて初めて
// 反映されること、同じバーガーへの書き込みが続いても、再計算が 1 回にまとまることを確かめる。
func TestStatsWorkerDeferredRecalculation(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn

	t.Run("レビューを投稿した時点では、依頼が 1 件登録されるだけで統計は変わらず、ワーカーが動くと統計が更新されて依頼が消える", func(t *testing.T) {
		alice, bob := w.user(t, "alice"), w.user(t, "bob")
		burger := w.burger(t, "Deferred Burger")

		w.review(t, alice, burger, 5, "great")
		if _, ok := dbtest.FetchBurgerStats(ctx, t, conn, burger); ok {
			t.Error("投稿しただけなのに、統計の行ができている(統計の計算は、ワーカーが行うはず)")
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 1 {
			t.Fatalf("投稿直後の再計算の依頼 = %d 件, want 1 件", n)
		}
		if n, err := w.worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 1 || got.AverageRating != 5.0 {
			t.Errorf("1 件目のあとの統計 = %+v, want 件数 1・平均 5.0", got)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 0 {
			t.Errorf("再計算のあとの依頼 = %d 件, want 0 件", n)
		}

		// 2 件目の投稿では、ワーカーが動くまで、統計は 1 件目のままである。
		w.review(t, bob, burger, 3, "ok")
		if got, _ := dbtest.FetchBurgerStats(ctx, t, conn, burger); got.ReviewCount != 1 || got.AverageRating != 5.0 {
			t.Errorf("2 件目の投稿直後の統計 = %+v, want まだ 1 件目のときの値", got)
		}
		w.settle(t)
		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 2 || got.AverageRating != 4.0 {
			t.Errorf("2 件目のあとの統計 = %+v, want 件数 2・平均 4.0", got)
		}
	})

	t.Run("同じバーガーへの書き込みが続いても、依頼は 1 件にまとまり、再計算は 1 回で済む", func(t *testing.T) {
		burger := w.burger(t, "Burst Burger")
		var lastVersion int64
		for i := 0; i < 5; i++ {
			w.review(t, w.user(t, fmt.Sprintf("burst%d", i)), burger, 1+i%5, "burst")
			req, ok := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
			if !ok || req.Version <= lastVersion {
				t.Fatalf("%d 件目の投稿後の依頼 = (%+v, %v), want version が進んだ依頼", i+1, req, ok)
			}
			lastVersion = req.Version
		}
		var requests int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM burger_stats_recalc_requests WHERE burger_id = $1`, burger).Scan(&requests); err != nil || requests != 1 {
			t.Fatalf("依頼の行数 = %d (エラー %v), want 1", requests, err)
		}

		if n, err := w.worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)(5 回の書き込みが 1 回の再計算になる)", n, err)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 5 {
			t.Errorf("統計 = %+v, want 5 件すべてを反映した件数 5", got)
		}
	})

	t.Run("ワーカーが止まっている間に溜まった依頼は、新しいワーカーが起動して処理する(再起動で取りこぼさない)", func(t *testing.T) {
		alice := w.user(t, "restart-alice")
		var burgers []string
		for i := 0; i < 3; i++ {
			burger := w.burger(t, fmt.Sprintf("Backlog Burger %d", i))
			w.review(t, alice, burger, 4, "backlog")
			burgers = append(burgers, burger)
		}

		fresh := statsworkertest.NewWorker(conn, infra.SystemClock{})
		if n, err := fresh.RunOnce(ctx); err != nil || n != 3 {
			t.Fatalf("RunOnce = (%d, %v), want (3, nil)", n, err)
		}
		for _, burger := range burgers {
			if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 1 {
				t.Errorf("バーガー %s の統計 = %+v, want 件数 1", burger, got)
			}
		}
	})

	t.Run("依頼の登録は、同じトランザクションが巻き戻れば、一緒に巻き戻る", func(t *testing.T) {
		burger := w.burger(t, "Untouched Burger")
		boom := errors.New("boom")
		err := w.unit.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
			if err := w.recalc.RequestRecalculation(ctx, tx, burger); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Do のエラー = %v, want %v", err, boom)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 0 {
			t.Errorf("巻き戻したあとの依頼 = %d 件, want 0 件", n)
		}
	})
}

// TestStatsWorkerConcurrentWrite は、再計算の最中に同じバーガーへ新しい書き込みが入る場面を、実際の
// 別の接続で作って、更新を取りこぼさないことを確かめる。
func TestStatsWorkerConcurrentWrite(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	pool := w.pool(t)
	otherReviews := usecase.NewReviews(query.NewReviewQuery(pool), uow.New(pool), w.recalc, storage.NewDisk(t.TempDir(), "/photos"))

	t.Run("再計算の最中に新しい投稿が確定すると、依頼は消えずに残り、次の再計算で最新になる。投稿は再計算に待たされない", func(t *testing.T) {
		alice, bob := w.user(t, "alice"), w.user(t, "bob")
		burger := w.burger(t, "Concurrent Burger")
		w.review(t, alice, burger, 5, "first")
		taken, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)

		var injected bool
		worker := w.hookedWorker(infra.SystemClock{}, 5, func(ctx context.Context, id string) error {
			if injected {
				return nil
			}
			injected = true
			// ワーカーは、バーガーの行をロックして、元データ(alice の 1 件)を読み終えた。この時点で、別の
			// 接続から、bob の投稿を確定させる。投稿はバーガーの行を更新しないので、再計算のロックに
			// 待たされない(待たされるなら、この呼び出しは期限切れで失敗する)。
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_, err := otherReviews.Create(writeCtx, bob, w.shop, burger, "", 3, "during recalculation", nil)
			return err
		})
		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, nil)", n, err)
		}

		// 統計は、再計算の途中で読んだ元データ(alice の 1 件)のものである。
		if got, _ := dbtest.FetchBurgerStats(ctx, t, conn, burger); got.ReviewCount != 1 {
			t.Errorf("再計算の直後の統計の件数 = %d, want 元データを読んだ時点の 1", got.ReviewCount)
		}
		// bob の投稿の依頼は、消されずに残っている。version は、取り出したときより進んでいる。
		left, ok := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
		if !ok {
			t.Fatal("再計算の最中に登録された依頼が、消えてしまった(bob の投稿が、統計に反映されなくなる)")
		}
		if left.Version <= taken.Version {
			t.Errorf("残った依頼の version = %d, want 取り出したときの %d より大きい", left.Version, taken.Version)
		}

		w.settle(t)
		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 2 || got.AverageRating != 4.0 {
			t.Errorf("次の再計算のあとの統計 = %+v, want 2 件の投稿を反映した件数 2・平均 4.0", got)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 0 {
			t.Errorf("次の再計算のあとの依頼 = %d 件, want 0 件", n)
		}
	})

	t.Run("2 つのワーカーが同じバーガーを再計算しても、後から入った投稿を反映した統計が最後に残る", func(t *testing.T) {
		alice, bob := w.user(t, "carol"), w.user(t, "dave")
		burger := w.burger(t, "Two Workers Burger")
		w.review(t, alice, burger, 5, "first")

		secondDone := make(chan struct{})
		var injected, secondStarted bool
		first := w.hookedWorker(infra.SystemClock{}, 5, func(ctx context.Context, id string) error {
			if injected {
				return nil
			}
			injected = true
			// 1 つ目のワーカーが元データを読み終えたあと、bob の投稿が確定し、そのあとで別のワーカー(別の
			// インスタンスを想定)が同じバーガーの再計算を始める。行のロックがあれば、2 つ目のワーカーは、
			// 1 つ目が終わるまで待たされ、そのあとで最新の元データを読む。ロックがなければ、2 つ目が
			// 先に最新の統計を保存して依頼を消し、そのあとに 1 つ目が古い元データの統計で上書きしてしまう。
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if _, err := otherReviews.Create(writeCtx, bob, w.shop, burger, "", 3, "second", nil); err != nil {
				return err
			}
			secondStarted = true
			go func() {
				defer close(secondDone)
				if _, err := statsworkertest.NewWorker(pool, infra.SystemClock{}).RunOnce(context.Background()); err != nil {
					t.Errorf("2 つ目のワーカーの RunOnce: %v", err)
				}
			}()
			select {
			case <-secondDone:
				t.Error("1 つ目の再計算の途中で、2 つ目のワーカーが同じバーガーの再計算を終えた(行のロックで待たされるはず)")
			case <-time.After(500 * time.Millisecond):
			}
			return nil
		})
		if _, err := first.RunOnce(ctx); err != nil {
			t.Fatalf("1 つ目のワーカーの RunOnce: %v", err)
		}
		if !secondStarted {
			t.Fatal("bob の投稿が確定できず、2 つ目のワーカーを動かせなかった")
		}
		<-secondDone
		w.settle(t)

		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 2 {
			t.Errorf("統計 = %+v, want 2 件の投稿を反映した件数 2(古い元データで上書きされていない)", got)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 0 {
			t.Errorf("依頼 = %d 件, want 0 件", n)
		}
	})
}

// TestStatsWorkerFailure は、再計算に失敗した依頼の扱いを、実際のデータベースの行で確かめる。失敗は、
// 失敗の回数・次の再試行の時刻・理由として行に残り、待ち時間が過ぎるまで再試行されず、上限の回数で
// 打ち切られる。1 つのバーガーの失敗は、ほかのバーガーの再計算を止めない。
func TestStatsWorkerFailure(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")

	// 失敗させるバーガーの再計算は、元データを読んだあとで、決まった回数だけ失敗する。
	newFailingWorker := func(clock usecase.Clock, maxAttempts int, failing string, failures *int) *usecase.StatsWorker {
		return w.hookedWorker(clock, maxAttempts, func(_ context.Context, id string) error {
			if id == failing && *failures > 0 {
				*failures--
				return errors.New("reading review facts failed")
			}
			return nil
		})
	}

	t.Run("失敗した依頼には失敗の回数・次の再試行の時刻・理由が残り、待ち時間が過ぎるまで再試行されず、ほかのバーガーは処理される", func(t *testing.T) {
		clock := newSteppingClock()
		bad, good := w.burger(t, "Failing Burger"), w.burger(t, "Fine Burger")
		w.review(t, alice, bad, 5, "x")
		w.review(t, alice, good, 4, "y")
		failures := 1
		worker := newFailingWorker(clock, 5, bad, &failures)

		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want 失敗した 1 件を除く (1, nil)", n, err)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, good); got.ReviewCount != 1 {
			t.Errorf("失敗していないバーガーの統計 = %+v, want 件数 1", got)
		}
		if _, ok := dbtest.FetchBurgerStats(ctx, t, conn, bad); ok {
			t.Error("失敗したバーガーの統計の行ができている")
		}
		req, ok := dbtest.FetchRecalcRequest(ctx, t, conn, bad)
		if !ok {
			t.Fatal("失敗した依頼が消えた")
		}
		if req.Attempts != 1 || req.NextAttemptAt == nil || !req.NextAttemptAt.Equal(clock.Now().Add(2*time.Second)) {
			t.Errorf("失敗した依頼 = %+v, want 失敗 1 回・次の再試行は 2 秒後", req)
		}
		if req.LastError == nil || !strings.Contains(*req.LastError, "reading review facts failed") {
			t.Errorf("失敗の理由 = %v, want 元のエラーの文言を含む", req.LastError)
		}

		// 待ち時間の間は、再試行されない。
		clock.Advance(time.Second)
		if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
			t.Fatalf("待ち時間の間の RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if again, _ := dbtest.FetchRecalcRequest(ctx, t, conn, bad); again.Attempts != 1 {
			t.Errorf("待ち時間の間に再試行された(失敗の回数 = %d, want 1 のまま)", again.Attempts)
		}

		// 待ち時間が過ぎたら再試行され、今度は成功して、依頼が消える。
		clock.Advance(2 * time.Second)
		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("待ち時間が過ぎたあとの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, bad); got.ReviewCount != 1 {
			t.Errorf("再試行のあとの統計 = %+v, want 件数 1", got)
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, bad); n != 0 {
			t.Errorf("再試行に成功したあとの依頼 = %d 件, want 0 件", n)
		}
	})

	t.Run("失敗が続くと待ち時間が倍になり、上限の回数で打ち切られ、行は残る。新しい書き込みで、最初からやり直せる", func(t *testing.T) {
		logs := captureSlog(t)
		clock := newSteppingClock()
		const maxAttempts = 3
		burger := w.burger(t, "Hopeless Burger")
		w.review(t, alice, burger, 2, "z")
		failures := 1000
		worker := newFailingWorker(clock, maxAttempts, burger, &failures)

		// 待ち時間は 2 秒 → 4 秒 → 8 秒と倍になる。3 回目の失敗で、上限の回数に達して打ち切られる。
		for attempt, wantDelay := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
			if _, err := worker.RunOnce(ctx); err != nil {
				t.Fatalf("%d 回目の RunOnce: %v", attempt+1, err)
			}
			req, _ := dbtest.FetchRecalcRequest(ctx, t, conn, burger)
			if req.Attempts != attempt+1 || req.NextAttemptAt == nil || !req.NextAttemptAt.Equal(clock.Now().Add(wantDelay)) {
				t.Fatalf("%d 回失敗したあとの依頼 = %+v, want 失敗 %d 回・次の再試行は %v 後", attempt+1, req, attempt+1, wantDelay)
			}
			clock.Advance(10 * time.Minute)
		}
		if got := logs.String(); !strings.Contains(got, "gave up") || !strings.Contains(got, burger) {
			t.Errorf("打ち切りのログがない: %s", got)
		}

		// 打ち切られた依頼は、時間が過ぎても、もう取り出されない。行は残る。
		before := failures
		clock.Advance(24 * time.Hour)
		if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
			t.Fatalf("打ち切り後の RunOnce = (%d, %v), want (0, nil)", n, err)
		}
		if failures != before {
			t.Error("打ち切ったあとも、再計算を試みている")
		}
		if n := dbtest.CountRecalcRequests(ctx, t, conn, burger); n != 1 {
			t.Errorf("打ち切った依頼の行 = %d 件, want 1 件(調査のために残す)", n)
		}

		// 新しい書き込みがあれば、依頼は最初からやり直しになり、再計算される。
		failures = 0
		w.review(t, w.user(t, "erin"), burger, 4, "again")
		if n, err := worker.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("新しい書き込みのあとの RunOnce = (%d, %v), want (1, nil)", n, err)
		}
		if got := dbtest.RequireConsistentStats(ctx, t, conn, burger); got.ReviewCount != 2 {
			t.Errorf("統計 = %+v, want 2 件の投稿を反映した件数 2", got)
		}
	})
}
