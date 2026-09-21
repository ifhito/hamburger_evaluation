package uow_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// このファイルは、メール確認での登録(確認メールのリンクを開いたときに、確認待ちをロックし、その内容で
// ユーザーを作り、確認待ちを削除する手順)を、実際の PostgreSQL に対して、本物の usecase・query・
// repository・UnitOfWork(ここからここまでを 1 つのトランザクションにする範囲を指定する仕組み)と
// つないで確かめる(環境変数 TEST_DATABASE_URL がなければスキップする)。
// この手順は、途中で失敗したら全体を取り消し、ユーザーだけが作られる、確認待ちだけが消える、が
// 起きないことが大事なので、トランザクションの効き目を、実データベースで確認する。

// noMail は、メールを送らない usecase.Mailer である(確認の手順はメールを送らない)。
type noMail struct{}

func (noMail) SendSignupConfirmation(usecase.SignupConfirmation)     {}
func (noMail) SendAlreadyRegistered(usecase.AlreadyRegisteredNotice) {}

// signupWorld は、確認の手順を試すための、本物の部品の組み合わせである。
type signupWorld struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	signups       *usecase.Signups
	verifications *domain.SignupVerifications
}

func newSignupWorld(t *testing.T) *signupWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed signup confirmation test in short mode")
	}
	ctx := context.Background()
	_, url := dbtest.New(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	verifications := domain.NewSignupVerifications(repository.NewSignupVerificationRepository(pool))
	signups := usecase.NewSignups(query.NewUserQuery(pool), verifications, uow.New(pool),
		infra.BcryptPasswordHasher{}, noMail{}, infra.NewJWTCodec("test-secret", time.Hour),
		usecase.SignupConfig{BaseURL: "https://app.example.com"})
	return &signupWorld{ctx: ctx, pool: pool, signups: signups, verifications: verifications}
}

// pending は、token に対応する確認待ちを作り、その id を返す。
func (w *signupWorld) pending(t *testing.T, email, token string) string {
	t.Helper()
	receipt, err := w.verifications.Create(w.ctx, domain.CreateSignupVerificationParams{
		Email:          email,
		Username:       "alice",
		PasswordDigest: "digest(" + token + ")",
		TokenHash:      domain.HashSignupToken(token),
	})
	if err != nil || !receipt.Accepted {
		t.Fatalf("確認待ちの作成 = (%+v, %v)", receipt, err)
	}
	return receipt.ID
}

func (w *signupWorld) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := w.pool.QueryRow(w.ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestUnitOfWorkSignupConfirm(t *testing.T) {
	t.Run("有効なトークンで確認すると、ユーザーが作られ、確認待ちは消え、同じトークンはもう使えない", func(t *testing.T) {
		w := newSignupWorld(t)
		w.pending(t, "Alice@Example.com", "tok")

		user, token, err := w.signups.Confirm(w.ctx, "tok")
		if err != nil {
			t.Fatalf("確認 error: %v", err)
		}
		if !domain.IsUUID(user.ID) || user.Username != "alice" || user.Email != "Alice@Example.com" || user.Admin {
			t.Errorf("作られたユーザー = %+v（管理者にはならない）", user)
		}
		if token == "" {
			t.Error("認証トークンが空")
		}
		var digest string
		if err := w.pool.QueryRow(w.ctx, "SELECT password_digest FROM users WHERE id = $1", user.ID).Scan(&digest); err != nil || digest != "digest(tok)" {
			t.Errorf("保存されたパスワードのハッシュ = %q (err %v), want 確認待ちのもの", digest, err)
		}
		if n := w.count(t, "signup_verifications"); n != 0 {
			t.Errorf("確認後の確認待ち = %d 行, want 0", n)
		}
		if _, _, err := w.signups.Confirm(w.ctx, "tok"); !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Errorf("同じトークンでの 2 回目の確認 error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
		if n := w.count(t, "users"); n != 1 {
			t.Errorf("ユーザー = %d 人, want 1", n)
		}
	})

	t.Run("確認までの間に同じメールアドレスのユーザーが作られていたら、区別できない失敗になり、確認待ちは残り、ユーザーは重複しない", func(t *testing.T) {
		w := newSignupWorld(t)
		w.pending(t, "alice@example.com", "tok")
		if _, err := w.pool.Exec(w.ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'other', 'other-digest')"); err != nil {
			t.Fatalf("別の経路でユーザーを作る: %v", err)
		}

		_, _, err := w.signups.Confirm(w.ctx, "tok")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) || errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("error = %v, want 区別できない %v だけ", err, domain.ErrSignupTokenInvalid)
		}
		if n := w.count(t, "users"); n != 1 {
			t.Errorf("ユーザー = %d 人, want 1（重複を作らない）", n)
		}
		if n := w.count(t, "signup_verifications"); n != 1 {
			t.Errorf("確認待ち = %d 行, want 1（ユーザーの作成に失敗したら、全体を取り消して、確認待ちを消さない）", n)
		}
	})

	t.Run("確認までの間に、大文字小文字だけが違うメールアドレスのユーザーが作られていても、区別できない失敗になり、ユーザーは重複しない", func(t *testing.T) {
		w := newSignupWorld(t)
		w.pending(t, "Case.User@Example.com", "case-token")
		if _, err := w.pool.Exec(w.ctx, `INSERT INTO users (email, username, password_digest) VALUES ('case.user@example.com', 'existing', 'd')`); err != nil {
			t.Fatal(err)
		}
		_, _, err := w.signups.Confirm(w.ctx, "case-token")
		if !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Fatalf("err = %v, want ErrSignupTokenInvalid", err)
		}
		var n int
		if err := w.pool.QueryRow(w.ctx, `SELECT count(*) FROM users WHERE lower(email) = 'case.user@example.com'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("大文字小文字を無視して同じメールの利用者が %d 人(err %v), want 1", n, err)
		}
	})

	t.Run("期限切れ・存在しないトークンは、ユーザーを作らずに失敗し、確認待ちは変わらない", func(t *testing.T) {
		w := newSignupWorld(t)
		w.pending(t, "alice@example.com", "tok")
		if _, err := w.pool.Exec(w.ctx, "UPDATE signup_verifications SET expires_at = now() - interval '1 second'"); err != nil {
			t.Fatalf("期限を過ぎさせる: %v", err)
		}
		for name, raw := range map[string]string{"期限切れ": "tok", "存在しない": "unknown", "空": ""} {
			if _, _, err := w.signups.Confirm(w.ctx, raw); !errors.Is(err, domain.ErrSignupTokenInvalid) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrSignupTokenInvalid)
			}
		}
		if n := w.count(t, "users"); n != 0 {
			t.Errorf("ユーザー = %d 人, want 0", n)
		}
		if n := w.count(t, "signup_verifications"); n != 1 {
			t.Errorf("確認待ち = %d 行, want 1（失敗した確認で行を消さない）", n)
		}
	})

	t.Run("同じトークンでの並行する確認は、ちょうど 1 件だけ成功し、負けた側はロックの段階で止まる", func(t *testing.T) {
		w := newSignupWorld(t)
		w.pending(t, "alice@example.com", "tok")

		const workers = 8
		errs := make([]error, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, errs[i] = w.signups.Confirm(w.ctx, "tok")
			}(i)
		}
		wg.Wait()

		succeeded := 0
		for _, err := range errs {
			switch {
			case err == nil:
				succeeded++
			case !errors.Is(err, domain.ErrSignupTokenInvalid):
				t.Errorf("成功でも ErrSignupTokenInvalid でもない error: %v", err)
			// ロックがあれば、負けた側は、先の確認が確定して確認待ちが消えたのを、ロックの段階で見て終わる。
			// ロックがないと、ユーザーの作成まで進んでから、メールアドレスの重複で失敗することになる。
			case !strings.Contains(err.Error(), "lock signup verification"):
				t.Errorf("負けた側が、ロックの段階で止まっていない: %v", err)
			}
		}
		if succeeded != 1 {
			t.Errorf("成功した確認 = %d 件, want 1", succeeded)
		}
		if n := w.count(t, "users"); n != 1 {
			t.Errorf("ユーザー = %d 人, want 1", n)
		}
		if n := w.count(t, "signup_verifications"); n != 0 {
			t.Errorf("確認待ち = %d 行, want 0", n)
		}
	})
}
