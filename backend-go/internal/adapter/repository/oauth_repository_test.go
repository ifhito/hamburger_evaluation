package repository_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// insertOAuthUser は、許可の記録の持ち主になるユーザーを作り、その id を返す。
func insertOAuthUser(ctx context.Context, t *testing.T, conn *pgx.Conn, email string) string {
	t.Helper()
	return dbtest.InsertUserRow(ctx, t, conn,
		"INSERT INTO users (email, username, password_digest) VALUES ($1, 'oauth user', 'digest') RETURNING id", email)
}

func oauthScopesOf(ctx context.Context, t *testing.T, conn *pgx.Conn, grantID string) []string {
	t.Helper()
	var scopes []string
	if err := conn.QueryRow(ctx, "SELECT scopes FROM oauth_grants WHERE id = $1", grantID).Scan(&scopes); err != nil {
		t.Fatalf("select scopes: %v", err)
	}
	return scopes
}

// newTokenSession は、テスト用のトークンの記録を作る。
func newTokenSession(kind domain.OAuthTokenKind, signature, requestID, grantID, userID string, expires time.Time) domain.OAuthTokenSession {
	return domain.OAuthTokenSession{
		Kind: kind, Signature: signature, RequestID: requestID, GrantID: grantID, UserID: userID,
		ClientID: "app", Active: true, ExpiresAt: expires, Request: []byte(`{"k":"v"}`),
	}
}

func TestOAuthGrantRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()

	t.Run("初めて許可すると、記録が作られ、id が返る", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		repo := repository.NewOAuthGrantRepository(conn)
		id, err := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		if err != nil || !domain.IsUUID(id) {
			t.Fatalf("CreateOAuthGrant = (%q, %v)", id, err)
		}
		if got := oauthScopesOf(ctx, t, conn, id); !reflect.DeepEqual(got, []string{domain.OAuthScopeRead}) {
			t.Errorf("scopes = %v", got)
		}
	})

	t.Run("同じアプリに範囲を広げて許可し直すと、同じ id のまま、範囲が広がる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		repo := repository.NewOAuthGrantRepository(conn)
		first, err := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "旧名", Scopes: []string{domain.OAuthScopeRead}})
		if err != nil {
			t.Fatal(err)
		}
		second, err := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "新名", Scopes: []string{domain.OAuthScopeWrite}})
		if err != nil || second != first {
			t.Fatalf("second = (%q, %v), want id %q", second, err, first)
		}
		if got := oauthScopesOf(ctx, t, conn, first); !reflect.DeepEqual(got, []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}) {
			t.Errorf("scopes = %v, want read+write", got)
		}
		var name string
		if err := conn.QueryRow(ctx, "SELECT client_name FROM oauth_grants WHERE id = $1", first).Scan(&name); err != nil || name != "新名" {
			t.Errorf("client_name = %q, %v, want 新名", name, err)
		}
	})

	t.Run("範囲を狭めた許可の要求では、すでに許可した範囲は減らない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		repo := repository.NewOAuthGrantRepository(conn)
		id, _ := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}})
		if _, err := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}}); err != nil {
			t.Fatal(err)
		}
		if got := oauthScopesOf(ctx, t, conn, id); len(got) != 2 {
			t.Errorf("scopes = %v, want read+write kept", got)
		}
	})

	t.Run("別の利用者が同じアプリを許可すると、別の記録になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertOAuthUser(ctx, t, conn, "a@example.com")
		bob := insertOAuthUser(ctx, t, conn, "b@example.com")
		repo := repository.NewOAuthGrantRepository(conn)
		a, _ := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: alice, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		b, _ := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: bob, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		if a == b {
			t.Errorf("grant ids are equal: %s", a)
		}
	})

	t.Run("許可を取り消すと、記録と、その記録から発行したトークンの記録が、すべて消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		grants := repository.NewOAuthGrantRepository(conn)
		sessions := repository.NewOAuthTokenSessionRepository(conn)
		grantID, _ := grants.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		exp := time.Now().Add(time.Hour)
		for _, k := range []domain.OAuthTokenKind{domain.OAuthTokenAccess, domain.OAuthTokenRefresh} {
			if err := sessions.CreateOAuthTokenSession(ctx, newTokenSession(k, "sig-"+string(k), uid.N(1), grantID, userID, exp)); err != nil {
				t.Fatal(err)
			}
		}
		if err := grants.DiscardOAuthGrant(ctx, userID, grantID); err != nil {
			t.Fatalf("DiscardOAuthGrant: %v", err)
		}
		if n := countRows(ctx, t, conn, "oauth_grants"); n != 0 {
			t.Errorf("oauth_grants rows = %d, want 0", n)
		}
		if n := countRows(ctx, t, conn, "oauth_token_sessions"); n != 0 {
			t.Errorf("oauth_token_sessions rows = %d, want 0 (cascade)", n)
		}
	})

	t.Run("別の利用者の許可を取り消そうとすると、見つからないエラーになり、記録は残る", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		alice := insertOAuthUser(ctx, t, conn, "a@example.com")
		bob := insertOAuthUser(ctx, t, conn, "b@example.com")
		repo := repository.NewOAuthGrantRepository(conn)
		grantID, _ := repo.CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: alice, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		if err := repo.DiscardOAuthGrant(ctx, bob, grantID); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("DiscardOAuthGrant = %v, want ErrOAuthGrantNotFound", err)
		}
		if n := countRows(ctx, t, conn, "oauth_grants"); n != 1 {
			t.Errorf("oauth_grants rows = %d, want 1", n)
		}
	})

	t.Run("存在しない許可を取り消そうとすると、見つからないエラーになる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		err := repository.NewOAuthGrantRepository(conn).DiscardOAuthGrant(ctx, userID, uid.N(9))
		if !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("DiscardOAuthGrant = %v, want ErrOAuthGrantNotFound", err)
		}
	})

	t.Run("利用者が退会(削除)されると、その許可の記録も消える", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		userID := insertOAuthUser(ctx, t, conn, "a@example.com")
		if _, err := repository.NewOAuthGrantRepository(conn).CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}}); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Fatal(err)
		}
		if n := countRows(ctx, t, conn, "oauth_grants"); n != 0 {
			t.Errorf("oauth_grants rows = %d, want 0", n)
		}
	})
}

func TestOAuthTokenSessionRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	// setup は、ユーザーと許可の記録を 1 つ作り、その id を返す。
	setup := func(t *testing.T) (conn *pgx.Conn, url, userID, grantID string) {
		t.Helper()
		conn, url = dbtest.New(t)
		userID = insertOAuthUser(ctx, t, conn, "a@example.com")
		var err error
		grantID, err = repository.NewOAuthGrantRepository(conn).CreateOAuthGrant(ctx, domain.CreateOAuthGrantParams{UserID: userID, ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}})
		if err != nil {
			t.Fatal(err)
		}
		return conn, url, userID, grantID
	}
	activeOf := func(t *testing.T, conn *pgx.Conn, kind domain.OAuthTokenKind, sig string) (active, found bool) {
		t.Helper()
		err := conn.QueryRow(ctx, "SELECT active FROM oauth_token_sessions WHERE kind = $1 AND signature = $2", string(kind), sig).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, false
		}
		if err != nil {
			t.Fatal(err)
		}
		return active, true
	}

	t.Run("記録を保存すると、有効な状態で読み出せる", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		if err := repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "s1", uid.N(1), grantID, userID, future)); err != nil {
			t.Fatal(err)
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenAccess, "s1"); !found || !active {
			t.Errorf("active = %v, found = %v, want an active row", active, found)
		}
	})

	t.Run("同じ種類・同じ署名の記録は、2 つ保存できない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		s := newTokenSession(domain.OAuthTokenAccess, "dup", uid.N(1), grantID, userID, future)
		if err := repo.CreateOAuthTokenSession(ctx, s); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateOAuthTokenSession(ctx, s); err == nil {
			t.Error("2 回目の保存がエラーにならなかった")
		}
	})

	t.Run("存在しない許可の記録を指すトークンの記録は、保存できない", func(t *testing.T) {
		conn, _, userID, _ := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		err := repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "orphan", uid.N(1), uid.N(7), userID, future))
		if err == nil {
			t.Error("存在しない許可を指す記録が保存できてしまった")
		}
	})

	t.Run("認可コードを使うと無効になり、もう一度使うと、使用済みのエラーになる", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAuthorizationCode, "code", uid.N(1), grantID, userID, future))
		if err := repo.UpdateOAuthTokenSessionInactive(ctx, domain.OAuthTokenAuthorizationCode, "code"); err != nil {
			t.Fatalf("1 回目: %v", err)
		}
		if err := repo.UpdateOAuthTokenSessionInactive(ctx, domain.OAuthTokenAuthorizationCode, "code"); !errors.Is(err, domain.ErrOAuthTokenSessionInactive) {
			t.Errorf("2 回目 = %v, want ErrOAuthTokenSessionInactive", err)
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenAuthorizationCode, "code"); !found || active {
			t.Errorf("使用済みの記録は、無効な状態で残るはず (active=%v found=%v)", active, found)
		}
	})

	t.Run("存在しない認可コードを使おうとすると、見つからないエラーになる", func(t *testing.T) {
		conn, _, _, _ := setup(t)
		err := repository.NewOAuthTokenSessionRepository(conn).UpdateOAuthTokenSessionInactive(ctx, domain.OAuthTokenAuthorizationCode, "nothing")
		if !errors.Is(err, domain.ErrOAuthTokenSessionNotFound) {
			t.Errorf("err = %v, want ErrOAuthTokenSessionNotFound", err)
		}
	})

	t.Run("同じ認可コードを並行して使おうとしても、成功するのは 1 つだけである", func(t *testing.T) {
		conn, url, userID, grantID := setup(t)
		_ = repository.NewOAuthTokenSessionRepository(conn).CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAuthorizationCode, "race", uid.N(1), grantID, userID, future))
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		repo := repository.NewOAuthTokenSessionRepository(pool)
		const workers = 8
		var wg sync.WaitGroup
		results := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- repo.UpdateOAuthTokenSessionInactive(ctx, domain.OAuthTokenAuthorizationCode, "race")
			}()
		}
		wg.Wait()
		close(results)
		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
			} else if !errors.Is(err, domain.ErrOAuthTokenSessionInactive) {
				t.Errorf("unexpected error: %v", err)
			}
		}
		if succeeded != 1 {
			t.Errorf("succeeded = %d, want exactly 1", succeeded)
		}
	})

	t.Run("更新トークンを入れ替えると、古い更新トークンは無効で残り、同じ系列のアクセストークンは消え、別の系列は変わらない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		mine, other := uid.N(1), uid.N(2)
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "r-mine", mine, grantID, userID, future))
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "a-mine", mine, grantID, userID, future))
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "a-other", other, grantID, userID, future))
		if err := repo.UpdateOAuthRefreshRotated(ctx, mine, "r-mine"); err != nil {
			t.Fatalf("Rotate: %v", err)
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenRefresh, "r-mine"); !found || active {
			t.Errorf("古い更新トークン: active=%v found=%v, want inactive kept", active, found)
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "a-mine"); found {
			t.Error("同じ系列のアクセストークンが残っている")
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "a-other"); !found {
			t.Error("別の系列のアクセストークンまで消えた")
		}
	})

	t.Run("入れ替え済みの更新トークンを、もう一度入れ替えようとすると、使用済みのエラーになり、何も消えない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		req := uid.N(1)
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "r", req, grantID, userID, future))
		_ = repo.UpdateOAuthRefreshRotated(ctx, req, "r")
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "a-new", req, grantID, userID, future))
		if err := repo.UpdateOAuthRefreshRotated(ctx, req, "r"); !errors.Is(err, domain.ErrOAuthTokenSessionInactive) {
			t.Errorf("err = %v, want ErrOAuthTokenSessionInactive", err)
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "a-new"); !found {
			t.Error("失敗した入れ替えが、新しいアクセストークンを消した")
		}
	})

	t.Run("別の系列の id を指定して更新トークンを入れ替えようとすると、入れ替えられず、何も変わらない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "r", uid.N(1), grantID, userID, future))
		if err := repo.UpdateOAuthRefreshRotated(ctx, uid.N(2), "r"); err == nil {
			t.Fatal("系列が違うのに、入れ替えが成功した")
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenRefresh, "r"); !found || !active {
			t.Errorf("更新トークンが変わった: active=%v found=%v", active, found)
		}
	})

	t.Run("同じ更新トークンを並行して入れ替えようとしても、成功するのは 1 つだけである", func(t *testing.T) {
		conn, url, userID, grantID := setup(t)
		req := uid.N(1)
		_ = repository.NewOAuthTokenSessionRepository(conn).CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "rr", req, grantID, userID, future))
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		repo := repository.NewOAuthTokenSessionRepository(pool)
		const workers = 8
		var wg sync.WaitGroup
		results := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- repo.UpdateOAuthRefreshRotated(ctx, req, "rr")
			}()
		}
		wg.Wait()
		close(results)
		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Errorf("succeeded = %d, want exactly 1", succeeded)
		}
	})

	t.Run("系列のアクセストークンを削除し、更新トークンを無効にしても、別の系列と、ほかの種類は変わらない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		mine, other := uid.N(1), uid.N(2)
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "a1", mine, grantID, userID, future))
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "r1", mine, grantID, userID, future))
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "a2", other, grantID, userID, future))
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenRefresh, "r2", other, grantID, userID, future))
		if err := repo.DiscardOAuthTokenSessionsByRequest(ctx, mine, domain.OAuthTokenAccess); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateOAuthTokenSessionsInactiveByRequest(ctx, mine, domain.OAuthTokenRefresh); err != nil {
			t.Fatal(err)
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "a1"); found {
			t.Error("a1 が残っている")
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenRefresh, "r1"); !found || active {
			t.Errorf("r1: active=%v found=%v, want inactive kept", active, found)
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "a2"); !found {
			t.Error("別の系列の a2 が消えた")
		}
		if active, found := activeOf(t, conn, domain.OAuthTokenRefresh, "r2"); !found || !active {
			t.Errorf("別の系列の r2: active=%v found=%v, want active", active, found)
		}
	})

	t.Run("記録を削除しても、なければエラーにならない", func(t *testing.T) {
		conn, _, _, _ := setup(t)
		if err := repository.NewOAuthTokenSessionRepository(conn).DiscardOAuthTokenSession(ctx, domain.OAuthTokenPKCE, "none"); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("期限切れの記録だけが、上限の件数まで削除される", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		repo := repository.NewOAuthTokenSessionRepository(conn)
		past := time.Now().Add(-time.Hour)
		for i, sig := range []string{"e1", "e2", "e3"} {
			_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, sig, uid.N(i+1), grantID, userID, past))
		}
		_ = repo.CreateOAuthTokenSession(ctx, newTokenSession(domain.OAuthTokenAccess, "live", uid.N(9), grantID, userID, future))
		n, err := repo.DiscardExpiredOAuthTokenSessions(ctx, 2)
		if err != nil || n != 2 {
			t.Fatalf("1 回目 = (%d, %v), want 2 deleted", n, err)
		}
		n, err = repo.DiscardExpiredOAuthTokenSessions(ctx, 2)
		if err != nil || n != 1 {
			t.Fatalf("2 回目 = (%d, %v), want 1 deleted", n, err)
		}
		if _, found := activeOf(t, conn, domain.OAuthTokenAccess, "live"); !found {
			t.Error("期限内の記録まで消えた")
		}
	})

	t.Run("知らない種類の記録は、表の制約で保存できない", func(t *testing.T) {
		conn, _, userID, grantID := setup(t)
		err := repository.NewOAuthTokenSessionRepository(conn).CreateOAuthTokenSession(ctx, newTokenSession("password", "s", uid.N(1), grantID, userID, future))
		if err == nil {
			t.Error("知らない種類の記録が保存できてしまった")
		}
	})
}
