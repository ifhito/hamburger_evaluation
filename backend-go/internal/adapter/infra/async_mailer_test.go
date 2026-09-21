package infra

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// gateDeliverer は、release されるまで送信を止める mailDeliverer の fake である。
type gateDeliverer struct {
	release chan struct{}
	err     error

	mu        sync.Mutex
	delivered []usecase.Mail
	deadlines []bool
}

func newGateDeliverer() *gateDeliverer { return &gateDeliverer{release: make(chan struct{})} }

func (g *gateDeliverer) Deliver(ctx context.Context, mail usecase.Mail) error {
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, hasDeadline := ctx.Deadline()
	g.mu.Lock()
	g.delivered = append(g.delivered, mail)
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

func mailTo(to string) usecase.Mail { return usecase.Mail{To: to, Subject: "s", Body: "b"} }

func TestAsyncMailerSendDoesNotWait(t *testing.T) {
	t.Run("送信が止まっていても Send は待たずに返る", func(t *testing.T) {
		d := newGateDeliverer()
		m := newAsyncMailer(d, 1, 10, time.Second)
		start := time.Now()
		m.Send(mailTo("a@example.com"))
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Errorf("Send が %v かかった", elapsed)
		}
		close(d.release)
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 1 {
			t.Errorf("delivered = %d, want 1", d.count())
		}
	})

	t.Run("キューが満杯でも Send は待たず、あふれた分は捨てる", func(t *testing.T) {
		d := newGateDeliverer()
		m := newAsyncMailer(d, 1, 2, time.Second)
		start := time.Now()
		for _, to := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
			m.Send(mailTo(to + "@example.com"))
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("Send が合計 %v かかった(満杯で待っている)", elapsed)
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

	t.Run("送信に失敗しても、あとのメールは送られる", func(t *testing.T) {
		d := newGateDeliverer()
		d.err = errors.New("smtp down")
		close(d.release)
		m := newAsyncMailer(d, 1, 10, time.Second)
		m.Send(mailTo("a@example.com"))
		m.Send(mailTo("b@example.com"))
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		if d.count() != 2 {
			t.Errorf("delivered = %d, want 2", d.count())
		}
	})

	t.Run("各送信には timeout つきの context が渡る", func(t *testing.T) {
		d := newGateDeliverer()
		close(d.release)
		m := newAsyncMailer(d, 1, 10, time.Second)
		m.Send(mailTo("a@example.com"))
		_ = m.Close(context.Background())
		if len(d.deadlines) != 1 || !d.deadlines[0] {
			t.Errorf("deadlines = %v, want [true]", d.deadlines)
		}
	})
}

func TestAsyncMailerRecipientWindow(t *testing.T) {
	d := newGateDeliverer()
	close(d.release)
	m := newAsyncMailer(d, 1, 10, time.Second)
	now := time.Now()
	m.now = func() time.Time { return now }

	m.Send(mailTo("Alice@Example.com"))
	m.Send(mailTo("alice@example.com")) // 大文字小文字だけが違う同じ宛先・同じ件名は、間隔内なので捨てる
	m.Send(usecase.Mail{To: "alice@example.com", Subject: "other", Body: "b"})
	now = now.Add(61 * time.Second)
	m.Send(mailTo("alice@example.com")) // 間隔を過ぎたので送る
	if err := m.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if d.count() != 3 {
		t.Errorf("delivered = %d, want 3（間隔内の同じ宛先・同じ件名の 1 通だけ捨てる）", d.count())
	}
}

func TestAsyncMailerClose(t *testing.T) {
	t.Run("Close はキューに残ったメールを送り切ってから返り、goroutine は残らない", func(t *testing.T) {
		before := runtime.NumGoroutine()
		d := newGateDeliverer()
		m := newAsyncMailer(d, 2, 10, time.Second)
		for _, to := range []string{"a", "b", "c", "d"} {
			m.Send(mailTo(to + "@example.com"))
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

	t.Run("Close のあとの Send は捨てられ、panic しない", func(t *testing.T) {
		d := newGateDeliverer()
		close(d.release)
		m := newAsyncMailer(d, 1, 10, time.Second)
		if err := m.Close(context.Background()); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		m.Send(mailTo("a@example.com"))
		if d.count() != 0 {
			t.Errorf("delivered = %d, want 0", d.count())
		}
		if err := m.Close(context.Background()); err != nil {
			t.Errorf("2 回目の Close returned error: %v", err)
		}
	})

	t.Run("送信が終わらなくても、Close は ctx の期限で返る", func(t *testing.T) {
		d := newGateDeliverer()
		m := newAsyncMailer(d, 1, 10, 5*time.Second)
		m.Send(mailTo("a@example.com"))
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := m.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close error = %v, want %v", err, context.DeadlineExceeded)
		}
		close(d.release)
		_ = m.Close(context.Background())
	})
}
