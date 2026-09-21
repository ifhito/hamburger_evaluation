package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// signupParams は、テスト用の確認待ちの入力を作る。token は tokenHash になる。
func signupParams(email, token string) domain.CreateSignupVerificationParams {
	return domain.CreateSignupVerificationParams{
		Email:          email,
		Username:       "alice",
		PasswordDigest: "digest(" + token + ")",
		TokenHash:      domain.HashSignupToken(token),
	}
}

func countRows(ctx context.Context, t *testing.T, conn *pgx.Conn, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestSignupVerificationRepository は、確認待ちの signup の書き込みを実際の PostgreSQL に対して
// 検証する（S16）。TEST_DATABASE_URL がなければ、dbtest.New の内部で skip される。
func TestSignupVerificationRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	t.Run("作成した確認待ちは受理され、間隔内の再作成は何も変えずに見送られる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)

		first, err := repo.CreateSignupVerification(ctx, signupParams("Alice@Example.com", "first"))
		if err != nil || !first.Accepted || !domain.IsUUID(first.ID) || first.Generation != 1 {
			t.Fatalf("1 回目 = (%+v, %v), want Accepted・UUID の ID・世代 1", first, err)
		}
		// 大文字小文字だけが違う email は、同じ確認待ちとして扱われ、間隔内なので見送られる。
		skipped, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "second"))
		if err != nil || skipped != (domain.SignupVerificationReceipt{}) {
			t.Fatalf("間隔内の 2 回目 = (%+v, %v), want 空の結果（Accepted=false）", skipped, err)
		}
		var email, tokenHash string
		if err := conn.QueryRow(ctx, "SELECT email, token_hash FROM signup_verifications").Scan(&email, &tokenHash); err != nil {
			t.Fatalf("select: %v", err)
		}
		if email != "Alice@Example.com" || tokenHash != domain.HashSignupToken("first") {
			t.Errorf("見送られたのに行が変わった: email=%q token_hash=%q", email, tokenHash)
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 1 {
			t.Errorf("行数 = %d, want 1", n)
		}
	})

	t.Run("間隔を過ぎた再作成は、最新の入力とトークンに置き換わり、古いトークンは無効になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		if _, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "first")); err != nil {
			t.Fatalf("1 回目: %v", err)
		}
		if _, err := conn.Exec(ctx, "UPDATE signup_verifications SET last_sent_at = now() - interval '61 seconds'"); err != nil {
			t.Fatalf("last_sent_at を進める: %v", err)
		}
		again := signupParams("ALICE@example.com", "second")
		again.Username = "alice2"
		receipt, err := repo.CreateSignupVerification(ctx, again)
		if err != nil || !receipt.Accepted || receipt.Generation != 2 {
			t.Fatalf("間隔後の 2 回目 = (%+v, %v), want Accepted・世代 2（置き換えるたびに 1 増える）", receipt, err)
		}
		var email, username, digest, tokenHash string
		if err := conn.QueryRow(ctx, "SELECT email, username, password_digest, token_hash FROM signup_verifications").
			Scan(&email, &username, &digest, &tokenHash); err != nil {
			t.Fatalf("select: %v", err)
		}
		if email != "ALICE@example.com" || username != "alice2" || digest != again.PasswordDigest || tokenHash != again.TokenHash {
			t.Errorf("最新の入力に置き換わっていない: %q %q %q %q", email, username, digest, tokenHash)
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 1 {
			t.Errorf("行数 = %d, want 1", n)
		}
		if _, err := repo.CreateUserFromSignupVerification(ctx, domain.HashSignupToken("first")); !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Errorf("古いトークンでの確認 error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
	})

	t.Run("有効なトークンで確認すると users が作られ、確認待ちは消え、再利用はできない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		if _, err := repo.CreateSignupVerification(ctx, signupParams("Alice@Example.com", "tok")); err != nil {
			t.Fatalf("作成: %v", err)
		}
		user, err := repo.CreateUserFromSignupVerification(ctx, domain.HashSignupToken("tok"))
		if err != nil {
			t.Fatalf("確認 returned error: %v", err)
		}
		if !domain.IsUUID(user.ID) || user.Username != "alice" || user.Email != "Alice@Example.com" || user.Admin {
			t.Errorf("作られた user = %+v", user)
		}
		var digest string
		if err := conn.QueryRow(ctx, "SELECT password_digest FROM users WHERE id = $1", user.ID).Scan(&digest); err != nil || digest != "digest(tok)" {
			t.Errorf("users の password_digest = %q (err %v), want digest(tok)", digest, err)
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 0 {
			t.Errorf("確認後の確認待ち = %d 行, want 0", n)
		}
		if _, err := repo.CreateUserFromSignupVerification(ctx, domain.HashSignupToken("tok")); !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Errorf("再利用 error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
		if n := countRows(ctx, t, conn, "users"); n != 1 {
			t.Errorf("users = %d 行, want 1", n)
		}
	})

	t.Run("期限切れ・存在しないトークンは ErrSignupTokenInvalid で、何も書かれない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		if _, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "tok")); err != nil {
			t.Fatalf("作成: %v", err)
		}
		if _, err := conn.Exec(ctx, "UPDATE signup_verifications SET expires_at = now() - interval '1 second'"); err != nil {
			t.Fatalf("期限を過ぎさせる: %v", err)
		}
		for name, hash := range map[string]string{
			"期限切れ":  domain.HashSignupToken("tok"),
			"存在しない": domain.HashSignupToken("unknown"),
			"空":     domain.HashSignupToken(""),
		} {
			if _, err := repo.CreateUserFromSignupVerification(ctx, hash); !errors.Is(err, domain.ErrSignupTokenInvalid) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrSignupTokenInvalid)
			}
		}
		if n := countRows(ctx, t, conn, "users"); n != 0 {
			t.Errorf("users = %d 行, want 0", n)
		}
	})

	t.Run("確認までの間に同じ email の users が作られていたら ErrEmailTaken で、users は重複しない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		if _, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "tok")); err != nil {
			t.Fatalf("作成: %v", err)
		}
		if _, err := conn.Exec(ctx,
			"INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'other', 'other-digest')"); err != nil {
			t.Fatalf("別経路で users を作る: %v", err)
		}
		if _, err := repo.CreateUserFromSignupVerification(ctx, domain.HashSignupToken("tok")); !errors.Is(err, domain.ErrEmailTaken) {
			t.Fatalf("error = %v, want %v", err, domain.ErrEmailTaken)
		}
		if n := countRows(ctx, t, conn, "users"); n != 1 {
			t.Errorf("users = %d 行, want 1（重複を作らない）", n)
		}
	})

	t.Run("同じトークンでの並行する確認は、ちょうど 1 件だけ成功する", func(t *testing.T) {
		_, url := dbtest.New(t)
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		defer pool.Close()
		repo := repository.NewSignupVerificationRepository(pool)
		if _, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "tok")); err != nil {
			t.Fatalf("作成: %v", err)
		}
		const workers = 8
		errs := make([]error, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = repo.CreateUserFromSignupVerification(ctx, domain.HashSignupToken("tok"))
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
			}
		}
		if succeeded != 1 {
			t.Errorf("成功した確認 = %d 件, want 1", succeeded)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil || n != 1 {
			t.Errorf("users = %d 行 (err %v), want 1", n, err)
		}
	})

	t.Run("期限切れの掃除は、上限の件数までだけ消し、有効な行は残す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		for i, token := range []string{"a", "b", "c", "d"} {
			email := token + "@example.com"
			if _, err := repo.CreateSignupVerification(ctx, signupParams(email, token)); err != nil {
				t.Fatalf("作成 %d: %v", i, err)
			}
		}
		if _, err := conn.Exec(ctx,
			"UPDATE signup_verifications SET expires_at = now() - interval '1 hour' WHERE email <> 'd@example.com'"); err != nil {
			t.Fatalf("期限を過ぎさせる: %v", err)
		}
		if n, err := repo.DiscardExpiredSignupVerifications(ctx, 2); err != nil || n != 2 {
			t.Fatalf("上限 2 の掃除 = (%d, %v), want (2, nil)", n, err)
		}
		if n, err := repo.DiscardExpiredSignupVerifications(ctx, 10); err != nil || n != 1 {
			t.Fatalf("残りの掃除 = (%d, %v), want (1, nil)", n, err)
		}
		var remaining string
		if err := conn.QueryRow(ctx, "SELECT email FROM signup_verifications").Scan(&remaining); err != nil || remaining != "d@example.com" {
			t.Errorf("残った行 = %q (err %v), want d@example.com のみ", remaining, err)
		}
	})
}
