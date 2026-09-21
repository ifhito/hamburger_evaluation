package oauthserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// minSecretBytes は、トークンの署名に使う秘密の鍵の最小のバイト数である(ライブラリの要求)。
const minSecretBytes = 32

// Config は、認可サーバーの設定である。
type Config struct {
	// Issuer は、この認可サーバーの URL(発行者)である。トークンの発行や認可の URL は、この下にある。
	// 末尾の "/" は取り除かれる。
	Issuer string
	// Resource は、発行するトークンの宛先(このトークンを使えるサーバーの URL。たとえば `/mcp` の URL)である。
	Resource string
	// ConsentURL は、利用者に許可を尋ねる画面(SPA)の URL である。
	ConsentURL string
	// Secret は、トークンの署名に使う秘密の鍵で、32 バイト以上でなければならない。値は決してログに出さない。
	Secret []byte
	// StaticClients は、固定で登録するアプリである(自分で説明を公開できないアプリや、ローカルでの確認用)。
	StaticClients []domain.OAuthClient
}

// Server は、OAuth の認可サーバーである。認可・トークン・取り消しの HTTP の窓口と、トークンの検証、
// 利用者が許可した認可要求への認可コードの発行を提供する。
type Server struct {
	cfg      Config
	provider fosite.OAuth2Provider
	store    *storage
	grants   usecase.OAuthGrantQuery
	users    usecase.UserQuery
}

// Deps は、認可サーバーが使う保存先と読み取りの窓口である。
type Deps struct {
	// Sessions は、発行したトークンの記録の保存先である。
	Sessions usecase.OAuthTokenSessionStore
	// Grants は、利用者が許可したアプリの記録の読み取りである(認可コードを発行するとき、許可の記録を確かめる)。
	Grants usecase.OAuthGrantQuery
	// Users は、トークンの持ち主が有効(退会していない)かを確かめるための読み取りである。
	Users usecase.UserQuery
	// Fetcher は、アプリが公開している説明の文書(CIMD)の取得役である。
	Fetcher MetadataFetcher
}

// New は、認可サーバーを組み立てる。
func New(cfg Config, deps Deps) (*Server, error) {
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	for name, raw := range map[string]string{"issuer": cfg.Issuer, "resource": cfg.Resource, "consent URL": cfg.ConsentURL} {
		if u, err := url.Parse(raw); err != nil || !u.IsAbs() || u.Host == "" || u.Fragment != "" {
			return nil, fmt.Errorf("oauth %s must be an absolute URL without a fragment", name)
		}
	}
	if len(cfg.Secret) < minSecretBytes {
		return nil, fmt.Errorf("oauth secret must be at least %d bytes", minSecretBytes)
	}
	clients, err := newClientResolver(cfg.StaticClients, deps.Fetcher, cfg.Resource, time.Now)
	if err != nil {
		return nil, err
	}
	store := &storage{sessions: deps.Sessions, clients: clients}
	fcfg := &fosite.Config{
		AccessTokenLifespan:            domain.OAuthAccessTokenTTL,
		RefreshTokenLifespan:           domain.OAuthRefreshTokenTTL,
		AuthorizeCodeLifespan:          domain.OAuthAuthorizationCodeTTL,
		GlobalSecret:                   cfg.Secret,
		ScopeStrategy:                  fosite.ExactScopeStrategy,
		AudienceMatchingStrategy:       fosite.DefaultAudienceMatchingStrategy,
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		// 空の一覧は「更新トークンに特別な範囲を要求しない」を表す(nil は、既定の offline 系の範囲を要求する)。
		RefreshTokenScopes: []string{},
		RedirectSecureChecker: func(_ context.Context, u *url.URL) bool {
			return domain.ValidateOAuthRedirectURI(u.String()) == nil
		},
		TokenURL: cfg.Issuer + "/oauth/token",
	}
	provider := compose.Compose(fcfg, store, compose.NewOAuth2HMACStrategy(fcfg),
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2TokenRevocationFactory,
		compose.OAuth2PKCEFactory,
	)
	return &Server{cfg: cfg, provider: provider, store: store, grants: deps.Grants, users: deps.Users}, nil
}

// ---- 認可の要求 ----

// normalizeParams は、認可・トークンの要求の値を、ライブラリに渡す形にする。
//   - 宛先(resource)を、ライブラリが扱う audience にする。指定がなければ、この認可サーバーの宛先を使う。
//     指定が違うときは、宛先の誤りを返し(ライブラリには正しい宛先を渡して要求の解釈を続けさせる。
//     アプリに戻せる形でエラーを返すため)、利用者が直接 audience を指定しても無視する。
//   - 範囲(scope)がなければ、既定の範囲(読むだけ)にする。ただし、これは認可の要求(authorize が true)だけで、
//     トークンの要求(認可コードの交換・更新)には補わない。更新のとき scope を省くのは「元の許可の範囲を保つ」
//     という意味なので、既定の範囲で置き換えてはならない。
func (s *Server) normalizeParams(params url.Values, authorize bool) (url.Values, error) {
	q := url.Values{}
	for k, v := range params {
		q[k] = append([]string(nil), v...)
	}
	resource, err := domain.ResolveOAuthResource(q["resource"], s.cfg.Resource)
	if err != nil {
		resource = s.cfg.Resource
		err = errInvalidTarget
	}
	q.Del("resource")
	q.Set("audience", resource)
	if authorize && len(strings.Fields(q.Get("scope"))) == 0 {
		q.Set("scope", strings.Join(domain.DefaultOAuthScopes(), " "))
	}
	return q, err
}

// errInvalidTarget は、宛先(resource)の誤りである。RFC 8707 が定める invalid_target で返す。
var errInvalidTarget = &fosite.RFC6749Error{
	ErrorField:       "invalid_target",
	DescriptionField: "The requested resource is not supported by this authorization server.",
	CodeField:        http.StatusBadRequest,
}

// parseAuthorize は、認可の要求(認可の URL の値)を、ライブラリで解釈して検証する。アプリが未登録・
// 戻り先が登録と違う・PKCE がない・範囲や宛先の誤りなどは、エラーで返る。エラーのときも、アプリへ
// 戻せる要求かどうかを判断するために、要求の値(nil でないことがある)を一緒に返す。
func (s *Server) parseAuthorize(ctx context.Context, params url.Values) (fosite.AuthorizeRequester, error) {
	q, targetErr := s.normalizeParams(params, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build authorize request: %w", err)
	}
	ar, err := s.provider.NewAuthorizeRequest(ctx, req)
	if err == nil {
		// ライブラリも PKCE を必須にしているが、認可コードを作る手前で確認するので、コードの記録を残さず、
		// 許可を尋ねる前に(認可の URL の時点で)断れる。
		if perr := domain.ValidateOAuthPKCE(ar.GetRequestForm().Get("code_challenge"), ar.GetRequestForm().Get("code_challenge_method")); perr != nil {
			err = fosite.ErrInvalidRequest.WithHint(perr.Error())
		} else {
			err = targetErr
		}
	}
	return ar, err
}

// HandleAuthorize は、認可の URL(GET /oauth/authorize)の窓口である。要求を検証し、問題がなければ、
// 利用者に許可を尋ねる画面(SPA)へ、同じ値のまま渡す。アプリへ結果を戻せない不正は、その場でエラーを返し
// (戻り先が不確かなので、リダイレクトしない)、戻せる不正は、アプリへエラーを付けて戻す。
func (s *Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ar, err := s.parseAuthorize(ctx, r.URL.Query())
	if err != nil {
		s.writeAuthorizeError(ctx, w, ar, err)
		return
	}
	target := s.cfg.ConsentURL
	if r.URL.RawQuery != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + r.URL.RawQuery
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) writeAuthorizeError(ctx context.Context, w http.ResponseWriter, ar fosite.AuthorizeRequester, err error) {
	if ar == nil {
		ar = fosite.NewAuthorizeRequest()
	}
	rec := newCapture()
	s.provider.WriteAuthorizeError(ctx, rec, ar, err)
	if loc := rec.header.Get("Location"); loc != "" {
		rec.header.Set("Location", s.withIssuer(loc))
	}
	rec.copyTo(w)
}

// withIssuer は、アプリへの戻り先の URL に、発行者(iss)を付ける。アプリは、結果がどの認可サーバーから
// 来たかを確かめられる(別の認可サーバーになりすました結果を取り違えない。RFC 9207)。
func (s *Server) withIssuer(location string) string {
	u, err := url.Parse(location)
	if err != nil {
		return location
	}
	q := u.Query()
	q.Set("iss", s.cfg.Issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

// AuthorizeRequestView は、利用者に許可を尋ねる画面に出す、認可の要求の内容である。
type AuthorizeRequestView struct {
	ClientID   string
	ClientName string
	Scopes     []string
}

// DescribeAuthorizeRequest は、認可の要求を検証して、画面に出す内容を返す。アプリへ結果を戻せない不正は、
// (wrap された)domain.ErrOAuthAuthorizeRequestInvalid を返す。戻せる不正(範囲の誤りなど)は、
// 許可を尋ねても意味がないので、同じエラーで返す。
func (s *Server) DescribeAuthorizeRequest(ctx context.Context, params url.Values) (AuthorizeRequestView, error) {
	ar, err := s.parseAuthorize(ctx, params)
	if err != nil {
		return AuthorizeRequestView{}, invalidRequest(err)
	}
	name := ar.GetClient().GetID()
	if c, cerr := s.store.clients.resolve(ctx, name); cerr == nil {
		name = c.Name
	}
	return AuthorizeRequestView{ClientID: ar.GetClient().GetID(), ClientName: name, Scopes: ar.GetRequestedScopes()}, nil
}

// IssueAuthorizationCode は、利用者(userID)が許可した(許可の記録が grantID の)認可の要求に、認可コードを
// 発行し、アプリへ戻す URL(認可コードと発行者を付けたもの)を返す。要求が不正なときは、アプリへ戻せるなら
// エラーを付けた戻り先を返し、戻せないなら(wrap された)domain.ErrOAuthAuthorizeRequestInvalid を返す。
func (s *Server) IssueAuthorizationCode(ctx context.Context, params url.Values, userID, grantID string) (string, error) {
	ar, err := s.parseAuthorize(ctx, params)
	if err != nil {
		return s.errorRedirect(ctx, ar, err)
	}
	// 許可の記録が、この利用者・このアプリのもので、求められた範囲を許可済みであることを、保存先から確かめる。
	// 確かめた範囲だけを付与する(呼び出し側が、別の許可の記録の id や、許可していない範囲を渡しても、
	// 認可コードは発行しない)。
	if err := s.checkGrant(ctx, ar, userID, grantID); err != nil {
		return "", err
	}
	for _, sc := range ar.GetRequestedScopes() {
		ar.GrantScope(sc)
	}
	for _, aud := range ar.GetRequestedAudience() {
		ar.GrantAudience(aud)
	}
	session := &fosite.DefaultSession{Subject: userID, Extra: map[string]interface{}{extraGrantID: grantID}}
	resp, err := s.provider.NewAuthorizeResponse(ctx, ar, session)
	if err != nil {
		return s.errorRedirect(ctx, ar, err)
	}
	rec := newCapture()
	s.provider.WriteAuthorizeResponse(ctx, rec, ar, resp)
	loc := rec.header.Get("Location")
	if loc == "" {
		return "", errors.New("oauth: authorize response has no redirect location")
	}
	return s.withIssuer(loc), nil
}

// checkGrant は、grantID の許可の記録が、userID とアプリのもので、要求された範囲を含むことを確かめる。
// 記録が別の利用者・別のアプリのもの、または存在しないなら(wrap された)domain.ErrOAuthGrantNotFound、
// 許可済みの範囲を超えるなら(wrap された)domain.ErrOAuthInvalidScope を返す。
func (s *Server) checkGrant(ctx context.Context, ar fosite.AuthorizeRequester, userID, grantID string) error {
	clientID := ar.GetClient().GetID()
	grant, err := s.grants.GetOAuthGrantByUserAndClient(ctx, userID, clientID)
	if err != nil {
		if errors.Is(err, domain.ErrOAuthGrantNotFound) {
			return err
		}
		return fmt.Errorf("issue authorization code: load grant: %w", err)
	}
	if grant.ID != grantID {
		return fmt.Errorf("%w: the grant is not the one for this user and app", domain.ErrOAuthGrantNotFound)
	}
	return grant.Permits(userID, clientID, ar.GetRequestedScopes())
}

// DenyAuthorization は、利用者が許可しなかった認可の要求に対して、アプリへ戻す URL(access_denied を付けたもの)を返す。
func (s *Server) DenyAuthorization(ctx context.Context, params url.Values) (string, error) {
	ar, err := s.parseAuthorize(ctx, params)
	if err != nil {
		return s.errorRedirect(ctx, ar, err)
	}
	return s.errorRedirect(ctx, ar, fosite.ErrAccessDenied)
}

// errorRedirect は、認可の失敗を、アプリへ戻す URL にする。戻せない要求(戻り先が確かでない)は、
// リダイレクトせずに、(wrap された)domain.ErrOAuthAuthorizeRequestInvalid を返す。
func (s *Server) errorRedirect(ctx context.Context, ar fosite.AuthorizeRequester, err error) (string, error) {
	if ar == nil || !ar.IsRedirectURIValid() {
		return "", invalidRequest(err)
	}
	rec := newCapture()
	s.provider.WriteAuthorizeError(ctx, rec, ar, err)
	loc := rec.header.Get("Location")
	if loc == "" {
		return "", invalidRequest(err)
	}
	return s.withIssuer(loc), nil
}

func invalidRequest(err error) error {
	rfc := fosite.ErrorToRFC6749Error(err)
	return fmt.Errorf("%w: %s", domain.ErrOAuthAuthorizeRequestInvalid, strings.TrimSpace(rfc.GetDescription()))
}

// ---- トークン・取り消し・検証 ----

// HandleToken は、トークンの URL(POST /oauth/token)の窓口である。認可コード(PKCE つき)と更新トークンを、
// アクセストークン(と入れ替えた更新トークン)に交換する。
func (s *Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.provider.WriteAccessError(ctx, w, nil, fosite.ErrInvalidRequest.WithHint("Unable to parse the request body."))
		return
	}
	q, targetErr := s.normalizeParams(r.PostForm, false)
	if targetErr != nil {
		s.provider.WriteAccessError(ctx, w, nil, targetErr)
		return
	}
	r.PostForm = q
	r.Form = q
	ar, err := s.provider.NewAccessRequest(ctx, r, &fosite.DefaultSession{})
	if err != nil {
		s.logServerError(err)
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	// 持ち主(認可コードを発行した利用者、更新トークンを発行された利用者)が、いまも有効(退会していない)かを、
	// トークンを発行する前に確かめる。退会した利用者には、認可コードの交換でも更新でも、新しいトークンを出さない。
	if err := s.ensureOwnerActive(ctx, ar); err != nil {
		s.logServerError(err)
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	resp, err := s.provider.NewAccessResponse(ctx, ar)
	if err != nil {
		if errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) {
			// 同じ認可コードが、ほぼ同時に 2 回使われた(先に使った要求が、コードを使用済みにした)。
			// 再利用なので、その認可から発行されたトークンを、すべて使えなくする。
			if rerr := s.store.revokeRequest(ctx, ar.GetID()); rerr != nil {
				s.logServerError(rerr)
			}
			err = fosite.ErrInvalidGrant.WithHint("The authorization code has already been used.")
		}
		s.logServerError(err)
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAccessResponse(ctx, w, ar, resp)
}

// ensureOwnerActive は、ar のトークンの持ち主が、いまも有効な利用者であることを確かめる。退会していれば
// invalid_grant(認可コードや更新トークンが、もう使えない)を返す。確かめられなかった(保存先の障害)ときは、
// サーバーの障害として返す。
func (s *Server) ensureOwnerActive(ctx context.Context, ar fosite.AccessRequester) error {
	subject := ar.GetSession().GetSubject()
	if subject == "" {
		return fosite.ErrInvalidGrant.WithHint("The grant has no resource owner.")
	}
	if _, err := s.users.GetActiveUserByID(ctx, subject); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return fosite.ErrInvalidGrant.WithHint("The resource owner is no longer available.")
		}
		return fosite.ErrServerError.WithWrap(fmt.Errorf("%w: load resource owner: %w", errStorage, err)).WithDebug(err.Error())
	}
	return nil
}

// HandleRevoke は、取り消しの URL(POST /oauth/revoke。RFC 7009)の窓口である。更新トークンを取り消すと、
// その認可から発行されたトークンが、すべて使えなくなる。
func (s *Server) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	err := s.provider.NewRevocationRequest(r.Context(), r)
	s.logServerError(err)
	s.provider.WriteRevocationResponse(r.Context(), w, err)
}

// IntrospectAccessToken は、アクセストークンの文字列を確かめ、その内容を返す。署名・期限・保存された記録との
// 照合はライブラリが行う。使えないトークンは、理由を区別せず domain.ErrOAuthInvalidToken を返し、
// 保存先の障害で確かめられなかったときだけ、それ以外のエラーを返す。
func (s *Server) IntrospectAccessToken(ctx context.Context, rawToken string) (domain.OAuthAccessToken, error) {
	session := &fosite.DefaultSession{}
	use, ar, err := s.provider.IntrospectToken(ctx, rawToken, fosite.AccessToken, session)
	if err != nil {
		if errors.Is(err, errStorage) {
			return domain.OAuthAccessToken{}, fmt.Errorf("introspect oauth access token: %w", err)
		}
		return domain.OAuthAccessToken{}, domain.ErrOAuthInvalidToken
	}
	if use != fosite.AccessToken {
		return domain.OAuthAccessToken{}, domain.ErrOAuthInvalidToken
	}
	return domain.OAuthAccessToken{
		UserID:    ar.GetSession().GetSubject(),
		ClientID:  ar.GetClient().GetID(),
		Scopes:    ar.GetGrantedScopes(),
		Audience:  ar.GetGrantedAudience(),
		ExpiresAt: ar.GetSession().GetExpiresAt(fosite.AccessToken),
	}, nil
}

var _ usecase.OAuthTokenIntrospector = (*Server)(nil)

// logServerError は、保存先の障害を、運用の調査のためにログに残す(トークン・コード・鍵は含まれない)。
func (s *Server) logServerError(err error) {
	if err != nil && errors.Is(err, errStorage) {
		slog.Error("oauth: storage failure", "error", err)
	}
}

// ---- 認可サーバーの情報 ----

// Metadata は、認可サーバーの情報(RFC 8414)である。アプリは、この URL から、認可・トークンの窓口と、
// 対応する方式を知る。
func (s *Server) Metadata() map[string]any {
	scopes := make([]string, 0, len(domain.OAuthScopes()))
	for _, sc := range domain.OAuthScopes() {
		scopes = append(scopes, sc.Name)
	}
	return map[string]any{
		"issuer":                                s.cfg.Issuer,
		"authorization_endpoint":                s.cfg.Issuer + "/oauth/authorize",
		"token_endpoint":                        s.cfg.Issuer + "/oauth/token",
		"revocation_endpoint":                   s.cfg.Issuer + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      scopes,
		// アプリが、自分の説明を URL で公開する方式(client_id にその URL を使う)に対応する。
		"client_id_metadata_document_supported": true,
		// 認可の結果の戻り先に、発行者(iss)を付ける。
		"authorization_response_iss_parameter_supported": true,
	}
}

// HandleMetadata は、認可サーバーの情報(GET /.well-known/oauth-authorization-server)の窓口である。
func (s *Server) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(s.Metadata())
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(body)
}

// ---- 応答の取り込み ----

// capture は、ライブラリが書く応答(ヘッダーと本文)を、いったん受け取るための http.ResponseWriter である。
// アプリへの戻り先(Location)を、URL として取り出して加工するために使う。
type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCapture() *capture { return &capture{header: http.Header{}, status: http.StatusOK} }

func (c *capture) Header() http.Header         { return c.header }
func (c *capture) WriteHeader(status int)      { c.status = status }
func (c *capture) Write(b []byte) (int, error) { return c.body.Write(b) }

func (c *capture) copyTo(w http.ResponseWriter) {
	for k, v := range c.header {
		w.Header()[k] = v
	}
	w.WriteHeader(c.status)
	_, _ = w.Write(c.body.Bytes())
}
