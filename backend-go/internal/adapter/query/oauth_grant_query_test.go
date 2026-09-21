package query_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestOAuthGrantQuery は、利用者が許可したアプリの記録の読み取りを、実際の PostgreSQL に対して検証する。
// データは SQL の INSERT で用意し、書き込みの adapter/repository には依存しない。
func TestOAuthGrantQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	q := query.NewOAuthGrantQuery(conn)

	alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('a@example.com', 'alice', 'd') RETURNING id`)
	bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('b@example.com', 'bob', 'd') RETURNING id`)
	insert := func(userID, clientID, name string, scopes []string, updated time.Time) string {
		t.Helper()
		return dbtest.InsertUUIDRow(ctx, t, conn,
			`INSERT INTO oauth_grants (user_id, client_id, client_name, scopes, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`,
			userID, clientID, name, scopes, updated)
	}
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	older := insert(alice, "app-old", "古いアプリ", []string{domain.OAuthScopeRead}, base)
	newer := insert(alice, "app-new", "新しいアプリ", []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}, base.Add(time.Hour))
	insert(bob, "app-old", "ボブのアプリ", []string{domain.OAuthScopeRead}, base.Add(2*time.Hour))

	t.Run("利用者とアプリの組で許可を読むと、すべての項目が復元される", func(t *testing.T) {
		got, err := q.GetOAuthGrantByUserAndClient(ctx, alice, "app-new")
		if err != nil {
			t.Fatal(err)
		}
		want := domain.OAuthGrant{
			ID: newer, UserID: alice, ClientID: "app-new", ClientName: "新しいアプリ",
			Scopes: []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}, CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour),
		}
		if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
			t.Errorf("timestamps = %v / %v", got.CreatedAt, got.UpdatedAt)
		}
		got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("grant = %+v, want %+v", got, want)
		}
	})

	t.Run("許可していないアプリの組は、見つからないエラーになり、別の利用者の許可は読めない", func(t *testing.T) {
		if _, err := q.GetOAuthGrantByUserAndClient(ctx, alice, "unknown"); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("err = %v, want ErrOAuthGrantNotFound", err)
		}
		carol := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('c@example.com', 'carol', 'd') RETURNING id`)
		if _, err := q.GetOAuthGrantByUserAndClient(ctx, carol, "app-old"); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("別の利用者の許可が読めた: %v", err)
		}
	})

	t.Run("許可した一覧は、その利用者のものだけを、最近使ったものから順に返す", func(t *testing.T) {
		got, err := q.ListOAuthGrantsByUser(ctx, alice)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != newer || got[1].ID != older {
			t.Errorf("grants = %+v, want [%s %s]", got, newer, older)
		}
	})

	t.Run("何も許可していない利用者の一覧は、nil ではなく空の一覧である", func(t *testing.T) {
		dave := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('d@example.com', 'dave', 'd') RETURNING id`)
		got, err := q.ListOAuthGrantsByUser(ctx, dave)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("grants = %#v, err = %v, want an empty non-nil slice", got, err)
		}
	})
}
