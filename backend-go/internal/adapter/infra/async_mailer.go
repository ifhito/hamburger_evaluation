package infra

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	// asyncMailerWorkers は、送信を並行して行う worker の数である。
	asyncMailerWorkers = 2
	// asyncMailerQueueSize は、送信待ちのメールを溜める有界のキューの大きさである。
	asyncMailerQueueSize = 100
	// asyncMailerSendTimeout は、1 通の送信の上限である。
	asyncMailerSendTimeout = 20 * time.Second
	// recentMailWindow は、同じ宛先・同じ件名のメールを続けて送らない間隔である
	// （確認メールの間隔と同じ）。
	recentMailWindow = domain.SignupResendInterval
	// recentMailLimit は、間隔の記録の最大件数である（メモリの上限）。
	recentMailLimit = 4096
)

// mailDeliverer は、メールを 1 通、同期的に送る。SMTPMailer がこれを満たす。
type mailDeliverer interface {
	Deliver(ctx context.Context, mail usecase.Mail) error
}

// AsyncMailer は、usecase.Mailer を非同期に実装する。Send は、有界のキューに入れて即座に返り、
// 少数の worker が SMTP へ送る。送信の失敗・遅延・キューの満杯は、Send の呼び出し側には
// 見えない（失敗はログに出す。応答時間や結果から、登録の有無を推測されないため）。
// 同じ宛先・同じ件名のメールは、60 秒に 1 通に絞る（DB の判定を通らない通知メールを
// 使った、第三者へのメールの大量送信を抑える。プロセスごとの記録で、複数のインスタンスで
// 共有はしない）。
type AsyncMailer struct {
	deliverer mailDeliverer
	timeout   time.Duration
	queue     chan usecase.Mail
	wg        sync.WaitGroup

	mu     sync.Mutex
	closed bool
	recent map[string]time.Time
	now    func() time.Time
}

// NewAsyncMailer は worker を起動して AsyncMailer を返す。終了時は Close を呼ぶこと。
func NewAsyncMailer(deliverer mailDeliverer) *AsyncMailer {
	return newAsyncMailer(deliverer, asyncMailerWorkers, asyncMailerQueueSize, asyncMailerSendTimeout)
}

func newAsyncMailer(deliverer mailDeliverer, workers, queueSize int, timeout time.Duration) *AsyncMailer {
	m := &AsyncMailer{
		deliverer: deliverer,
		timeout:   timeout,
		queue:     make(chan usecase.Mail, queueSize),
		recent:    make(map[string]time.Time),
		now:       time.Now,
	}
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.work()
	}
	return m
}

var _ usecase.Mailer = (*AsyncMailer)(nil)

// Send はメールをキューに入れて即座に返る。閉じた後・キューが満杯・直前に同じ宛先へ同じ件名を
// 送った場合は、送らずに捨て、ログにだけ残す（宛先の値はログに出さない）。
func (m *AsyncMailer) Send(mail usecase.Mail) {
	key := strings.ToLower(mail.To) + "\x00" + mail.Subject

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		log.Printf("mail: dropped (mailer is closed)")
		return
	}
	now := m.now()
	if last, ok := m.recent[key]; ok && now.Sub(last) < recentMailWindow {
		log.Printf("mail: dropped (sent to the same recipient within %s)", recentMailWindow)
		return
	}
	select {
	case m.queue <- mail:
		m.remember(key, now)
	default:
		log.Printf("mail: dropped (queue is full)")
	}
}

// remember は、送った時刻を記録する。上限に達したら、まず期間外の記録を掃除し、それでも
// 上限なら記録しない（メモリを増やさない。間隔の絞りが効かなくなるだけで、送信はできる）。
func (m *AsyncMailer) remember(key string, now time.Time) {
	if len(m.recent) >= recentMailLimit {
		for k, t := range m.recent {
			if now.Sub(t) >= recentMailWindow {
				delete(m.recent, k)
			}
		}
		if len(m.recent) >= recentMailLimit {
			return
		}
	}
	m.recent[key] = now
}

func (m *AsyncMailer) work() {
	defer m.wg.Done()
	for mail := range m.queue {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		if err := m.deliverer.Deliver(ctx, mail); err != nil {
			log.Printf("mail: delivery failed: %v", err)
		}
		cancel()
	}
}

// Close は、新しいメールの受け付けを止め、キューに残っているメールを送り切るのを、ctx が
// 終わるまで待つ。呼び出し後の Send は捨てられる。ctx が先に終わったときは ctx.Err() を返す
// （worker は、実行中の送信が timeout で終わるまで残りうる）。
func (m *AsyncMailer) Close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.queue)
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
