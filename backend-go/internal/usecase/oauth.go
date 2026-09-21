package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// OAuthTokenSessionQuery は、発行したトークンの記録の、読み取り専用の窓口である。認可ライブラリの
// 保存先(adapter/oauthserver)が、トークンの検証や交換のときに、記録を読むために使う。
// 書き込みのメソッドは置かない(書き込みは domain.OAuthTokenSessions を通す)。
type OAuthTokenSessionQuery interface {
	// GetOAuthTokenSession は、kind と signature の記録を返す(無効になった記録も返す。再利用を検知するため)。
	// なければ、(wrap された)domain.ErrOAuthTokenSessionNotFound を返す。
	GetOAuthTokenSession(ctx context.Context, kind domain.OAuthTokenKind, signature string) (domain.OAuthTokenSession, error)
}

// OAuthTokenIntrospector は、アクセストークンの文字列を確かめる口である。署名・期限・保存された記録との
// 照合は、認可ライブラリが行い(adapter/oauthserver が実装する)、使えないトークンは、理由を区別せず
// (wrap された)domain.ErrOAuthInvalidToken を返す。
type OAuthTokenIntrospector interface {
	IntrospectAccessToken(ctx context.Context, rawToken string) (domain.OAuthAccessToken, error)
}

// OAuthAccessTokens は、OAuth のアクセストークンで API を呼ぶ要求を認証する use case である。
// 保護する側(たとえば `/mcp`)が、Authorization ヘッダーのトークンをここへ渡す。
// トークンの宛先が保護する側の URL であること、必要な範囲を許可されていること、持ち主のユーザーが
// まだ有効であることの判断は、ここ(と domain)にあり、HTTP の層には置かない。
type OAuthAccessTokens struct {
	introspector OAuthTokenIntrospector
	users        UserQuery
	resource     string
}

// NewOAuthAccessTokens は、宛先 resource(保護する側の URL)のためのトークンの認証を返す。
func NewOAuthAccessTokens(introspector OAuthTokenIntrospector, users UserQuery, resource string) *OAuthAccessTokens {
	return &OAuthAccessTokens{introspector: introspector, users: users, resource: resource}
}

// Authenticate は、rawToken を確かめ、持ち主のユーザーと、トークンの内容を返す。使えない(存在しない・
// 改ざん・期限切れ・取り消し済み・宛先違い・持ち主が退会済み)ときは domain.ErrOAuthInvalidToken、
// 有効だが required の範囲が足りないときは、足りない範囲を持つ *domain.InsufficientScopeError を返す。
func (a *OAuthAccessTokens) Authenticate(ctx context.Context, rawToken string, required ...string) (domain.User, domain.OAuthAccessToken, error) {
	token, err := a.introspector.IntrospectAccessToken(ctx, rawToken)
	if err != nil {
		if errors.Is(err, domain.ErrOAuthInvalidToken) {
			return domain.User{}, domain.OAuthAccessToken{}, domain.ErrOAuthInvalidToken
		}
		return domain.User{}, domain.OAuthAccessToken{}, fmt.Errorf("authenticate oauth access token: %w", err)
	}
	if err := token.Check(a.resource, required...); err != nil {
		return domain.User{}, domain.OAuthAccessToken{}, err
	}
	user, err := a.users.GetActiveUserByID(ctx, token.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.OAuthAccessToken{}, domain.ErrOAuthInvalidToken
		}
		return domain.User{}, domain.OAuthAccessToken{}, fmt.Errorf("authenticate oauth access token: load user: %w", err)
	}
	return user, token, nil
}
