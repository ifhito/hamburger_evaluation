package infra

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// statsRunner は、統計の再計算の 1 サイクルを実行する。usecase.StatsWorker がこれを満たす。
type statsRunner interface {
	RunOnce(ctx context.Context) (int, error)
}

// StatsWorkerLoop は、統計の再計算のワーカーを、一定の間隔で回し続ける goroutine である(1 本だけ)。
// 起動した直後に 1 サイクル実行し(前回の停止までに溜まっていた依頼を処理する。再起動で取りこぼさない)、
// そのあとは interval ごとに 1 サイクルずつ実行する。1 サイクルは、上限件数までの依頼だけを処理するので、
// 溜まっていても、1 回の処理が長くなりすぎない(溜まった分は、続くサイクルで処理する)。
type StatsWorkerLoop struct {
	stop   chan struct{}
	done   chan struct{}
	cancel context.CancelFunc
	once   sync.Once
}

// StartStatsWorker は、runner を interval ごとに実行する goroutine を起動する。
// 止めるときは Stop を呼ぶこと。
func StartStatsWorker(runner statsRunner, interval time.Duration) *StatsWorkerLoop {
	// サイクルの中の DB の操作は、Stop の呼び出しでは取り消さない(処理中のバッチを終えるため)。
	// Stop が待ち時間の上限に達したときだけ、cancel で取り消す。
	runCtx, cancel := context.WithCancel(context.Background())
	l := &StatsWorkerLoop{stop: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	go l.run(runCtx, runner, interval)
	return l
}

func (l *StatsWorkerLoop) run(ctx context.Context, runner statsRunner, interval time.Duration) {
	defer close(l.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// 停止が要求されていれば、新しいサイクルは始めない(タイマーと同時に来ても、停止を優先する)。
		select {
		case <-l.stop:
			return
		default:
		}
		if _, err := runner.RunOnce(ctx); err != nil && ctx.Err() == nil {
			// 1 サイクルの失敗(依頼の一覧を読めない、など)は、次のサイクルで再試行する。
			slog.Error("stats worker cycle failed", "error", err)
		}
		select {
		case <-l.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Stop は、新しいサイクルを始めないようにし、処理中のサイクル(バッチ)を終えてから戻る。
// ctx の期限までに終わらなければ、処理中の DB の操作を取り消し、goroutine が終わるのを待って、
// ctx のエラーを返す(取り消された依頼は残り、次の起動で処理される)。複数回呼んでもよい。
func (l *StatsWorkerLoop) Stop(ctx context.Context) error {
	l.once.Do(func() { close(l.stop) })
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		l.cancel()
		<-l.done
		return ctx.Err()
	}
}
