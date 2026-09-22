package infra

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakeStatsRunner は、RunOnce の呼ばれ方を記録する statsRunner である。release が nil でなければ、
// RunOnce は、release が閉じられるか ctx が取り消されるまで、処理中のまま待つ。
type fakeStatsRunner struct {
	mu        sync.Mutex
	calls     int
	cancelled bool
	release   chan struct{}
	started   chan struct{}
	err       error
}

func newFakeStatsRunner() *fakeStatsRunner {
	return &fakeStatsRunner{started: make(chan struct{}, 100)}
}

func (r *fakeStatsRunner) RunOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	r.calls++
	release, err := r.release, r.err
	r.mu.Unlock()
	select {
	case r.started <- struct{}{}:
	default:
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			r.mu.Lock()
			r.cancelled = true
			r.mu.Unlock()
			return 0, ctx.Err()
		}
	}
	return 0, err
}

func (r *fakeStatsRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *fakeStatsRunner) wasCancelled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

// stopAsync は Stop を別の goroutine で呼び、その結果を返すチャネルを返す。
func stopAsync(l *StatsWorkerLoop, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- l.Stop(ctx) }()
	return done
}

func TestStatsWorkerLoop(t *testing.T) {
	t.Run("起動した直後に 1 サイクル実行する(間隔が長くても、溜まっていた依頼をすぐ処理する)", func(t *testing.T) {
		r := newFakeStatsRunner()
		l := StartStatsWorker(r, time.Hour)
		waitFor(t, "最初のサイクル", func() bool { return r.callCount() >= 1 })
		if err := l.Stop(context.Background()); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
		if got := r.callCount(); got != 1 {
			t.Errorf("サイクルの回数 = %d, want 1(間隔の 1 時間は、まだ経っていない)", got)
		}
	})

	t.Run("間隔ごとにサイクルを繰り返す", func(t *testing.T) {
		r := newFakeStatsRunner()
		l := StartStatsWorker(r, 5*time.Millisecond)
		waitFor(t, "3 回のサイクル", func() bool { return r.callCount() >= 3 })
		if err := l.Stop(context.Background()); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
	})

	t.Run("サイクルが失敗しても、次のサイクルに進む", func(t *testing.T) {
		r := newFakeStatsRunner()
		r.err = errors.New("db down")
		l := StartStatsWorker(r, 5*time.Millisecond)
		waitFor(t, "失敗したあとの 3 回のサイクル", func() bool { return r.callCount() >= 3 })
		if err := l.Stop(context.Background()); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
	})

	t.Run("Stop は、処理中のサイクルを取り消さず、終わるまで待ってから戻る", func(t *testing.T) {
		r := newFakeStatsRunner()
		r.release = make(chan struct{})
		l := StartStatsWorker(r, time.Hour)
		<-r.started

		stopped := stopAsync(l, context.Background())
		select {
		case err := <-stopped:
			t.Fatalf("処理中のサイクルが終わる前に Stop が戻った: %v", err)
		case <-time.After(50 * time.Millisecond):
		}

		close(r.release)
		select {
		case err := <-stopped:
			if err != nil {
				t.Errorf("Stop returned error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("サイクルが終わったのに Stop が戻らない")
		}
		if r.wasCancelled() {
			t.Error("Stop が、処理中のサイクルを取り消した(バッチを最後まで終えるはず)")
		}
	})

	t.Run("Stop のあとは、新しいサイクルを始めない", func(t *testing.T) {
		r := newFakeStatsRunner()
		l := StartStatsWorker(r, time.Millisecond)
		waitFor(t, "最初のサイクル", func() bool { return r.callCount() >= 1 })
		if err := l.Stop(context.Background()); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
		after := r.callCount()
		time.Sleep(30 * time.Millisecond)
		if got := r.callCount(); got != after {
			t.Errorf("Stop のあとにサイクルが %d 回増えた", got-after)
		}
	})

	t.Run("待ち時間の期限に達したら、処理中のサイクルを取り消し、期限のエラーを返す", func(t *testing.T) {
		r := newFakeStatsRunner()
		r.release = make(chan struct{})
		l := StartStatsWorker(r, time.Hour)
		<-r.started

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := l.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop = %v, want %v", err, context.DeadlineExceeded)
		}
		if !r.wasCancelled() {
			t.Error("期限に達したのに、処理中のサイクルが取り消されていない")
		}
	})

	t.Run("Stop は何度呼んでも安全で、goroutine は残らない", func(t *testing.T) {
		before := runtime.NumGoroutine()
		r := newFakeStatsRunner()
		l := StartStatsWorker(r, 5*time.Millisecond)
		waitFor(t, "最初のサイクル", func() bool { return r.callCount() >= 1 })
		for i := 0; i < 3; i++ {
			if err := l.Stop(context.Background()); err != nil {
				t.Fatalf("Stop #%d returned error: %v", i+1, err)
			}
		}
		waitFor(t, "goroutine が元の数に戻る", func() bool { return runtime.NumGoroutine() <= before })
	})
}

// orderedRunner は、呼ばれた順番を共有の記録に残し、決まった件数とエラーを返す statsRunner である。
type orderedRunner struct {
	name  string
	order *[]string
	n     int
	err   error
	after func()
}

func (r orderedRunner) RunOnce(context.Context) (int, error) {
	*r.order = append(*r.order, r.name)
	if r.after != nil {
		r.after()
	}
	return r.n, r.err
}

func TestStatsCycle(t *testing.T) {
	t.Run("ワーカーを渡した順に 1 回ずつ実行し、処理した件数の合計を返す", func(t *testing.T) {
		var order []string
		cycle := NewStatsCycle(orderedRunner{name: "burgers", order: &order, n: 3}, orderedRunner{name: "shops", order: &order, n: 2})
		n, err := cycle.RunOnce(context.Background())
		if err != nil || n != 5 {
			t.Fatalf("RunOnce = (%d, %v), want (5, nil)", n, err)
		}
		if len(order) != 2 || order[0] != "burgers" || order[1] != "shops" {
			t.Errorf("実行の順 = %v, want [burgers shops]", order)
		}
	})

	t.Run("1 つのワーカーが失敗しても、続くワーカーは実行し、失敗をまとめて返す", func(t *testing.T) {
		var order []string
		boom := errors.New("boom")
		cycle := NewStatsCycle(orderedRunner{name: "burgers", order: &order, err: boom}, orderedRunner{name: "shops", order: &order, n: 1})
		n, err := cycle.RunOnce(context.Background())
		if !errors.Is(err, boom) || n != 1 {
			t.Fatalf("RunOnce = (%d, %v), want (1, boom を含むエラー)", n, err)
		}
		if len(order) != 2 {
			t.Errorf("実行の順 = %v, want 2 つとも実行する", order)
		}
	})

	t.Run("ctx が取り消されたら、続くワーカーは実行しない", func(t *testing.T) {
		var order []string
		ctx, cancel := context.WithCancel(context.Background())
		cycle := NewStatsCycle(
			orderedRunner{name: "burgers", order: &order, err: context.Canceled, after: cancel},
			orderedRunner{name: "shops", order: &order},
		)
		if _, err := cycle.RunOnce(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if len(order) != 1 {
			t.Errorf("実行の順 = %v, want 取り消し後は実行しない", order)
		}
	})

	t.Run("周期の実行(StatsWorkerLoop)に渡すと、サイクルのたびに、すべてのワーカーが実行される", func(t *testing.T) {
		first, second := newFakeStatsRunner(), newFakeStatsRunner()
		loop := StartStatsWorker(NewStatsCycle(first, second), time.Millisecond)
		deadline := time.After(5 * time.Second)
		for first.callCount() < 2 || second.callCount() < 2 {
			select {
			case <-deadline:
				t.Fatalf("2 サイクル分が実行されない(first %d, second %d)", first.callCount(), second.callCount())
			case <-time.After(time.Millisecond):
			}
		}
		if err := loop.Stop(context.Background()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})
}
