package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestUserSessionRepository は、セッショントークン記録の作成・個別失効・利用者単位失効・期限切れ掃除を
// 実際の PostgreSQL に対して検証する。
func TestUserSessionRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	hash := func(tag byte) string { return strings.Repeat(string(tag), 64) }

	t.Run("保存したセッションはトークンハッシュ指定で削除でき、別セッションは残る", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewUserSessionRepository(conn)

		if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: hash('a'), TTL: time.Hour}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: hash('b'), TTL: time.Hour}); err != nil {
			t.Fatal(err)
		}
		if err := repo.DiscardUserSessionByTokenHash(ctx, hash('a')); err != nil {
			t.Fatal(err)
		}
		if n := countRows(ctx, t, conn, "user_sessions"); n != 1 {
			t.Fatalf("残りのセッション = %d, want 1", n)
		}
	})

	t.Run("利用者単位の削除では対象利用者のセッションだけがすべて消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'd') RETURNING id`)
		repo := repository.NewUserSessionRepository(conn)

		for _, p := range []domain.CreateUserSessionParams{
			{UserID: alice, TokenHash: hash('1'), TTL: time.Hour},
			{UserID: alice, TokenHash: hash('2'), TTL: time.Hour},
			{UserID: bob, TokenHash: hash('3'), TTL: time.Hour},
		} {
			if err := repo.CreateUserSession(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.DiscardUserSessionsByUser(ctx, alice); err != nil {
			t.Fatal(err)
		}
		if n := countRows(ctx, t, conn, "user_sessions"); n != 1 {
			t.Fatalf("残りのセッション = %d, want 1(bob の分だけ)", n)
		}
	})

	t.Run("期限切れの掃除は上限件数まで削除し、有効期限内のセッションは残す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewUserSessionRepository(conn)

		for _, tag := range []byte{'1', '2', '3', '4'} {
			if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: hash(tag), TTL: time.Hour}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.Exec(ctx, `UPDATE user_sessions SET expires_at = now() - interval '1 second' WHERE token_hash <> $1`, hash('4')); err != nil {
			t.Fatal(err)
		}
		deleted, err := repo.DiscardExpiredUserSessions(ctx, 2)
		if err != nil || deleted != 2 {
			t.Fatalf("1 回目の掃除 = %d, %v, want 2 件", deleted, err)
		}
		deleted2, err := repo.DiscardExpiredUserSessions(ctx, 2)
		if err != nil || deleted2 != 1 {
			t.Fatalf("2 回目の掃除 = %d, %v, want 1 件", deleted2, err)
		}
		var surviving string
		if err := conn.QueryRow(ctx, `SELECT token_hash FROM user_sessions`).Scan(&surviving); err != nil || surviving != hash('4') {
			t.Fatalf("残った行 = %q (%v), want %q", surviving, err, hash('4'))
		}
	})

	t.Run("64 文字でないトークンハッシュや重複するハッシュは DB 制約で拒否される", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		repo := repository.NewUserSessionRepository(conn)

		if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: "too-short", TTL: time.Hour}); err == nil {
			t.Fatal("64 文字でないハッシュが保存できてしまった")
		}
		if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: hash('z'), TTL: time.Hour}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateUserSession(ctx, domain.CreateUserSessionParams{UserID: alice, TokenHash: hash('z'), TTL: time.Hour}); err == nil {
			t.Fatal("重複するトークンハッシュが保存できてしまった")
		}
	})
}
