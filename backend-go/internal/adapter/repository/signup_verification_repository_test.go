package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
// 検証する。TEST_DATABASE_URL がなければ、dbtest.New の内部で skip される。
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
		if err := repo.LockSignupVerification(ctx, domain.HashSignupToken("first")); !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Errorf("古いトークンでのロック error = %v, want %v", err, domain.ErrSignupTokenInvalid)
		}
	})

	t.Run("有効なトークンならロックでき、id を指定して削除すると、同じトークンはもう使えない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewSignupVerificationRepository(conn)
		receipt, err := repo.CreateSignupVerification(ctx, signupParams("Alice@Example.com", "tok"))
		if err != nil || !receipt.Accepted {
			t.Fatalf("作成 = (%+v, %v)", receipt, err)
		}
		if err := repo.LockSignupVerification(ctx, domain.HashSignupToken("tok")); err != nil {
			t.Fatalf("有効なトークンのロック error: %v", err)
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 1 {
			t.Errorf("ロックだけでは確認待ちは変わらない: %d 行, want 1", n)
		}
		if err := repo.DiscardSignupVerification(ctx, receipt.ID); err != nil {
			t.Fatalf("削除 error: %v", err)
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 0 {
			t.Errorf("削除後の確認待ち = %d 行, want 0", n)
		}
		if err := repo.LockSignupVerification(ctx, domain.HashSignupToken("tok")); !errors.Is(err, domain.ErrSignupTokenInvalid) {
			t.Errorf("削除後のロック error = %v, want %v（使用済みと同じ扱い）", err, domain.ErrSignupTokenInvalid)
		}
	})

	t.Run("期限切れ・存在しないトークンのロックは ErrSignupTokenInvalid で、確認待ちは変わらない", func(t *testing.T) {
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
			if err := repo.LockSignupVerification(ctx, hash); !errors.Is(err, domain.ErrSignupTokenInvalid) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrSignupTokenInvalid)
			}
		}
		if n := countRows(ctx, t, conn, "signup_verifications"); n != 1 {
			t.Errorf("確認待ち = %d 行, want 1（ロックの失敗で行を消さない）", n)
		}
	})

	t.Run("トランザクションの中でロックすると、同じ確認待ちのロックは、先のトランザクションが終わるまで待たされる", func(t *testing.T) {
		_, url := dbtest.New(t)
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		defer pool.Close()
		repo := repository.NewSignupVerificationRepository(pool)
		receipt, err := repo.CreateSignupVerification(ctx, signupParams("alice@example.com", "tok"))
		if err != nil || !receipt.Accepted {
			t.Fatalf("作成 = (%+v, %v)", receipt, err)
		}
		hash := domain.HashSignupToken("tok")

		first, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("先のトランザクションの開始: %v", err)
		}
		defer func() { _ = first.Rollback(ctx) }()
		if err := repository.NewSignupVerificationRepository(first).LockSignupVerification(ctx, hash); err != nil {
			t.Fatalf("先のロック error: %v", err)
		}

		second, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("後のトランザクションの開始: %v", err)
		}
		defer func() { _ = second.Rollback(ctx) }()
		done := make(chan error, 1)
		go func() { done <- repository.NewSignupVerificationRepository(second).LockSignupVerification(ctx, hash) }()
		select {
		case err := <-done:
			t.Fatalf("先のロックが残っているのに、後のロックが待たずに終わった: %v", err)
		case <-time.After(300 * time.Millisecond):
		}

		// 先のトランザクションが確認待ちを削除して確定すると、待っていた側は、行が消えているのを見て、
		// 使用済みと同じ結果になる。
		if err := repository.NewSignupVerificationRepository(first).DiscardSignupVerification(ctx, receipt.ID); err != nil {
			t.Fatalf("先のトランザクションでの削除: %v", err)
		}
		if err := first.Commit(ctx); err != nil {
			t.Fatalf("先のトランザクションの確定: %v", err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, domain.ErrSignupTokenInvalid) {
				t.Errorf("待っていたロックの結果 = %v, want %v", err, domain.ErrSignupTokenInvalid)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("先のトランザクションが終わっても、待っていたロックが返らない")
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
