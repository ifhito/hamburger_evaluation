package oauthserver_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 認可コードを発行するときに、許可の記録が、その利用者・そのアプリのもので、求められた範囲を
// 許可済みであることを、保存先から確かめる。
func TestIssueAuthorizationCodeChecksTheGrant(t *testing.T) {
	_, challenge := pkcePair()

	t.Run("読み取りだけを許可した記録で、書き込みを含む認可コードを発行しようとすると、断り、トークンの記録は作らない", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		params := authParams(staticClientID, staticRedirect, challenge, "scope", "hamburger:read hamburger:write")

		loc, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, grantID)
		if !errors.Is(err, domain.ErrOAuthInvalidScope) {
			t.Errorf("err = %v (redirect %q), want ErrOAuthInvalidScope", err, loc)
		}
		if n := r.count(`SELECT count(*) FROM oauth_token_sessions`); n != 0 {
			t.Errorf("トークンの記録が %d 件作られた, want 0", n)
		}
	})

	t.Run("別の利用者の許可の記録では、認可コードを発行できない", func(t *testing.T) {
		r := newRig(t)
		bob := dbtest.InsertUserRow(r.ctx, t, r.conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'd') RETURNING id`)
		bobGrant, err := r.grants.Approve(r.ctx, bob, domain.OAuthClient{ID: staticClientID, Name: "Dev App"}, []string{domain.OAuthScopeRead, domain.OAuthScopeWrite})
		if err != nil {
			t.Fatal(err)
		}
		// alice(r.userID)が、bob の許可の記録の id で、書き込みを含む認可コードを求める。
		params := authParams(staticClientID, staticRedirect, challenge, "scope", "hamburger:read hamburger:write")
		if _, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, bobGrant); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("err = %v, want ErrOAuthGrantNotFound", err)
		}
		if n := r.count(`SELECT count(*) FROM oauth_token_sessions`); n != 0 {
			t.Errorf("トークンの記録が %d 件作られた, want 0", n)
		}
	})

	t.Run("別のアプリの許可の記録では、認可コードを発行できない", func(t *testing.T) {
		r := newRig(t)
		r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		otherGrant := r.approve(metadataURL, "Other App", domain.OAuthScopeRead, domain.OAuthScopeWrite)
		params := authParams(staticClientID, staticRedirect, challenge, "scope", "hamburger:read hamburger:write")
		if _, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, otherGrant); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("err = %v, want ErrOAuthGrantNotFound", err)
		}
	})

	t.Run("存在しない許可の記録では、認可コードを発行できない", func(t *testing.T) {
		r := newRig(t)
		params := authParams(staticClientID, staticRedirect, challenge)
		if _, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10"); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("err = %v, want ErrOAuthGrantNotFound", err)
		}
	})

	t.Run("許可済みの範囲に収まる要求には、発行でき、発行されるトークンは要求した範囲だけになる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead, domain.OAuthScopeWrite)
		verifier, ch := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, ch), grantID) // 要求は読み取りだけ
		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		if resp.Status != http.StatusOK || resp.str("scope") != domain.OAuthScopeRead {
			t.Errorf("status = %d scope = %q, want 200 read only", resp.Status, resp.str("scope"))
		}
	})
}

// slowTokenWrites は、トークンの記録の書き込みを、意図的に遅くする(テスト用のトリガー)。
// accessInsert は、アクセストークンの保存を遅くする(認可コードを使用済みにしたあと、トークンを保存する前に、
// 別の要求が割り込む状況を、確実に作る)。pkceDelete は、認可コードに結び付けた PKCE の情報の削除を遅くする
// (2 つの交換が、どちらも PKCE の確認を通ってから、認可コードの使用済み化で競い合う状況を作る)。
// テストごとの専用のデータベースにだけ作るので、本番には影響しない。
func slowTokenWrites(t *testing.T, r *rig, accessInsert, pkceDelete time.Duration) {
	t.Helper()
	sec := func(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', 3, 64) }
	sql := `CREATE FUNCTION slow_access_token_insert() RETURNS trigger LANGUAGE plpgsql AS $$
	        BEGIN PERFORM pg_sleep(` + sec(accessInsert) + `); RETURN NEW; END $$;
	        CREATE TRIGGER slow_access_token BEFORE INSERT ON oauth_token_sessions
	        FOR EACH ROW WHEN (NEW.kind = 'access_token') EXECUTE FUNCTION slow_access_token_insert();
	        CREATE FUNCTION slow_pkce_delete() RETURNS trigger LANGUAGE plpgsql AS $$
	        BEGIN PERFORM pg_sleep(` + sec(pkceDelete) + `); RETURN OLD; END $$;
	        CREATE TRIGGER slow_pkce BEFORE DELETE ON oauth_token_sessions
	        FOR EACH ROW WHEN (OLD.kind = 'pkce') EXECUTE FUNCTION slow_pkce_delete()`
	if _, err := r.pool.Exec(r.ctx, sql); err != nil {
		t.Fatalf("create slow triggers: %v", err)
	}
}

// 同じ認可コードを、ほぼ同時に 2 回交換したとき、先に認可コードを使用済みにした側のトークンが、
// トークンを保存する前に、あとから来た側の再利用の検知(系列の取り消し)に割り込まれて、取り消したはずなのに
// 有効なまま保存されてしまう、という状態にならない。勝った側のトークンは、負けた側の取り消しで、使えなくなる。
func TestConcurrentCodeExchangeLeavesNoLiveToken(t *testing.T) {
	r := newRig(t)
	slowTokenWrites(t, r, 600*time.Millisecond, 400*time.Millisecond)
	grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
	verifier, challenge := pkcePair()
	code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)

	results := make(chan tokenResponse, 2)
	go func() { results <- r.exchange(staticClientID, staticRedirect, code, verifier) }()
	time.Sleep(100 * time.Millisecond)
	go func() { results <- r.exchange(staticClientID, staticRedirect, code, verifier) }()
	a, b := <-results, <-results

	winner, loser := a, b
	if winner.Status != http.StatusOK {
		winner, loser = b, a
	}
	if winner.Status != http.StatusOK || loser.Status != http.StatusBadRequest || loser.str("error") != "invalid_grant" {
		t.Fatalf("結果 = %d %v / %d %v, want 1 つが 200、もう 1 つが 400 invalid_grant", a.Status, a.Body, b.Status, b.Body)
	}
	if _, err := r.srv.IntrospectAccessToken(r.ctx, winner.str("access_token")); !errors.Is(err, domain.ErrOAuthInvalidToken) {
		t.Errorf("再利用を検知したのに、先に交換した側のアクセストークンが使える (err = %v)", err)
	}
	if resp := r.refresh(staticClientID, winner.str("refresh_token")); resp.Status == http.StatusOK {
		t.Error("再利用を検知したのに、先に交換した側の更新トークンで新しいトークンが発行された")
	}
	if n := r.count(`SELECT count(*) FROM oauth_token_sessions WHERE kind = 'access_token'`); n != 0 {
		t.Errorf("アクセストークンの記録が %d 件残っている, want 0", n)
	}
	if n := r.count(`SELECT count(*) FROM oauth_token_sessions WHERE kind = 'refresh_token' AND active`); n != 0 {
		t.Errorf("有効な更新トークンの記録が %d 件残っている, want 0", n)
	}
}

// 更新トークンをほぼ同時に 2 回使ったとき、「再利用を検知して系列を全部取り消した」のに、先に入れ替えた側の
// 新しいトークンが有効なまま残る、という状態にならない。
func TestConcurrentRefreshLeavesNoLiveTokenAfterReuseDetection(t *testing.T) {
	r := newRig(t)
	_, refresh, _ := r.tokens(domain.OAuthScopeRead)
	slowTokenWrites(t, r, 600*time.Millisecond, 0)

	firstDone := make(chan tokenResponse, 1)
	go func() { firstDone <- r.refresh(staticClientID, refresh) }()
	time.Sleep(250 * time.Millisecond)
	second := r.refresh(staticClientID, refresh)
	first := <-firstDone

	if first.Status != http.StatusOK {
		t.Fatalf("1 つ目 = %d %v, want 200", first.Status, first.Body)
	}
	if second.Status == http.StatusOK {
		t.Fatalf("2 つ目にも新しいトークンが発行された: %v", second.Body)
	}
	_, introspectErr := r.srv.IntrospectAccessToken(r.ctx, first.str("access_token"))
	if second.str("error") == "invalid_grant" && introspectErr == nil {
		t.Error("再利用を検知して系列を取り消したのに、1 つ目の新しいアクセストークンが使える")
	}
	if second.str("error") != "invalid_grant" && introspectErr != nil {
		t.Errorf("再利用として取り消していないのに、1 つ目の新しいアクセストークンが使えない: %v", introspectErr)
	}
}

// 退会(論理削除)したユーザーには、更新・認可コードの交換のどちらでも、新しいトークンを発行しない。
func TestWithdrawnUserGetsNoNewTokens(t *testing.T) {
	withdraw := func(t *testing.T, r *rig) {
		t.Helper()
		if _, err := r.pool.Exec(r.ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, r.userID); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("退会したユーザーの更新トークンでは、新しいトークンを発行せず、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		withdraw(t, r)
		resp := r.refresh(staticClientID, refresh)
		if resp.Status != http.StatusBadRequest || resp.str("error") != "invalid_grant" || resp.str("access_token") != "" {
			t.Errorf("= %d %v, want 400 invalid_grant without tokens", resp.Status, resp.Body)
		}
	})

	t.Run("認可コードを発行したあとに退会したユーザーの認可コードでは、トークンを発行せず、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		withdraw(t, r)
		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		if resp.Status != http.StatusBadRequest || resp.str("error") != "invalid_grant" || resp.str("access_token") != "" {
			t.Errorf("= %d %v, want 400 invalid_grant without tokens", resp.Status, resp.Body)
		}
	})

	t.Run("退会していないユーザーの更新には、影響しない", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		if resp := r.refresh(staticClientID, refresh); resp.Status != http.StatusOK {
			t.Errorf("= %d %v, want 200", resp.Status, resp.Body)
		}
	})
}

// 更新のとき、scope を指定しなくても、元の許可の範囲が維持される(既定の範囲に縮まらない)。
func TestRefreshKeepsTheGrantedScopes(t *testing.T) {
	r := newRig(t)
	_, refresh, first := r.tokens(domain.OAuthScopeRead, domain.OAuthScopeWrite)
	if !strings.Contains(first.str("scope"), domain.OAuthScopeWrite) {
		t.Fatalf("最初のトークンの scope = %q", first.str("scope"))
	}
	resp := r.refresh(staticClientID, refresh)
	if resp.Status != http.StatusOK {
		t.Fatalf("更新 = %d %v", resp.Status, resp.Body)
	}
	token, err := r.srv.IntrospectAccessToken(context.Background(), resp.str("access_token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := token.Check(testResource, domain.OAuthScopeRead, domain.OAuthScopeWrite); err != nil {
		t.Errorf("更新後のトークンが、元の範囲(読み取りと書き込み)を持たない: %v (scope %q)", err, resp.str("scope"))
	}
}

type noHasher struct{}

func (noHasher) Hash(p string) (string, error) { return "digest:" + p, nil }
func (noHasher) Compare(d, p string) error     { return nil }

// 本人が退会(usecase.Users.Delete)すると、実際のデータベースで、その利用者の許可とトークンの記録がすべて消え、
// 発行済みのアクセストークンも更新トークンも使えなくなる。別の利用者の許可は残る。
func TestWithdrawalRevokesTheUsersGrantsAndTokens(t *testing.T) {
	r := newRig(t)
	access, refresh, _ := r.tokens(domain.OAuthScopeRead)
	bob := dbtest.InsertUserRow(r.ctx, t, r.conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'd') RETURNING id`)
	if _, err := r.grants.Approve(r.ctx, bob, domain.OAuthClient{ID: staticClientID, Name: "Dev App"}, []string{domain.OAuthScopeRead}); err != nil {
		t.Fatal(err)
	}

	userQuery := query.NewUserQuery(r.pool)
	users := usecase.NewUsers(userQuery, domain.NewUsers(repository.NewUserRepository(r.pool)), uow.New(r.pool),
		usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), usecase.NewShopStatsRecalculator(uowtest.Clock{}), noHasher{})
	if err := users.Delete(r.ctx, domain.User{ID: r.userID}, r.userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if n := r.count(`SELECT count(*) FROM oauth_grants WHERE user_id = $1`, r.userID); n != 0 {
		t.Errorf("退会した利用者の許可が %d 件残っている", n)
	}
	if n := r.count(`SELECT count(*) FROM oauth_token_sessions WHERE user_id = $1`, r.userID); n != 0 {
		t.Errorf("退会した利用者のトークンの記録が %d 件残っている", n)
	}
	if n := r.count(`SELECT count(*) FROM oauth_grants WHERE user_id = $1`, bob); n != 1 {
		t.Errorf("別の利用者の許可 = %d 件, want 1", n)
	}
	if _, err := r.srv.IntrospectAccessToken(r.ctx, access); !errors.Is(err, domain.ErrOAuthInvalidToken) {
		t.Errorf("退会後のアクセストークン err = %v, want ErrOAuthInvalidToken", err)
	}
	if resp := r.refresh(staticClientID, refresh); resp.Status != http.StatusBadRequest {
		t.Errorf("退会後の更新トークンで取り直せた: %d %v", resp.Status, resp.Body)
	}
}
