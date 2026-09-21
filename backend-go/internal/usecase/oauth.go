package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

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
	// 判定の順番: 宛先 → 持ち主が有効か → 範囲。持ち主が退会済みのトークンは、範囲に関係なく、常に無効
	// (401 相当)にする。範囲の判定を先にすると、退会済みの持ち主の読み取りだけのトークンが、書き込みを
	// 求めたときに、無効ではなく範囲の不足(403 相当)になってしまう。
	if err := token.Check(a.resource); err != nil {
		return domain.User{}, domain.OAuthAccessToken{}, err
	}
	user, err := a.users.GetActiveUserByID(ctx, token.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.OAuthAccessToken{}, domain.ErrOAuthInvalidToken
		}
		return domain.User{}, domain.OAuthAccessToken{}, fmt.Errorf("authenticate oauth access token: load user: %w", err)
	}
	if err := token.Check(a.resource, required...); err != nil {
		return domain.User{}, domain.OAuthAccessToken{}, err
	}
	return user, token, nil
}

// OAuthGrantQuery は、利用者が許可したアプリの記録の、読み取り専用の窓口である。
// 書き込みのメソッドは置かない(書き込みは domain.OAuthGrants を通す)。
type OAuthGrantQuery interface {
	// GetOAuthGrantByUserAndClient は、利用者とアプリの組の許可を返す。なければ、(wrap された)
	// domain.ErrOAuthGrantNotFound を返す。
	GetOAuthGrantByUserAndClient(ctx context.Context, userID, clientID string) (domain.OAuthGrant, error)
	// ListOAuthGrantsByUser は、利用者が許可したアプリを、最近使ったものから順に、limit 件まで返す(offset 件を
	// 飛ばす)。2 つ目の戻り値は、続き(次のページ)があるかである。
	ListOAuthGrantsByUser(ctx context.Context, userID string, limit, offset int32) ([]domain.OAuthGrant, bool, error)
}

// AuthorizeRequestView は、利用者に許可を尋ねる画面に出す、認可の要求の内容である。
type AuthorizeRequestView struct {
	ClientID   string
	ClientName string
	// Scopes は、アプリが求めた範囲の名前である。
	Scopes []string
}

// OAuthAuthorizer は、認可の要求の解釈と、結果のアプリへの戻し方を、認可ライブラリに任せる口である
// (adapter/oauthserver が実装する)。params は、認可の URL の値(client_id・redirect_uri・scope・state・
// code_challenge など)で、要求は、呼ぶたびに検証し直す。アプリへ結果を戻せない不正は、(wrap された)
// domain.ErrOAuthAuthorizeRequestInvalid を返し、戻せる不正は、エラーを付けた戻り先を返す。
type OAuthAuthorizer interface {
	// DescribeAuthorizeRequest は、要求を検証して、画面に出す内容を返す。
	DescribeAuthorizeRequest(ctx context.Context, params url.Values) (AuthorizeRequestView, error)
	// IssueAuthorizationCode は、利用者(userID)が許可した(許可の記録が grantID の)要求に、認可コードを発行し、
	// アプリへ戻す URL を返す。
	IssueAuthorizationCode(ctx context.Context, params url.Values, userID, grantID string) (redirectTo string, err error)
	// DenyAuthorization は、利用者が許可しなかった要求に対して、アプリへ戻す URL(access_denied)を返す。
	DenyAuthorization(ctx context.Context, params url.Values) (redirectTo string, err error)
}

// ConsentView は、許可の画面に出す内容である。範囲の説明は、frontend に写さず、ここ(domain の規則)から返す。
type ConsentView struct {
	ClientID   string
	ClientName string
	Scopes     []domain.OAuthScope
	// ConsentRequired が false なら、求められた範囲は、すでに許可済みの範囲に収まる。画面は、尋ねずに、
	// そのまま許可を送ってよい。
	ConsentRequired bool
}

// OAuthConsents は、許可の画面(利用者が、アプリに何を許すかを決める)の use case である。
type OAuthConsents struct {
	authorizer OAuthAuthorizer
	grants     OAuthGrantQuery
	writes     *domain.OAuthGrants
}

// NewOAuthConsents は、許可の画面の use case を返す。
func NewOAuthConsents(authorizer OAuthAuthorizer, grants OAuthGrantQuery, writes *domain.OAuthGrants) *OAuthConsents {
	return &OAuthConsents{authorizer: authorizer, grants: grants, writes: writes}
}

// ParseAuthorizeQuery は、frontend が渡す、認可の URL の値(URL の `?` のあとの文字列)を、値の組にする。
// 先頭の "?" は取り除く。解釈できない文字列は、(wrap された)domain.ErrOAuthAuthorizeRequestInvalid を返す。
func ParseAuthorizeQuery(raw string) (url.Values, error) {
	params, err := url.ParseQuery(strings.TrimPrefix(raw, "?"))
	if err != nil {
		return nil, fmt.Errorf("%w: the query string is malformed", domain.ErrOAuthAuthorizeRequestInvalid)
	}
	return params, nil
}

// Describe は、認可の要求を検証し、許可の画面に出す内容を返す。この利用者が、すでにこのアプリに、
// 求められた範囲を全部許可していれば、ConsentRequired は false になる(範囲が広がったときだけ尋ね直す)。
func (c *OAuthConsents) Describe(ctx context.Context, userID string, params url.Values) (ConsentView, error) {
	view, err := c.authorizer.DescribeAuthorizeRequest(ctx, params)
	if err != nil {
		return ConsentView{}, err
	}
	var granted []string
	existing, err := c.grants.GetOAuthGrantByUserAndClient(ctx, userID, view.ClientID)
	switch {
	case err == nil:
		granted = existing.Scopes
	case errors.Is(err, domain.ErrOAuthGrantNotFound):
	default:
		return ConsentView{}, fmt.Errorf("describe oauth consent: %w", err)
	}
	scopes := make([]domain.OAuthScope, 0, len(view.Scopes))
	for _, name := range view.Scopes {
		if s, ok := domain.OAuthScopeByName(name); ok {
			scopes = append(scopes, s)
		}
	}
	return ConsentView{
		ClientID:        view.ClientID,
		ClientName:      view.ClientName,
		Scopes:          scopes,
		ConsentRequired: domain.OAuthConsentRequired(granted, view.Scopes),
	}, nil
}

// Decide は、利用者の許可・拒否を受けて、アプリへ戻す URL を返す。許可なら、許可の記録を残してから、
// 認可コードを発行する(記録を残せなかったときは、コードを発行しない)。拒否なら、何も記録せず、
// access_denied を付けて戻す。要求は、ここでも検証し直すので、画面が見た内容と違う値を送っても、
// 検証を通らなければ、許可は残らない。
func (c *OAuthConsents) Decide(ctx context.Context, userID string, params url.Values, approve bool) (string, error) {
	if !approve {
		return c.authorizer.DenyAuthorization(ctx, params)
	}
	view, err := c.authorizer.DescribeAuthorizeRequest(ctx, params)
	if err != nil {
		return "", err
	}
	grantID, err := c.writes.Approve(ctx, userID, domain.OAuthClient{ID: view.ClientID, Name: view.ClientName}, view.Scopes)
	if err != nil {
		return "", fmt.Errorf("decide oauth consent: %w", err)
	}
	return c.authorizer.IssueAuthorizationCode(ctx, params, userID, grantID)
}

// ConnectedApps は、利用者が許可したアプリの一覧と、取り消しの use case である。
type ConnectedApps struct {
	grants OAuthGrantQuery
	writes *domain.OAuthGrants
}

// NewConnectedApps は、許可したアプリの一覧と取り消しの use case を返す。
func NewConnectedApps(grants OAuthGrantQuery, writes *domain.OAuthGrants) *ConnectedApps {
	return &ConnectedApps{grants: grants, writes: writes}
}

// List は、利用者が許可したアプリを、最近使ったものから順に、ページ送りで返す。1 ページの件数の規則は、
// 既存の一覧(ショップ・レビュー)と同じ domain.PageBounds で、範囲外の page / perPage は、エラーにせず補正される
// (page < 1 は 1、perPage < 1 は 20、perPage の上限は 100)。2 つ目の戻り値は、次のページがあるかである。
func (a *ConnectedApps) List(ctx context.Context, userID string, page, perPage int) ([]domain.OAuthGrant, bool, error) {
	limit, offset := domain.PageBounds(page, perPage)
	grants, hasMore, err := a.grants.ListOAuthGrantsByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list connected apps: %w", err)
	}
	return grants, hasMore, nil
}

// Revoke は、利用者本人の許可を取り消す。そのアプリのトークンは、すぐに使えなくなる。
// 形式が正規でない id・存在しない許可・別の利用者の許可は、区別せず、(wrap された)
// domain.ErrOAuthGrantNotFound を返す。
func (a *ConnectedApps) Revoke(ctx context.Context, userID, grantID string) error {
	if !domain.IsUUID(grantID) {
		return fmt.Errorf("revoke connected app: %w", domain.ErrOAuthGrantNotFound)
	}
	return a.writes.Revoke(ctx, userID, grantID)
}

// OAuthTokenSessionScope は、トークンの記録の書き込みと読み取りを 1 組にしたものである。組の 2 つは、同じ
// 接続(または同じトランザクション)に結び付いている。
type OAuthTokenSessionScope struct {
	Writes *domain.OAuthTokenSessions
	Reads  OAuthTokenSessionQuery
}

// OAuthTokenSessionTx は、1 つのトランザクションに結び付いた OAuthTokenSessionScope と、その確定・取り消しである。
type OAuthTokenSessionTx struct {
	OAuthTokenSessionScope
	Commit   func(ctx context.Context) error
	Rollback func(ctx context.Context) error
}

// OAuthTokenSessionStore は、トークンの記録の保存先である。Scope はトランザクションの外(1 回ごとに確定する)、
// Begin は 1 つのトランザクションの中で、書き込みと読み取りを行う。認可ライブラリは、認可コードを使用済みに
// してからトークンを保存するまでを、1 つのトランザクションにまとめて欲しい(途中に別の要求が割り込んで、
// 取り消したはずのトークンが、あとから保存されないようにするため)。その手順は、ライブラリが決める順番なので、
// 1 つの関数にまとめられず、開始・確定・取り消しに分かれた形で渡す。
type OAuthTokenSessionStore interface {
	Scope() OAuthTokenSessionScope
	Begin(ctx context.Context) (OAuthTokenSessionTx, error)
}
