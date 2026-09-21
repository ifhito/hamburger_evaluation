package usecase_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeAuthorizer は、認可ライブラリの代わりに、決めた結果を返し、呼ばれた内容を記録するテスト用の実装である。
type fakeAuthorizer struct {
	view      usecase.AuthorizeRequestView
	describe  error
	issueErr  error
	issued    []string // "<userID>/<grantID>"
	denied    int
	describes int
}

func (f *fakeAuthorizer) DescribeAuthorizeRequest(context.Context, url.Values) (usecase.AuthorizeRequestView, error) {
	f.describes++
	return f.view, f.describe
}

func (f *fakeAuthorizer) IssueAuthorizationCode(_ context.Context, _ url.Values, userID, grantID string) (string, error) {
	if f.issueErr != nil {
		return "", f.issueErr
	}
	f.issued = append(f.issued, userID+"/"+grantID)
	return "https://app.example.com/cb?code=abc", nil
}

func (f *fakeAuthorizer) DenyAuthorization(context.Context, url.Values) (string, error) {
	f.denied++
	return "https://app.example.com/cb?error=access_denied", nil
}

// fakeGrantStore は、許可の記録の読み取り(usecase.OAuthGrantQuery)と書き込み(domain.OAuthGrantRepository)を
// 1 つのメモリ上の表で満たすテスト用の実装である。
type fakeGrantStore struct {
	grants    []domain.OAuthGrant
	queryErr  error
	createErr error
	created   []domain.CreateOAuthGrantParams
	revoked   [][2]string
	revokeErr error
}

func (f *fakeGrantStore) GetOAuthGrantByUserAndClient(_ context.Context, userID, clientID string) (domain.OAuthGrant, error) {
	if f.queryErr != nil {
		return domain.OAuthGrant{}, f.queryErr
	}
	for _, g := range f.grants {
		if g.UserID == userID && g.ClientID == clientID {
			return g, nil
		}
	}
	return domain.OAuthGrant{}, domain.ErrOAuthGrantNotFound
}

func (f *fakeGrantStore) ListOAuthGrantsByUser(_ context.Context, userID string) ([]domain.OAuthGrant, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	var out []domain.OAuthGrant
	for _, g := range f.grants {
		if g.UserID == userID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeGrantStore) CreateOAuthGrant(_ context.Context, p domain.CreateOAuthGrantParams) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, p)
	return "grant-new", nil
}

func (f *fakeGrantStore) DiscardOAuthGrant(_ context.Context, userID, grantID string) error {
	f.revoked = append(f.revoked, [2]string{userID, grantID})
	return f.revokeErr
}

func newConsents(a *fakeAuthorizer, s *fakeGrantStore) *usecase.OAuthConsents {
	return usecase.NewOAuthConsents(a, s, domain.NewOAuthGrants(s))
}

var appView = usecase.AuthorizeRequestView{ClientID: "app", ClientName: "アプリ", Scopes: []string{domain.OAuthScopeRead}}

func TestOAuthConsentsDescribe(t *testing.T) {
	ctx := context.Background()

	t.Run("初めてのアプリには、アプリの名前と、求められた範囲の説明をつけて、尋ねる必要があると返す", func(t *testing.T) {
		view, err := newConsents(&fakeAuthorizer{view: appView}, &fakeGrantStore{}).Describe(ctx, "u1", url.Values{})
		if err != nil {
			t.Fatal(err)
		}
		want := usecase.ConsentView{
			ClientID: "app", ClientName: "アプリ", ConsentRequired: true,
			Scopes: []domain.OAuthScope{{Name: domain.OAuthScopeRead, Description: "ショップ・レビュー・プロフィールを読む"}},
		}
		if !reflect.DeepEqual(view, want) {
			t.Errorf("view = %+v, want %+v", view, want)
		}
	})

	t.Run("すでに求められた範囲を許可済みなら、尋ねる必要がないと返す", func(t *testing.T) {
		store := &fakeGrantStore{grants: []domain.OAuthGrant{{UserID: "u1", ClientID: "app", Scopes: []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}}}}
		view, err := newConsents(&fakeAuthorizer{view: appView}, store).Describe(ctx, "u1", url.Values{})
		if err != nil || view.ConsentRequired {
			t.Errorf("view = %+v, err = %v, want no consent required", view, err)
		}
	})

	t.Run("範囲が広がった要求は、許可済みの範囲を超えるので、尋ねる必要があると返す", func(t *testing.T) {
		wider := appView
		wider.Scopes = []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}
		store := &fakeGrantStore{grants: []domain.OAuthGrant{{UserID: "u1", ClientID: "app", Scopes: []string{domain.OAuthScopeRead}}}}
		view, err := newConsents(&fakeAuthorizer{view: wider}, store).Describe(ctx, "u1", url.Values{})
		if err != nil || !view.ConsentRequired || len(view.Scopes) != 2 {
			t.Errorf("view = %+v, err = %v, want consent required with 2 scopes", view, err)
		}
	})

	t.Run("別の利用者が許可した記録は、この利用者の許可としては数えない", func(t *testing.T) {
		store := &fakeGrantStore{grants: []domain.OAuthGrant{{UserID: "someone-else", ClientID: "app", Scopes: []string{domain.OAuthScopeRead}}}}
		view, err := newConsents(&fakeAuthorizer{view: appView}, store).Describe(ctx, "u1", url.Values{})
		if err != nil || !view.ConsentRequired {
			t.Errorf("view = %+v, err = %v", view, err)
		}
	})

	t.Run("認可の要求が不正なときは、そのエラーをそのまま返し、許可の記録を読みに行かない", func(t *testing.T) {
		bad := fmtInvalid()
		store := &fakeGrantStore{queryErr: errors.New("must not be called")}
		_, err := newConsents(&fakeAuthorizer{describe: bad}, store).Describe(ctx, "u1", url.Values{})
		if !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})

	t.Run("許可の記録の読み取りが障害で失敗したときは、エラーを返す", func(t *testing.T) {
		store := &fakeGrantStore{queryErr: errors.New("database is down")}
		if _, err := newConsents(&fakeAuthorizer{view: appView}, store).Describe(ctx, "u1", url.Values{}); err == nil {
			t.Error("エラーにならなかった")
		}
	})
}

func fmtInvalid() error {
	return errors.Join(domain.ErrOAuthAuthorizeRequestInvalid, errors.New("bad redirect_uri"))
}

func TestOAuthConsentsDecide(t *testing.T) {
	ctx := context.Background()

	t.Run("許可すると、許可の記録を残してから、その記録の id で認可コードを発行し、アプリへ戻す URL を返す", func(t *testing.T) {
		auth, store := &fakeAuthorizer{view: appView}, &fakeGrantStore{}
		redirect, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, true)
		if err != nil || redirect != "https://app.example.com/cb?code=abc" {
			t.Fatalf("Decide = %q, %v", redirect, err)
		}
		if len(store.created) != 1 || store.created[0].UserID != "u1" || store.created[0].ClientID != "app" ||
			!reflect.DeepEqual(store.created[0].Scopes, []string{domain.OAuthScopeRead}) {
			t.Errorf("saved = %+v", store.created)
		}
		if !reflect.DeepEqual(auth.issued, []string{"u1/grant-new"}) {
			t.Errorf("issued = %v, want [u1/grant-new]", auth.issued)
		}
	})

	t.Run("拒否すると、許可を記録せず、拒否をアプリへ戻す URL を返す", func(t *testing.T) {
		auth, store := &fakeAuthorizer{view: appView}, &fakeGrantStore{}
		redirect, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, false)
		if err != nil || redirect != "https://app.example.com/cb?error=access_denied" {
			t.Fatalf("Decide = %q, %v", redirect, err)
		}
		if len(store.created) != 0 || len(auth.issued) != 0 || auth.denied != 1 {
			t.Errorf("created = %d, issued = %d, denied = %d", len(store.created), len(auth.issued), auth.denied)
		}
	})

	t.Run("認可の要求が不正なときは、許可を記録せず、認可コードも発行しない", func(t *testing.T) {
		auth, store := &fakeAuthorizer{describe: fmtInvalid()}, &fakeGrantStore{}
		_, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, true)
		if !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) || len(store.created) != 0 || len(auth.issued) != 0 {
			t.Errorf("err = %v, created = %d, issued = %d", err, len(store.created), len(auth.issued))
		}
	})

	t.Run("許可の記録を残せなかったときは、認可コードを発行しない", func(t *testing.T) {
		auth, store := &fakeAuthorizer{view: appView}, &fakeGrantStore{createErr: errors.New("database is down")}
		if _, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, true); err == nil || len(auth.issued) != 0 {
			t.Errorf("err = %v, issued = %d, want an error and no code", err, len(auth.issued))
		}
	})

	t.Run("認可コードの発行に失敗したときは、そのエラーを返す", func(t *testing.T) {
		issueErr := errors.New("cannot issue")
		auth, store := &fakeAuthorizer{view: appView, issueErr: issueErr}, &fakeGrantStore{}
		if _, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, true); !errors.Is(err, issueErr) {
			t.Errorf("err = %v, want %v", err, issueErr)
		}
	})

	t.Run("知らない範囲を含む要求を、許可として記録しない(範囲は domain の規則で確かめる)", func(t *testing.T) {
		bad := appView
		bad.Scopes = []string{"admin"}
		auth, store := &fakeAuthorizer{view: bad}, &fakeGrantStore{}
		_, err := newConsents(auth, store).Decide(ctx, "u1", url.Values{}, true)
		if !errors.Is(err, domain.ErrOAuthInvalidScope) || len(store.created) != 0 || len(auth.issued) != 0 {
			t.Errorf("err = %v, created = %d, issued = %d", err, len(store.created), len(auth.issued))
		}
	})
}

func TestParseAuthorizeQuery(t *testing.T) {
	t.Run("先頭の ? があってもなくても、値の組に分けられる", func(t *testing.T) {
		for _, raw := range []string{"?client_id=app&state=x%20y", "client_id=app&state=x%20y"} {
			p, err := usecase.ParseAuthorizeQuery(raw)
			if err != nil || p.Get("client_id") != "app" || p.Get("state") != "x y" {
				t.Errorf("ParseAuthorizeQuery(%q) = %v, %v", raw, p, err)
			}
		}
	})
	t.Run("解釈できない文字列は、認可の要求の不正として断る", func(t *testing.T) {
		if _, err := usecase.ParseAuthorizeQuery("a=%zz"); !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})
}

func TestConnectedApps(t *testing.T) {
	ctx := context.Background()
	const grantID = "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10"

	t.Run("許可したアプリの一覧は、その利用者の分だけを返す", func(t *testing.T) {
		store := &fakeGrantStore{grants: []domain.OAuthGrant{{ID: "g1", UserID: "u1"}, {ID: "g2", UserID: "u2"}}}
		got, err := usecase.NewConnectedApps(store, domain.NewOAuthGrants(store)).List(ctx, "u1")
		if err != nil || len(got) != 1 || got[0].ID != "g1" {
			t.Errorf("List = %+v, %v", got, err)
		}
	})

	t.Run("一覧の読み取りが失敗したときは、エラーを返す", func(t *testing.T) {
		store := &fakeGrantStore{queryErr: errors.New("database is down")}
		if _, err := usecase.NewConnectedApps(store, domain.NewOAuthGrants(store)).List(ctx, "u1"); err == nil {
			t.Error("エラーにならなかった")
		}
	})

	t.Run("許可を取り消すと、本人の利用者 id と許可の id で、取り消しを依頼する", func(t *testing.T) {
		store := &fakeGrantStore{}
		if err := usecase.NewConnectedApps(store, domain.NewOAuthGrants(store)).Revoke(ctx, "u1", grantID); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(store.revoked, [][2]string{{"u1", grantID}}) {
			t.Errorf("revoked = %v", store.revoked)
		}
	})

	t.Run("正規の形でない許可の id は、保存先へ渡さず、見つからないエラーにする", func(t *testing.T) {
		store := &fakeGrantStore{}
		err := usecase.NewConnectedApps(store, domain.NewOAuthGrants(store)).Revoke(ctx, "u1", "not-a-uuid")
		if !errors.Is(err, domain.ErrOAuthGrantNotFound) || len(store.revoked) != 0 {
			t.Errorf("err = %v, revoked = %v", err, store.revoked)
		}
	})

	t.Run("見つからない許可の取り消しは、見つからないエラーを返す", func(t *testing.T) {
		store := &fakeGrantStore{revokeErr: domain.ErrOAuthGrantNotFound}
		if err := usecase.NewConnectedApps(store, domain.NewOAuthGrants(store)).Revoke(ctx, "u1", grantID); !errors.Is(err, domain.ErrOAuthGrantNotFound) {
			t.Errorf("err = %v, want ErrOAuthGrantNotFound", err)
		}
	})
}
