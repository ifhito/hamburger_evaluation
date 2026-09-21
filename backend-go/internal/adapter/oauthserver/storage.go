package oauthserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"
	ftx "github.com/ory/fosite/storage"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// extraGrantID は、セッションの追加の値に、許可の記録(domain.OAuthGrant)の id を入れるキーである。
// トークンの記録を、許可に結び付けて保存する(許可を取り消すと、記録が連鎖して消える)ために使う。
const extraGrantID = "grant_id"

// discardExpiredLimit は、認可コードを発行するたびに、ついでに片付ける期限切れの記録の上限の件数である。
const discardExpiredLimit = 50

// errStorage は、保存先(データベース)の障害を表す。トークンが無効なのではなく、確かめられなかった
// ことを、呼び出し側(トークンの検証)が区別するために使う。
var errStorage = errors.New("oauth storage failure")

// storage は、認可ライブラリが要求する保存の契約を、domain の書き込みオブジェクトと usecase の読み取りの窓口で
// 実装する。トークンの文字列そのものは受け取らず、ライブラリが計算した署名(signature)だけを保存する。
type storage struct {
	sessions usecase.OAuthTokenSessionStore
	clients  *clientResolver
}

var (
	_ fosite.Storage                = (*storage)(nil)
	_ oauth2.CoreStorage            = (*storage)(nil)
	_ oauth2.TokenRevocationStorage = (*storage)(nil)
	_ pkce.PKCERequestStorage       = (*storage)(nil)
	_ ftx.Transactional             = (*storage)(nil)
)

// txKey は、ライブラリが開始したトランザクション(usecase.OAuthTokenSessionTx)を、context に入れるキーである。
type txKey struct{}

// scope は、ctx がトランザクションを持っていればそれに結び付いた、持っていなければトランザクションの外の、
// トークンの記録の書き込みと読み取りの組を返す。
func (s *storage) scope(ctx context.Context) usecase.OAuthTokenSessionScope {
	if tx, ok := ctx.Value(txKey{}).(usecase.OAuthTokenSessionTx); ok {
		return tx.OAuthTokenSessionScope
	}
	return s.sessions.Scope()
}

// BeginTX、Commit、Rollback は、ライブラリが、認可コードを使用済みにしてから、トークンを保存するまで
// (更新トークンの入れ替えから、新しいトークンを保存するまでも同じ)を、1 つのトランザクションにまとめるための口である。
// まとめないと、認可コードの再利用を検知した別の要求が、系列を取り消したあとに、先の交換が、まだ保存していなかった
// トークンを保存してしまい、取り消したはずのトークンが有効なまま残る。まとめれば、認可コードの行のロックで、
// 後から来た要求は、先の交換が確定するまで待たされ、そのあとで取り消すので、保存されたトークンも取り消される。
func (s *storage) BeginTX(ctx context.Context) (context.Context, error) {
	tx, err := s.sessions.Begin(ctx)
	if err != nil {
		return ctx, fmt.Errorf("%w: begin transaction: %w", errStorage, err)
	}
	return context.WithValue(ctx, txKey{}, tx), nil
}

func (s *storage) Commit(ctx context.Context) error {
	tx, ok := ctx.Value(txKey{}).(usecase.OAuthTokenSessionTx)
	if !ok {
		return fmt.Errorf("%w: commit without a transaction", errStorage)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: commit: %w", errStorage, err)
	}
	return nil
}

func (s *storage) Rollback(ctx context.Context) error {
	tx, ok := ctx.Value(txKey{}).(usecase.OAuthTokenSessionTx)
	if !ok {
		return nil
	}
	return tx.Rollback(ctx)
}

// storedRequest は、認可の内容を、ライブラリが復元できる形で保存するための JSON の形である。
type storedRequest struct {
	ID                string          `json:"id"`
	RequestedAt       time.Time       `json:"requested_at"`
	ClientID          string          `json:"client_id"`
	RequestedScope    []string        `json:"requested_scope"`
	GrantedScope      []string        `json:"granted_scope"`
	RequestedAudience []string        `json:"requested_audience"`
	GrantedAudience   []string        `json:"granted_audience"`
	Form              url.Values      `json:"form"`
	Session           json.RawMessage `json:"session"`
}

// identity は、認可の内容から、利用者の id と許可の記録の id を取り出す。認可コードを発行する側が
// セッションに入れなかったときは、実装の誤りなので、保存せずにエラーにする。
func identity(req fosite.Requester) (userID, grantID string, err error) {
	sess, ok := req.GetSession().(*fosite.DefaultSession)
	if !ok || sess == nil {
		return "", "", errors.New("oauth session has an unexpected type")
	}
	userID = sess.GetSubject()
	grantID, _ = sess.Extra[extraGrantID].(string)
	if userID == "" || grantID == "" {
		return "", "", errors.New("oauth session has no user or grant")
	}
	return userID, grantID, nil
}

func (s *storage) save(ctx context.Context, kind domain.OAuthTokenKind, signature string, req fosite.Requester, expiryKey fosite.TokenType, fallbackTTL time.Duration) error {
	userID, grantID, err := identity(req)
	if err != nil {
		return err
	}
	sess := req.GetSession()
	rawSession, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("encode oauth session: %w", err)
	}
	payload, err := json.Marshal(storedRequest{
		ID:                req.GetID(),
		RequestedAt:       req.GetRequestedAt(),
		ClientID:          req.GetClient().GetID(),
		RequestedScope:    req.GetRequestedScopes(),
		GrantedScope:      req.GetGrantedScopes(),
		RequestedAudience: req.GetRequestedAudience(),
		GrantedAudience:   req.GetGrantedAudience(),
		Form:              req.GetRequestForm(),
		Session:           rawSession,
	})
	if err != nil {
		return fmt.Errorf("encode oauth request: %w", err)
	}
	expires := sess.GetExpiresAt(expiryKey)
	if expires.IsZero() {
		expires = time.Now().Add(fallbackTTL)
	}
	if err := s.scope(ctx).Writes.Save(ctx, domain.OAuthTokenSession{
		Kind:      kind,
		Signature: signature,
		RequestID: req.GetID(),
		GrantID:   grantID,
		UserID:    userID,
		ClientID:  req.GetClient().GetID(),
		ExpiresAt: expires,
		Request:   payload,
	}); err != nil {
		return fmt.Errorf("%w: %w", errStorage, err)
	}
	return nil
}

// load は、記録を読んで、認可の内容を復元する。session には、保存されていたセッションを読み込む。
// 復元するアプリは、識別子と、固定の使い方(newFositeClient)だけを持つ。戻り先などの登録情報は、
// トークンの検証や交換には要らないので、取り直さない(アプリの説明の文書を取りに行かない)。
func (s *storage) load(ctx context.Context, kind domain.OAuthTokenKind, signature string, session fosite.Session) (fosite.Requester, domain.OAuthTokenSession, error) {
	rec, err := s.scope(ctx).Reads.GetOAuthTokenSession(ctx, kind, signature)
	if err != nil {
		if errors.Is(err, domain.ErrOAuthTokenSessionNotFound) {
			return nil, rec, fosite.ErrNotFound
		}
		return nil, rec, fmt.Errorf("%w: %w", errStorage, err)
	}
	var p storedRequest
	if err := json.Unmarshal(rec.Request, &p); err != nil {
		return nil, rec, fmt.Errorf("%w: decode oauth request: %w", errStorage, err)
	}
	if session != nil {
		if err := json.Unmarshal(p.Session, session); err != nil {
			return nil, rec, fmt.Errorf("%w: decode oauth session: %w", errStorage, err)
		}
	}
	return &fosite.Request{
		ID:                p.ID,
		RequestedAt:       p.RequestedAt,
		Client:            newFositeClient(p.ClientID, nil, s.clients.resource),
		RequestedScope:    p.RequestedScope,
		GrantedScope:      p.GrantedScope,
		RequestedAudience: p.RequestedAudience,
		GrantedAudience:   p.GrantedAudience,
		Form:              p.Form,
		Session:           session,
	}, rec, nil
}

// ---- 認可コード ----

// CreateAuthorizeCodeSession は、認可コードの記録を保存する。ついでに、期限切れの記録を少し片付ける
// (専用の掃除の仕組みを持たずに、記録が増え続けないようにする)。
func (s *storage) CreateAuthorizeCodeSession(ctx context.Context, signature string, req fosite.Requester) error {
	if err := s.save(ctx, domain.OAuthTokenAuthorizationCode, signature, req, fosite.AuthorizeCode, domain.OAuthAuthorizationCodeTTL); err != nil {
		return err
	}
	if _, err := s.scope(ctx).Writes.DiscardExpired(ctx, discardExpiredLimit); err != nil {
		slog.Warn("oauth: discard expired token sessions failed", "error", err)
	}
	return nil
}

// GetAuthorizeCodeSession は、認可コードの記録を返す。使用済みなら、記録と一緒に
// fosite.ErrInvalidatedAuthorizeCode を返す(ライブラリが、その認可で発行済みのトークンを取り消すため)。
func (s *storage) GetAuthorizeCodeSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	req, rec, err := s.load(ctx, domain.OAuthTokenAuthorizationCode, signature, session)
	if err != nil {
		return nil, err
	}
	if !rec.Active {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}
	return req, nil
}

// InvalidateAuthorizeCodeSession は、認可コードを使用済みにする。並行して 2 つの要求が同じコードを使おうと
// しても、成功するのは 1 つだけである。
func (s *storage) InvalidateAuthorizeCodeSession(ctx context.Context, signature string) error {
	err := s.scope(ctx).Writes.Use(ctx, domain.OAuthTokenAuthorizationCode, signature)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrOAuthTokenSessionInactive):
		return fosite.ErrInvalidatedAuthorizeCode
	case errors.Is(err, domain.ErrOAuthTokenSessionNotFound):
		return fosite.ErrNotFound
	}
	return fmt.Errorf("%w: %w", errStorage, err)
}

// ---- PKCE ----

func (s *storage) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.save(ctx, domain.OAuthTokenPKCE, signature, req, fosite.AuthorizeCode, domain.OAuthAuthorizationCodeTTL)
}

func (s *storage) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	req, _, err := s.load(ctx, domain.OAuthTokenPKCE, signature, session)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (s *storage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	return s.forget(ctx, domain.OAuthTokenPKCE, signature)
}

// ---- アクセストークン ----

func (s *storage) CreateAccessTokenSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.save(ctx, domain.OAuthTokenAccess, signature, req, fosite.AccessToken, domain.OAuthAccessTokenTTL)
}

func (s *storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	req, rec, err := s.load(ctx, domain.OAuthTokenAccess, signature, session)
	if err != nil {
		return nil, err
	}
	if !rec.Active {
		return req, fosite.ErrInactiveToken
	}
	return req, nil
}

func (s *storage) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	return s.forget(ctx, domain.OAuthTokenAccess, signature)
}

// ---- 更新トークン ----

// CreateRefreshTokenSession は、更新トークンの記録を保存する。対になるアクセストークンの署名は、
// 記録には使わない(同じ認可の系列の id で結び付いている)。
func (s *storage) CreateRefreshTokenSession(ctx context.Context, signature, _ string, req fosite.Requester) error {
	return s.save(ctx, domain.OAuthTokenRefresh, signature, req, fosite.RefreshToken, domain.OAuthRefreshTokenTTL)
}

// GetRefreshTokenSession は、更新トークンの記録を返す。入れ替え済みなら、記録と一緒に
// fosite.ErrInactiveToken を返す(ライブラリが、再利用として、その系列を取り消すため)。
func (s *storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	req, rec, err := s.load(ctx, domain.OAuthTokenRefresh, signature, session)
	if err != nil {
		return nil, err
	}
	if !rec.Active {
		return req, fosite.ErrInactiveToken
	}
	return req, nil
}

func (s *storage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	return s.forget(ctx, domain.OAuthTokenRefresh, signature)
}

// RotateRefreshToken は、更新トークンを入れ替え済みにし、同じ系列の古いアクセストークンを使えなくする。
// 並行して同じ更新トークンを使おうとしても、成功するのは 1 つだけである。
func (s *storage) RotateRefreshToken(ctx context.Context, requestID, signature string) error {
	err := s.scope(ctx).Writes.Rotate(ctx, requestID, signature)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrOAuthTokenSessionInactive):
		return fosite.ErrInactiveToken
	case errors.Is(err, domain.ErrOAuthTokenSessionNotFound):
		return fosite.ErrNotFound
	}
	return fmt.Errorf("%w: %w", errStorage, err)
}

// ---- 取り消し ----

// RevokeRefreshToken と RevokeAccessToken は、ライブラリが、再利用の検知や取り消しの要求で、系列の
// トークンを全部使えなくするために呼ぶ。domain の RevokeRequest が、アクセストークンの削除と
// 更新トークンの無効化を、どちらも行う(2 回呼ばれても結果は同じ)。
func (s *storage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	return s.revokeRequest(ctx, requestID)
}

func (s *storage) RevokeAccessToken(ctx context.Context, requestID string) error {
	return s.revokeRequest(ctx, requestID)
}

func (s *storage) revokeRequest(ctx context.Context, requestID string) error {
	if err := s.scope(ctx).Writes.RevokeRequest(ctx, requestID); err != nil {
		return fmt.Errorf("%w: %w", errStorage, err)
	}
	return nil
}

func (s *storage) forget(ctx context.Context, kind domain.OAuthTokenKind, signature string) error {
	if err := s.scope(ctx).Writes.Forget(ctx, kind, signature); err != nil {
		return fmt.Errorf("%w: %w", errStorage, err)
	}
	return nil
}
