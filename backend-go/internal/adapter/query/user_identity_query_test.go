package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestUserIdentityQuery は、外部のサービスのアカウントとの結び付きなどの読み取りを、実際の PostgreSQL に対して
// 検証する(書き込みの adapter/repository には依存しないので、データは SQL で直接入れる)。
func TestUserIdentityQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()

	t.Run("サービス上の ID で結び付きを引け、利用者ごとに結び付けた順に一覧できる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		if _, err := conn.Exec(ctx, `INSERT INTO user_identities (user_id, provider, provider_user_id, email) VALUES ($1, 'google', 'sub-1', 'alice@gmail.example')`, alice); err != nil {
			t.Fatal(err)
		}
		q := query.NewUserIdentityQuery(conn)

		got, err := q.GetIdentityByProviderUserID(ctx, domain.ProviderGoogle, "sub-1")
		if err != nil || got.UserID != alice || got.Email != "alice@gmail.example" || got.ProviderUserID != "sub-1" || !domain.IsUUID(got.ID) {
			t.Fatalf("GetIdentityByProviderUserID = %+v, %v", got, err)
		}
		list, err := q.ListIdentitiesByUser(ctx, alice)
		if err != nil || len(list) != 1 || list[0].ProviderUserID != "sub-1" {
			t.Fatalf("ListIdentitiesByUser = %+v, %v", list, err)
		}
		if _, err := q.GetIdentityByProviderUserID(ctx, domain.ProviderGoogle, "unknown"); !errors.Is(err, domain.ErrIdentityNotFound) {
			t.Fatalf("知らない ID = %v, want ErrIdentityNotFound", err)
		}
		if others, err := q.ListIdentitiesByUser(ctx, "00000000-0000-4000-8000-000000000000"); err != nil || len(others) != 0 {
			t.Fatalf("結び付きのない利用者の一覧 = %+v, %v, want 空", others, err)
		}
	})

	t.Run("メールでの検索は、大文字小文字を区別せず、退会済みの利用者は含めない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('Alice@Example.com', 'alice', 'd') RETURNING id`)
		dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest, discarded_at) VALUES ('gone@example.com', 'gone', 'd', now()) RETURNING id`)
		q := query.NewUserIdentityQuery(conn)

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
		q := query.NewUserIdentityQuery(conn)

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
