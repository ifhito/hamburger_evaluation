package infra

import (
	"context"
	"log"
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
	// asyncMailerSendTimeout は、1 通の処理（記録・送信）の上限である。
	asyncMailerSendTimeout = 20 * time.Second
	// resultRecordTimeout は、送信の結果（sent / failed）を記録する DB 呼び出しの上限である。
	// 送信の timeout が尽きたあとでも結果を記録できるように、送信の context とは別に持つ。
	resultRecordTimeout = 5 * time.Second
)

// mailDeliverer は、メールを 1 通、同期的に送る。SMTPMailer がこれを満たす。
type mailDeliverer interface {
	Deliver(ctx context.Context, msg mailMessage) error
}

// mailJob は、キューに入れる 1 通分の仕事である。文面（msg）は、受け付けた時点で作ってある。
type mailJob struct {
	kind domain.MailKind
	key  string
	msg  mailMessage
}

// AsyncMailer は、usecase.Mailer を非同期に実装する。メソッドは、文面を組み立てて有界のキューに入れ、
// 即座に返る。少数の worker が、次の順で処理する。
//
//  1. 冪等キーで送信の記録を pending で作る（domain.MailDeliveries）。同じキーの要求はすでに扱って
//     いるので、送らない（同じ要求は、メールが 1 通だけ出る）
//  2. SMTP で送る
//  3. 結果を sent / failed（試行の回数・失敗の種類・切り詰めた理由）で記録する
//
// 送信の失敗・遅延・キューの満杯・記録の失敗は、呼び出し側には見えない（失敗はログに出す。応答の中身と
// 時間が、これらで変わらないので、登録の有無を推測されない）。再送はしない。記録できなかったときは、
// 冪等を守るために送らない（重複して送るより、送らないほうを選ぶ）。
type AsyncMailer struct {
	deliverer mailDeliverer
	records   *domain.MailDeliveries
	timeout   time.Duration
	queue     chan mailJob
	wg        sync.WaitGroup

	mu     sync.Mutex
	closed bool
}

// NewAsyncMailer は worker を起動して AsyncMailer を返す。終了時は Close を呼ぶこと。
func NewAsyncMailer(deliverer mailDeliverer, records *domain.MailDeliveries) *AsyncMailer {
	return newAsyncMailer(deliverer, records, asyncMailerWorkers, asyncMailerQueueSize, asyncMailerSendTimeout)
}

func newAsyncMailer(deliverer mailDeliverer, records *domain.MailDeliveries, workers, queueSize int, timeout time.Duration) *AsyncMailer {
	m := &AsyncMailer{
		deliverer: deliverer,
		records:   records,
		timeout:   timeout,
		queue:     make(chan mailJob, queueSize),
	}
	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.work()
	}
	return m
}

var _ usecase.Mailer = (*AsyncMailer)(nil)

// SendSignupConfirmation は、確認リンクつきのメールをキューに入れて即座に返る。
func (m *AsyncMailer) SendSignupConfirmation(n usecase.SignupConfirmation) {
	m.enqueue(mailJob{kind: domain.MailKindSignupConfirmation, key: n.IdempotencyKey, msg: renderSignupConfirmation(n)})
}

// SendAlreadyRegistered は、「すでに登録済み」の通知をキューに入れて即座に返る。
func (m *AsyncMailer) SendAlreadyRegistered(n usecase.AlreadyRegisteredNotice) {
	m.enqueue(mailJob{kind: domain.MailKindAlreadyRegistered, key: n.IdempotencyKey, msg: renderAlreadyRegistered(n)})
}

// enqueue は仕事をキューに入れて即座に返る。閉じたあと・キューが満杯のときは、捨てて、ログにだけ残す
// （宛先の値はログに出さない）。
func (m *AsyncMailer) enqueue(job mailJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		log.Printf("mail: dropped (mailer is closed)")
		return
	}
	select {
	case m.queue <- job:
	default:
		log.Printf("mail: dropped (queue is full)")
	}
}

func (m *AsyncMailer) work() {
	defer m.wg.Done()
	for job := range m.queue {
		m.process(job)
	}
}

// process は、1 通分を、記録 → 送信 → 結果の記録の順で処理する。
func (m *AsyncMailer) process(job mailJob) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	id, recorded, err := m.records.Record(ctx, domain.CreateMailDeliveryParams{
		Kind:           job.kind,
		Recipient:      job.msg.To,
		IdempotencyKey: job.key,
	})
	if err != nil {
		log.Printf("mail: record delivery (not sent): %v", err)
		return
	}
	if !recorded {
		return // 同じ冪等キーの要求は、すでに扱っている。
	}

	sendErr := m.deliverer.Deliver(ctx, job.msg)

	resultCtx, resultCancel := context.WithTimeout(context.Background(), resultRecordTimeout)
	defer resultCancel()
	if sendErr == nil {
		if err := m.records.MarkSent(resultCtx, id); err != nil {
			log.Printf("mail: record result: %v", err)
		}
		return
	}
	log.Printf("mail: delivery failed: %v", sendErr)
	if err := m.records.MarkFailed(resultCtx, id, classifyDeliveryError(sendErr), sendErr.Error()); err != nil {
		log.Printf("mail: record result: %v", err)
	}
}

// Close は、新しいメールの受け付けを止め、キューに残っているメールを処理し切るのを、ctx が
// 終わるまで待つ。呼び出し後の送信は捨てられる。ctx が先に終わったときは ctx.Err() を返す
// （worker は、実行中の処理が timeout で終わるまで残りうる）。
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
