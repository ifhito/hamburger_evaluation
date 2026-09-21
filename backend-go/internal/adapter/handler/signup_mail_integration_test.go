package handler_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/smtptest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// mailKit は、本物の PostgreSQL・repository・非同期の送信・SMTP の実装（プロセス内のテスト用サーバー、または
// 接続できないポート）を通して、signup のメールの送信と記録を確かめるための一式である。
type mailKit struct {
	pool   *pgxpool.Pool
	router http.Handler
	// now は、signup の usecase の時計である。advance で進める（リクエストは同期的に処理されるので、競合しない）。
	now time.Time
}

// advance は、テストの時計を d だけ進める。
func (k *mailKit) advance(d time.Duration) { k.now = k.now.Add(d) }

// newMailKit は、smtpHost:smtpPort に平文（認証なし）で送る、signup の router を組み立てる。
// 時計は固定して、必要なときだけ advance で進める（通知の冪等キーの時間の窓が、テストの途中で
// 意図せずまたがれないようにするため）。
func newMailKit(t *testing.T, smtpHost string, smtpPort int) *mailKit {
	t.Helper()
	ctx := context.Background()
	_, url := dbtest.New(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	hasher := hasherFake{}
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	userQuery := query.NewUserQuery(pool)
	userWrites := domain.NewUsers(repository.NewUserRepository(pool))

	smtpMailer, err := infra.NewSMTPMailer(infra.Config{
		SMTPHost: smtpHost, SMTPPort: smtpPort, SMTPSecurity: "none", MailFrom: "Hamburger <noreply@example.com>",
	})
	if err != nil {
		t.Fatalf("NewSMTPMailer: %v", err)
	}
	mailer := infra.NewAsyncMailer(smtpMailer, domain.NewMailDeliveries(repository.NewMailDeliveryRepository(pool)))
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mailer.Close(closeCtx); err != nil {
			t.Errorf("mailer.Close: %v", err)
		}
	})

	unitOfWork := uow.New(pool)
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	kit := &mailKit{pool: pool, now: time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC)}
	signups := usecase.NewSignups(userQuery, domain.NewSignupVerifications(repository.NewSignupVerificationRepository(pool)),
		unitOfWork, hasher, mailer, codec, usecase.SignupConfig{BaseURL: "https://app.example.com", Now: func() time.Time { return kit.now }})
	kit.router = handler.NewRouter(pool, usecase.NewAuth(userQuery, hasher, codec, codec), signups,
		usecase.NewShops(query.NewShopQuery(pool), domain.NewShops(repository.NewShopRepository(pool))),
		usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(userQuery, userWrites, unitOfWork, recalc, hasher), nil, nil, nil)
	return kit
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

// closedPort は、待ち受けているものがないポート（接続すると拒否される）を返す。
func closedPort(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port := splitHostPort(t, ln.Addr().String())
	_ = ln.Close()
	return host, port
}

// deliveryRow は mail_deliveries の 1 行（テストで見る列）である。
type deliveryRow struct {
	kind, recipient, key, status string
	failureKind, lastError       *string
	attempts                     int
}

// waitDeliveries は、recipient 宛の記録が want 行になり、すべてが pending でなくなるまで待ち、その行を返す。
// 非同期に処理されるので、記録が落ち着くのを待つ。
func (k *mailKit) waitDeliveries(t *testing.T, recipient string, want int) []deliveryRow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows, err := k.pool.Query(context.Background(),
			"SELECT kind, recipient, idempotency_key, status, failure_kind, last_error, attempts FROM mail_deliveries WHERE recipient = $1 ORDER BY created_at", recipient)
		if err != nil {
			t.Fatalf("query mail_deliveries: %v", err)
		}
		var got []deliveryRow
		pending := false
		for rows.Next() {
			var r deliveryRow
			if err := rows.Scan(&r.kind, &r.recipient, &r.key, &r.status, &r.failureKind, &r.lastError, &r.attempts); err != nil {
				t.Fatalf("scan: %v", err)
			}
			pending = pending || r.status == "pending"
			got = append(got, r)
		}
		rows.Close()
		if len(got) == want && !pending {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("記録が落ち着かない: 行 = %d（want %d）, pending あり = %v", len(got), want, pending)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// allDeliveryText は、mail_deliveries の全行の全列を 1 つの文字列にしたものを返す（漏れの確認用）。
func (k *mailKit) allDeliveryText(t *testing.T) string {
	t.Helper()
	var text string
	if err := k.pool.QueryRow(context.Background(),
		"SELECT coalesce(string_agg(d::text, ' '), '') FROM mail_deliveries d").Scan(&text); err != nil {
		t.Fatalf("read mail_deliveries: %v", err)
	}
	return text
}

func headerWithoutDate(h http.Header) string {
	c := h.Clone()
	c.Del("Date")
	return fmt.Sprint(c)
}

// TestSignupMailDeliveryIntegration は、signup のメールの記録と冪等を、本物の DB・非同期の送信・SMTP の実装で
// 確かめる。SMTP が動いていても落ちていても、応答は同じで、記録だけが sent / failed に分かれる。
func TestSignupMailDeliveryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	const password = "Password123!"
	var okResponse *httptest.ResponseRecorder

	t.Run("送れたら sent で記録され、確認リンクのメールが 1 通届き、リンクで確認できる。記録に本文・トークン・パスワードは入らない", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{})
		host, port := splitHostPort(t, srv.Addr())
		kit := newMailKit(t, host, port)

		okResponse = do(kit.router, http.MethodPost, "/signup", signupBody("alice", "alice@example.com", password), "")
		if okResponse.Code != http.StatusAccepted || okResponse.Body.String() != signupAcceptedBody {
			t.Fatalf("signup = %d %s, want 202 %s", okResponse.Code, okResponse.Body, signupAcceptedBody)
		}
		rows := kit.waitDeliveries(t, "alice@example.com", 1)
		var verificationID string
		if err := kit.pool.QueryRow(context.Background(), "SELECT id FROM signup_verifications WHERE email = 'alice@example.com'").Scan(&verificationID); err != nil {
			t.Fatalf("read signup_verifications: %v", err)
		}
		r := rows[0]
		if r.kind != "signup_confirmation" || r.status != "sent" || r.attempts != 1 || r.failureKind != nil || r.lastError != nil {
			t.Errorf("記録 = %+v, want signup_confirmation・sent・試行 1・失敗の列は空", r)
		}
		if want := domain.SignupConfirmationMailKey(verificationID, 1); r.key != want {
			t.Errorf("冪等キー = %q, want %q（確認待ちの id + 世代）", r.key, want)
		}

		mails := srv.Received()
		if len(mails) != 1 {
			t.Fatalf("届いたメール = %d 通, want 1", len(mails))
		}
		mail := mails[0]
		if mail.To[0] != "alice@example.com" || !strings.Contains(mail.Data, "Subject: Confirm your email address") {
			t.Errorf("届いたメール = %+v", mail)
		}
		idx := strings.Index(mail.Data, confirmLinkPrefix)
		if idx < 0 {
			t.Fatalf("本文に確認リンクがない:\n%s", mail.Data)
		}
		token := strings.Fields(mail.Data[idx+len(confirmLinkPrefix):])[0]

		// 記録(DB)には、本文・確認トークン・パスワードが入っていない。
		text := kit.allDeliveryText(t)
		for _, secret := range []string{token, password, "Please confirm", "signup/confirm", "token=", "Subject", domain.HashSignupToken(token)} {
			if strings.Contains(text, secret) {
				t.Errorf("mail_deliveries の行に %q が含まれている: %s", secret, text)
			}
		}

		// 届いたリンクのトークンで、実際に確認できる。
		if rec := do(kit.router, http.MethodPost, "/signup/confirm", confirmBody(token), ""); rec.Code != http.StatusCreated {
			t.Errorf("confirm = %d %s, want 201", rec.Code, rec.Body)
		}
	})

	t.Run("登録済みの email への signup を繰り返しても、通知は 1 通・記録は 1 行で、応答は未登録と同じである", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{})
		host, port := splitHostPort(t, srv.Addr())
		kit := newMailKit(t, host, port)
		if _, err := kit.pool.Exec(context.Background(),
			"INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'digest:x')"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		for i := 0; i < 4; i++ {
			rec := do(kit.router, http.MethodPost, "/signup", signupBody("x", "bob@example.com", password), "")
			if rec.Code != okResponse.Code || rec.Body.String() != okResponse.Body.String() || headerWithoutDate(rec.Header()) != headerWithoutDate(okResponse.Header()) {
				t.Fatalf("繰り返し %d 回目の応答 = %d %s %v, want 未登録と同一の %d %s %v",
					i+1, rec.Code, rec.Body, rec.Header(), okResponse.Code, okResponse.Body, okResponse.Header())
			}
		}
		rows := kit.waitDeliveries(t, "bob@example.com", 1)
		if r := rows[0]; r.kind != "already_registered" || r.status != "sent" || r.attempts != 1 {
			t.Errorf("記録 = %+v, want already_registered・sent・試行 1", r)
		}
		time.Sleep(200 * time.Millisecond) // 後から 2 通目が出ないことを確かめる
		mails := srv.Received()
		if len(mails) != 1 || !strings.Contains(mails[0].Data, "Subject: You already have an account") || strings.Contains(strings.ToLower(mails[0].Data), "token") {
			t.Errorf("届いたメール = %d 通 %+v, want 通知 1 通（確認リンク・トークンを含めない）", len(mails), mails)
		}
		if n := countRows(t, kit.pool, "mail_deliveries"); n != 1 {
			t.Errorf("記録 = %d 行, want 1", n)
		}

		// 60 秒の窓をまたぐと、別の冪等キーになり、新しい通知が 1 通出る（同じ窓の中では出ない）。
		kit.advance(domain.MailResendInterval)
		for i := 0; i < 2; i++ {
			do(kit.router, http.MethodPost, "/signup", signupBody("x", "bob@example.com", password), "")
		}
		rows = kit.waitDeliveries(t, "bob@example.com", 2)
		if rows[0].key == rows[1].key || rows[1].status != "sent" {
			t.Errorf("窓をまたいだ記録 = %+v, want 別の冪等キーで 2 行とも sent", rows)
		}
		time.Sleep(200 * time.Millisecond)
		if n := len(srv.Received()); n != 2 {
			t.Errorf("届いたメール = %d 通, want 2（窓ごとに 1 通）", n)
		}
	})

	t.Run("SMTP に接続できなくても、応答は同じで、記録が failed・試行 1・一時的な失敗・切り詰めた理由になる", func(t *testing.T) {
		host, port := closedPort(t)
		kit := newMailKit(t, host, port)

		rec := do(kit.router, http.MethodPost, "/signup", signupBody("carol", "carol@example.com", password), "")
		if rec.Code != okResponse.Code || rec.Body.String() != okResponse.Body.String() || headerWithoutDate(rec.Header()) != headerWithoutDate(okResponse.Header()) {
			t.Fatalf("SMTP 停止中の応答 = %d %s %v, want 成功時と同一の %d %s %v",
				rec.Code, rec.Body, rec.Header(), okResponse.Code, okResponse.Body, okResponse.Header())
		}
		rows := kit.waitDeliveries(t, "carol@example.com", 1)
		r := rows[0]
		if r.status != "failed" || r.attempts != 1 || r.failureKind == nil || *r.failureKind != "temporary" {
			t.Fatalf("記録 = %+v, want failed・試行 1・temporary", r)
		}
		if r.lastError == nil || *r.lastError == "" || len([]rune(*r.lastError)) > domain.MaxMailErrorLength {
			t.Errorf("last_error = %v, want 1〜%d 文字", r.lastError, domain.MaxMailErrorLength)
		}
		text := kit.allDeliveryText(t)
		for _, secret := range []string{password, "Please confirm", "signup/confirm", "token="} {
			if strings.Contains(text, secret) {
				t.Errorf("failed の記録に %q が含まれている: %s", secret, text)
			}
		}
	})
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
