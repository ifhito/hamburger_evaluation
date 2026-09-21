package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestUserQuery は、user の読み取り（adapter/query の UserQuery）を、共有の dbtest の
// スキャフォールドを通じて実際の PostgreSQL に対して検証する（TEST_DATABASE_URL がなければ
// スキップする）。データは SQL の INSERT で用意し、書き込みの adapter/repository には依存しない。
func TestUserQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	userQuery := query.NewUserQuery(conn)

	aliceID := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`,
		"alice@example.com", "alice", "digest-alice", false)
	wantUser := domain.User{ID: aliceID, Username: "alice", Email: "alice@example.com", Admin: false}

	t.Run("GetActiveUserByEmail は user と digest を返す", func(t *testing.T) {
		creds, err := userQuery.GetActiveUserByEmail(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("GetActiveUserByEmail returned error: %v", err)
		}
		if creds.User != wantUser {
			t.Fatalf("GetActiveUserByEmail user = %+v, want %+v", creds.User, wantUser)
		}
		if creds.PasswordDigest != "digest-alice" {
			t.Fatalf("GetActiveUserByEmail digest = %q, want %q", creds.PasswordDigest, "digest-alice")
		}
	})

	t.Run("GetActiveUserByID は user を返す", func(t *testing.T) {
		user, err := userQuery.GetActiveUserByID(ctx, aliceID)
		if err != nil {
			t.Fatalf("GetActiveUserByID returned error: %v", err)
		}
		if user != wantUser {
			t.Fatalf("GetActiveUserByID = %+v, want %+v", user, wantUser)
		}
	})

	t.Run("存在しない email と id は ErrUserNotFound になる", func(t *testing.T) {
		if _, err := userQuery.GetActiveUserByEmail(ctx, "nobody@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByEmail error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if _, err := userQuery.GetActiveUserByID(ctx, uid.N(1000)); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByID error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("discard 済みの user は active な検索から除外される", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE users SET discarded_at = now() WHERE id = $1", aliceID); err != nil {
			t.Fatalf("discard user: %v", err)
		}
		if _, err := userQuery.GetActiveUserByEmail(ctx, "alice@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByEmail error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if _, err := userQuery.GetActiveUserByID(ctx, aliceID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("GetActiveUserByID error = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("メールでの検索は、大文字小文字を区別せず、退会済みの利用者は含めない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('Alice@Example.com', 'alice', 'd') RETURNING id`)
		dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest, discarded_at) VALUES ('gone@example.com', 'gone', 'd', now()) RETURNING id`)
		q := query.NewUserQuery(conn)

		if u, err := q.GetActiveUserByEmailIgnoreCase(ctx, "alice@example.COM"); err != nil || u.Username != "alice" {
			t.Fatalf("大文字小文字違いの検索 = %+v, %v", u, err)
		}
		if _, err := q.GetActiveUserByEmailIgnoreCase(ctx, "gone@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("退会済みが見つかった: %v", err)
		}
		if _, err := q.GetActiveUserByEmailIgnoreCase(ctx, "nobody@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("知らないメール = %v", err)
		}
	})

	t.Run("パスワードの有無を引け、パスワードなしのアカウントのログイン用の digest は空文字列になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		none := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username) VALUES ('g@example.com', 'g') RETURNING id`)
		with := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('p@example.com', 'p', 'digest') RETURNING id`)
		q := query.NewUserQuery(conn)

		if has, err := q.GetActiveUserHasPassword(ctx, none); err != nil || has {
			t.Fatalf("パスワードなし: has = %v, err = %v", has, err)
		}
		if has, err := q.GetActiveUserHasPassword(ctx, with); err != nil || !has {
			t.Fatalf("パスワードあり: has = %v, err = %v", has, err)
		}
		if _, err := q.GetActiveUserHasPassword(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("知らない利用者 = %v", err)
		}
		creds, err := query.NewUserQuery(conn).GetActiveUserByEmail(ctx, "g@example.com")
		if err != nil || creds.PasswordDigest != "" {
			t.Fatalf("パスワードなしの digest = %q, err = %v", creds.PasswordDigest, err)
		}
	})
}
