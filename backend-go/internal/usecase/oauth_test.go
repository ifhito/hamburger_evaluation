package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeIntrospector は、トークンの文字列に対する、決めた内容を返すテスト用の確認役である。
type fakeIntrospector struct {
	token domain.OAuthAccessToken
	err   error
}

func (f fakeIntrospector) IntrospectAccessToken(context.Context, string) (domain.OAuthAccessToken, error) {
	return f.token, f.err
}

func TestOAuthAccessTokensAuthenticate(t *testing.T) {
	const resource = "https://api.example.com/mcp"
	alice := domain.User{ID: "u-alice", Username: "alice"}
	valid := domain.OAuthAccessToken{
		UserID: alice.ID, ClientID: "app", Scopes: []string{domain.OAuthScopeRead}, Audience: []string{resource},
	}
	users := &fakeUserQuery{getByID: func(_ context.Context, id string) (domain.User, error) {
		if id == alice.ID {
			return alice, nil
		}
		return domain.User{}, domain.ErrUserNotFound
	}}
	storageDown := errors.New("database is down")

	tests := []struct {
		name       string
		introspect fakeIntrospector
		users      usecase.UserQuery
		required   []string
		wantUser   bool
		wantErr    error
	}{
		{"有効なトークンと、必要な範囲を満たす操作なら、持ち主のユーザーを返す", fakeIntrospector{token: valid}, users, []string{domain.OAuthScopeRead}, true, nil},
		{"必要な範囲を指定しなければ、宛先とユーザーだけを確かめて通す", fakeIntrospector{token: valid}, users, nil, true, nil},
		{"使えないトークンは、無効なトークンとして断る", fakeIntrospector{err: domain.ErrOAuthInvalidToken}, users, nil, false, domain.ErrOAuthInvalidToken},
		{"別のサーバー宛てのトークンは、無効なトークンとして断る", fakeIntrospector{token: domain.OAuthAccessToken{UserID: alice.ID, Scopes: valid.Scopes, Audience: []string{"https://other.example.com/mcp"}}}, users, nil, false, domain.ErrOAuthInvalidToken},
		{"書き込みの範囲がないトークンで書き込みを求めると、範囲の不足として断る", fakeIntrospector{token: valid}, users, []string{domain.OAuthScopeWrite}, false, domain.ErrOAuthInsufficientScope},
		{"持ち主が退会している(見つからない)トークンは、無効なトークンとして断る", fakeIntrospector{token: domain.OAuthAccessToken{UserID: "u-gone", Scopes: valid.Scopes, Audience: valid.Audience}}, users, nil, false, domain.ErrOAuthInvalidToken},
		{"トークンの確認が保存先の障害で失敗したときは、無効なトークンではなく、障害として返す", fakeIntrospector{err: storageDown}, users, nil, false, storageDown},
		{"持ち主の検索が障害で失敗したときは、無効なトークンではなく、障害として返す", fakeIntrospector{token: valid}, &fakeUserQuery{getByID: func(context.Context, string) (domain.User, error) { return domain.User{}, storageDown }}, nil, false, storageDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := usecase.NewOAuthAccessTokens(tt.introspect, tt.users, resource)
			user, token, err := auth.Authenticate(context.Background(), "raw", tt.required...)
			if tt.wantErr == nil {
				if err != nil || user != alice || token.ClientID != "app" {
					t.Fatalf("Authenticate = (%+v, %+v, %v)", user, token, err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == storageDown && errors.Is(err, domain.ErrOAuthInvalidToken) {
				t.Error("障害が、無効なトークンとして扱われた")
			}
			if user.ID != "" {
				t.Errorf("エラーなのにユーザーが返った: %+v", user)
			}
		})
	}
}
