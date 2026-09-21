package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestOAuthTokenSessionQuery は、発行したトークンの記録の読み取りを、実際の PostgreSQL に対して検証する。
// データは SQL の INSERT で用意し、書き込みの adapter/repository には依存しない。
func TestOAuthTokenSessionQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed query test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	q := query.NewOAuthTokenSessionQuery(conn)

	userID := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ('a@example.com', 'alice', 'digest') RETURNING id`)
	grantID := dbtest.InsertUUIDRow(ctx, t, conn,
		`INSERT INTO oauth_grants (user_id, client_id, client_name, scopes) VALUES ($1, 'app', 'アプリ', ARRAY['hamburger:read']) RETURNING id`, userID)
	requestID := uid.N(1)
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	insert := func(kind, sig string, active bool) {
		t.Helper()
		if _, err := conn.Exec(ctx,
			`INSERT INTO oauth_token_sessions (kind, signature, request_id, grant_id, user_id, client_id, active, request, expires_at)
			 VALUES ($1, $2, $3, $4, $5, 'app', $6, '{"id":"x"}', $7)`,
			kind, sig, requestID, grantID, userID, active, expires); err != nil {
			t.Fatalf("insert token session: %v", err)
		}
	}
	insert("access_token", "live", true)
	insert("refresh_token", "used", false)

	t.Run("有効な記録を読むと、すべての項目が復元される", func(t *testing.T) {
		got, err := q.GetOAuthTokenSession(ctx, domain.OAuthTokenAccess, "live")
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != domain.OAuthTokenAccess || got.Signature != "live" || got.RequestID != requestID ||
			got.GrantID != grantID || got.UserID != userID || got.ClientID != "app" || !got.Active ||
			!got.ExpiresAt.Equal(expires) || string(got.Request) != `{"id": "x"}` {
			t.Errorf("session = %+v (request %s)", got, got.Request)
		}
	})

	t.Run("使用済み(無効)の記録も読める(再利用を検知するため)", func(t *testing.T) {
		got, err := q.GetOAuthTokenSession(ctx, domain.OAuthTokenRefresh, "used")
		if err != nil || got.Active {
			t.Errorf("session = %+v, err = %v, want an inactive session", got, err)
		}
	})

	t.Run("署名が同じでも、種類が違えば別の記録として扱い、なければ見つからないエラーになる", func(t *testing.T) {
		_, err := q.GetOAuthTokenSession(ctx, domain.OAuthTokenRefresh, "live")
		if !errors.Is(err, domain.ErrOAuthTokenSessionNotFound) {
			t.Errorf("err = %v, want ErrOAuthTokenSessionNotFound", err)
		}
	})
}
