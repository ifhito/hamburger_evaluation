package query_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestUserSessionQuery は、セッショントークンのハッシュと利用者 ID による有効ユーザーの解決を
// 実際の PostgreSQL に対して検証する（adapter/repository には依存せず SQL でデータを用意する）。
func TestUserSessionQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	hash := func(tag byte) string { return strings.Repeat(string(tag), 64) }

	t.Run("有効期限内かつ退会していない利用者の自己紹介と管理者権限を含めて返す", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn,
			`INSERT INTO users (email, username, bio, password_digest, admin) VALUES ('alice@example.com', 'alice', 'burger lover', 'd', true) RETURNING id`)
		if _, err := conn.Exec(ctx,
			`INSERT INTO user_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`,
			alice, hash('a')); err != nil {
			t.Fatal(err)
		}

		got, err := query.NewUserSessionQuery(conn).GetActiveUserBySessionTokenHash(ctx, alice, hash('a'))
		want := domain.User{ID: alice, Username: "alice", Bio: "burger lover", Email: "alice@example.com", Admin: true}
		if err != nil || got != want {
			t.Fatalf("GetActiveUserBySessionTokenHash = %+v, %v, want %+v", got, err, want)
		}
	})

	t.Run("期限切れ・未知のハッシュ・利用者 ID の不一致・退会済み利用者はすべて ErrUserSessionNotFound になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
		bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'd') RETURNING id`)
		gone := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest, discarded_at) VALUES ('gone@example.com', 'gone', 'd', now()) RETURNING id`)

		if _, err := conn.Exec(ctx, `INSERT INTO user_sessions (user_id, token_hash, expires_at) VALUES
			($1, $2, now() - interval '1 second'),
			($1, $3, now() + interval '1 hour'),
			($4, $5, now() + interval '1 hour')`,
			alice, hash('e'), hash('v'), gone, hash('g')); err != nil {
			t.Fatal(err)
		}

		q := query.NewUserSessionQuery(conn)
		cases := []struct {
			name   string
			userID string
			hash   string
		}{
			{"期限切れのセッション", alice, hash('e')},
			{"存在しないトークンハッシュ", alice, hash('x')},
			{"トークンハッシュは存在するが別の利用者 ID", bob, hash('v')},
			{"退会済み利用者のセッション", gone, hash('g')},
		}
		for _, tc := range cases {
			if _, err := q.GetActiveUserBySessionTokenHash(ctx, tc.userID, tc.hash); !errors.Is(err, domain.ErrUserSessionNotFound) {
				t.Errorf("%s: err = %v, want ErrUserSessionNotFound", tc.name, err)
			}
		}
	})
}
