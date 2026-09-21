package infra

import (
	"context"
	"errors"
	"fmt"
	"net/textproto"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// mailRow は fakeMailRepo が持つ送信の記録の 1 行である。
type mailRow struct {
	id        string
	kind      domain.MailKind
	recipient string
	key       string
	status    string
	failure   domain.MailFailure
	attempts  int
	lastError string
}

// fakeMailRepo は in-memory の domain.MailDeliveryRepository である。冪等キーの一意性（同じキーは
// 2 行目を作らない）を、DB と同じく守る。
type fakeMailRepo struct {
	mu        sync.Mutex
	rows      []*mailRow
	createErr error
}

var _ domain.MailDeliveryRepository = (*fakeMailRepo)(nil)

func (r *fakeMailRepo) CreateMailDelivery(_ context.Context, p domain.CreateMailDeliveryParams) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return "", false, r.createErr
	}
	for _, row := range r.rows {
		if row.key == p.IdempotencyKey {
			return "", false, nil
		}
	}
	row := &mailRow{id: fmt.Sprintf("id-%d", len(r.rows)+1), kind: p.Kind, recipient: p.Recipient, key: p.IdempotencyKey, status: "pending"}
	r.rows = append(r.rows, row)
	return row.id, true, nil
}

func (r *fakeMailRepo) find(id string) *mailRow {
	for _, row := range r.rows {
		if row.id == id {
			return row
		}
	}
	panic("unknown id " + id)
}

func (r *fakeMailRepo) UpdateMailDeliverySent(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.find(id)
	row.status, row.attempts = "sent", row.attempts+1
	return nil
}

func (r *fakeMailRepo) UpdateMailDeliveryFailed(_ context.Context, id string, failure domain.MailFailure, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.find(id)
	row.status, row.attempts, row.failure, row.lastError = "failed", row.attempts+1, failure, reason
	return nil
}

func (r *fakeMailRepo) snapshot() []mailRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mailRow, len(r.rows))
	for i, row := range r.rows {
		out[i] = *row
	}
	return out
}

// gateDeliverer は、release されるまで送信を止める mailDeliverer の fake である。
type gateDeliverer struct {
	release chan struct{}
	err     error
	repo    *fakeMailRepo // 送信の最中に、記録が pending であることを確かめる

	mu             sync.Mutex
	delivered      []mailMessage
	deadlines      []bool
	statusDuringTx []string
}

func newGateDeliverer(repo *fakeMailRepo) *gateDeliverer {
	return &gateDeliverer{release: make(chan struct{}), repo: repo}
}

func (g *gateDeliverer) Deliver(ctx context.Context, msg mailMessage) error {
	if g.repo != nil {
		rows := g.repo.snapshot()
		status := ""
		for _, row := range rows {
			if row.recipient == msg.To {
				status = row.status
			}
		}
		g.mu.Lock()
		g.statusDuringTx = append(g.statusDuringTx, status)
		g.mu.Unlock()
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, hasDeadline := ctx.Deadline()
	g.mu.Lock()
	g.delivered = append(g.delivered, msg)
	g.deadlines = append(g.deadlines, hasDeadline)
	g.mu.Unlock()
	return g.err
}

func (g *gateDeliverer) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.delivered)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func confirmation(to, key string) usecase.SignupConfirmation {
	return usecase.SignupConfirmation{To: to, ConfirmURL: "https://app.example.com/signup/confirm?token=abc", ValidFor: 24 * time.Hour, IdempotencyKey: key}
}

func notice(to, key string) usecase.AlreadyRegisteredNotice {
	return usecase.AlreadyRegisteredNotice{To: to, SignInURL: "https://app.example.com/signin", IdempotencyKey: key}
}

func newTestAsyncMailer(d mailDeliverer, repo *fakeMailRepo, workers, queueSize int) *AsyncMailer {
	return newAsyncMailer(d, domain.NewMailDeliveries(repo), workers, queueSize, time.Second)
}

func TestAsyncMailerSendDoesNotWait(t *testing.T) {
	t.Run("送信が止まっていても、メソッドは待たずに返る", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		m := newTestAsyncMailer(d, repo, 1, 10)
		start := time.Now()
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		m.SendAlreadyRegistered(notice("b@example.com", "k2"))
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Errorf("メソッドが %v かかった", elapsed)
		}
		close(d.release)
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 2 {
			t.Errorf("delivered = %d, want 2", d.count())
		}
	})

	t.Run("キューが満杯でも待たず、あふれた分は捨てる", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		m := newTestAsyncMailer(d, repo, 1, 2)
		start := time.Now()
		for i := 0; i < 8; i++ {
			m.SendSignupConfirmation(confirmation(fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("k%d", i)))
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("合計 %v かかった(満杯で待っている)", elapsed)
		}
		close(d.release)
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		// 送信中の 1 通と、キューの 2 通だけが届く(worker の取り出しの前後で 2〜3 通)。
		if n := d.count(); n < 2 || n > 3 {
			t.Errorf("delivered = %d, want 2〜3（キューの大きさ 2 + 送信中の 1）", n)
		}
	})

	t.Run("各処理には timeout つきの context が渡る", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		_ = m.Close(context.Background())
		if len(d.deadlines) != 1 || !d.deadlines[0] {
			t.Errorf("deadlines = %v, want [true]", d.deadlines)
		}
	})
}

// TestAsyncMailerRecordsDeliveries は、送信の前に pending で記録し、結果を sent / failed で記録することを
// 確かめる。
func TestAsyncMailerRecordsDeliveries(t *testing.T) {
	t.Run("送信の前に pending で記録され、送れたら sent・試行 1 回になる。種類・宛先・冪等キーが記録される", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		m.SendSignupConfirmation(confirmation("a@example.com", "signup_confirmation:v1:1"))
		m.SendAlreadyRegistered(notice("b@example.com", "already_registered:b@example.com:7"))
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		for i, status := range d.statusDuringTx {
			if status != "pending" {
				t.Errorf("送信 %d の最中の記録 = %q, want pending（送る前に記録する）", i, status)
			}
		}
		rows := repo.snapshot()
		if len(rows) != 2 {
			t.Fatalf("記録 = %d 行, want 2", len(rows))
		}
		want := []mailRow{
			{kind: domain.MailKindSignupConfirmation, recipient: "a@example.com", key: "signup_confirmation:v1:1", status: "sent", attempts: 1},
			{kind: domain.MailKindAlreadyRegistered, recipient: "b@example.com", key: "already_registered:b@example.com:7", status: "sent", attempts: 1},
		}
		for i, w := range want {
			got := rows[i]
			if got.kind != w.kind || got.recipient != w.recipient || got.key != w.key || got.status != w.status || got.attempts != w.attempts {
				t.Errorf("記録 %d = %+v, want %+v", i, got, w)
			}
		}
		// 出した文面は、種類に合った件名になる。
		if d.delivered[0].Subject != "Confirm your email address for BurgerStack" || d.delivered[1].Subject != "You already have a BurgerStack account" {
			t.Errorf("件名 = %q / %q", d.delivered[0].Subject, d.delivered[1].Subject)
		}
	})

	t.Run("同じ冪等キーの要求は、何度来てもメールが 1 通・記録が 1 行だけになる", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		close(d.release)
		m := newTestAsyncMailer(d, repo, 2, 20)
		for i := 0; i < 5; i++ {
			m.SendSignupConfirmation(confirmation("a@example.com", "same-key"))
		}
		m.SendSignupConfirmation(confirmation("a@example.com", "other-key"))
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 2 {
			t.Errorf("delivered = %d, want 2（same-key が 1 通、other-key が 1 通）", d.count())
		}
		if rows := repo.snapshot(); len(rows) != 2 {
			t.Errorf("記録 = %d 行, want 2", len(rows))
		}
	})

	t.Run("失敗したら failed・試行 1 回・失敗の種類と切り詰めた理由が記録され、次のメールは送られる", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		d.err = fmt.Errorf("smtp: rcpt to: %w", &textproto.Error{Code: 550, Msg: strings.Repeat("no such user ", 40)})
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		m.SendSignupConfirmation(confirmation("b@example.com", "k2"))
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 2 {
			t.Fatalf("delivered = %d, want 2（失敗しても、あとのメールは処理される）", d.count())
		}
		for _, row := range repo.snapshot() {
			if row.status != "failed" || row.attempts != 1 || row.failure != domain.MailFailurePermanent {
				t.Errorf("記録 = %+v, want failed・試行 1・permanent（550 は恒久的な失敗）", row)
			}
			if n := len([]rune(row.lastError)); n == 0 || n > domain.MaxMailErrorLength {
				t.Errorf("last_error の長さ = %d, want 1〜%d", n, domain.MaxMailErrorLength)
			}
		}
	})

	t.Run("接続できない失敗は、一時的な失敗として記録される", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		d.err = errors.New("smtp: connect: dial tcp: connection refused")
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		m.SendAlreadyRegistered(notice("a@example.com", "k1"))
		_ = m.Close(context.Background())
		rows := repo.snapshot()
		if len(rows) != 1 || rows[0].status != "failed" || rows[0].failure != domain.MailFailureTemporary {
			t.Errorf("記録 = %+v, want failed・temporary", rows)
		}
	})

	t.Run("記録に失敗したら、冪等を守るため送らない", func(t *testing.T) {
		repo := &fakeMailRepo{createErr: errors.New("db down")}
		d := newGateDeliverer(repo)
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		_ = m.Close(context.Background())
		if d.count() != 0 {
			t.Errorf("delivered = %d, want 0（記録できないときは送らない）", d.count())
		}
	})
}

func TestAsyncMailerClose(t *testing.T) {
	t.Run("Close はキューに残ったメールを処理し切ってから返り、goroutine は残らない", func(t *testing.T) {
		before := runtime.NumGoroutine()
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		m := newTestAsyncMailer(d, repo, 2, 10)
		for i := 0; i < 4; i++ {
			m.SendSignupConfirmation(confirmation(fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("k%d", i)))
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(d.release)
		}()
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 4 {
			t.Errorf("delivered = %d, want 4", d.count())
		}
		waitFor(t, "goroutine が元の数に戻る", func() bool { return runtime.NumGoroutine() <= before })
	})

	t.Run("Close のあとの送信は捨てられ、panic しない", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		close(d.release)
		m := newTestAsyncMailer(d, repo, 1, 10)
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		m.SendAlreadyRegistered(notice("a@example.com", "k2"))
		if d.count() != 0 {
			t.Errorf("delivered = %d, want 0", d.count())
		}
		if err := m.Close(context.Background()); err != nil {
			t.Errorf("2 回目の Close returned error: %v", err)
		}
	})

	t.Run("送信が終わらなくても、Close は ctx の期限で返る", func(t *testing.T) {
		repo := &fakeMailRepo{}
		d := newGateDeliverer(repo)
		m := newAsyncMailer(d, domain.NewMailDeliveries(repo), 1, 10, 5*time.Second)
		m.SendSignupConfirmation(confirmation("a@example.com", "k1"))
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := m.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close error = %v, want %v", err, context.DeadlineExceeded)
		}
		close(d.release)
		_ = m.Close(context.Background())
	})
}
