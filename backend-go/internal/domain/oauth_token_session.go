package domain

import (
	"context"
	"time"
)

// OAuthTokenKind は、保存するトークンの記録の種類である。
type OAuthTokenKind string

const (
	// OAuthTokenAuthorizationCode は、認可コード(アプリが許可の証と交換するための引換券。1 回だけ使える)である。
	OAuthTokenAuthorizationCode OAuthTokenKind = "authorization_code"
	// OAuthTokenAccess は、アクセストークン(API を呼ぶための許可の証)である。
	OAuthTokenAccess OAuthTokenKind = "access_token"
	// OAuthTokenRefresh は、更新トークン(アクセストークンを取り直すための証。使うたびに入れ替わる)である。
	OAuthTokenRefresh OAuthTokenKind = "refresh_token"
	// OAuthTokenPKCE は、認可コードに結び付けた PKCE の情報である(認可コードの横取りを防ぐ確認に使う)。
	OAuthTokenPKCE OAuthTokenKind = "pkce"
)

// OAuthTokenSession は、発行したトークンの記録である。トークンの文字列そのものは保存せず、
// 照合に使う署名(Signature)だけを保存する(記録が漏れても、トークンは作れない)。
// 同じ認可から発行された認可コード・アクセストークン・更新トークンは、同じ RequestID を持つ
// (「系列」。再利用を検知したとき、系列ごと無効にする)。
type OAuthTokenSession struct {
	Kind      OAuthTokenKind
	Signature string
	RequestID string
	// GrantID は、この記録の元になった許可の記録である。許可を取り消すと、この記録も消える。
	GrantID  string
	UserID   string
	ClientID string
	// Active が false なら、使用済みまたは入れ替え済みで、もう使えない(再利用の検知のために、記録は残す)。
	Active    bool
	ExpiresAt time.Time
	// Request は、認可ライブラリが、認可の内容を復元するために使う保存形式である(domain は中身を解釈しない)。
	Request []byte
}

// ---- repository の契約(実装は adapter/repository) ----

// OAuthTokenSessionRepository は、トークンの記録の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード(書き込みオブジェクトの OAuthTokenSessions)だけで、usecase は呼ばない。書き込み専用で、
// 読み取りのメソッドは置かない。
type OAuthTokenSessionRepository interface {
	// CreateOAuthTokenSession は、記録を有効な状態で保存する。
	CreateOAuthTokenSession(ctx context.Context, session OAuthTokenSession) error
	// UpdateOAuthTokenSessionInactive は、kind と signature の記録を、使用済み(無効)にする。判定と更新は
	// 1 つの文で行うので、同じ記録を並行して使おうとしても、成功するのは 1 つだけである。すでに無効なら
	// (wrap された)ErrOAuthTokenSessionInactive、記録がなければ ErrOAuthTokenSessionNotFound を返す。
	UpdateOAuthTokenSessionInactive(ctx context.Context, kind OAuthTokenKind, signature string) error
	// UpdateOAuthRefreshRotated は、requestID の系列の、signature の更新トークンを入れ替え済み(無効)にし、
	// 同じ系列のアクセストークンを削除する。判定と更新は 1 つの文で行い、すでに無効・存在しないなら、
	// UpdateOAuthTokenSessionInactive と同じエラーを返す。
	UpdateOAuthRefreshRotated(ctx context.Context, requestID, signature string) error
	// UpdateOAuthRequestRevoked は、requestID の系列を取り消す: アクセストークンを削除し、更新トークンを(再利用の
	// 検知のために記録を残して)無効にする。1 つの操作で行うので、失敗したときに、片方だけが反映されることはない。
	UpdateOAuthRequestRevoked(ctx context.Context, requestID string) error
	// DiscardOAuthTokenSession は、kind と signature の記録を削除する。なくても、エラーにしない。
	DiscardOAuthTokenSession(ctx context.Context, kind OAuthTokenKind, signature string) error
	// DiscardExpiredOAuthTokenSessions は、期限切れの記録を、最大 limit 件まで削除し、削除した件数を返す。
	DiscardExpiredOAuthTokenSessions(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// OAuthTokenSessions は、トークンの記録の集約の書き込みオブジェクトである。OAuthTokenSessionRepository を
// 持つのはこの型だけである。
type OAuthTokenSessions struct {
	repo OAuthTokenSessionRepository
}

// NewOAuthTokenSessions は repo を使う OAuthTokenSessions を返す。
func NewOAuthTokenSessions(repo OAuthTokenSessionRepository) *OAuthTokenSessions {
	return &OAuthTokenSessions{repo: repo}
}

// Save は、発行したトークンの記録を保存する。
func (s *OAuthTokenSessions) Save(ctx context.Context, session OAuthTokenSession) error {
	session.Active = true
	return s.repo.CreateOAuthTokenSession(ctx, session)
}

// Use は、1 回だけ使える記録(認可コード)を使用済みにする。並行して使おうとしても、成功するのは
// 1 つだけで、残りは ErrOAuthTokenSessionInactive を返す。
func (s *OAuthTokenSessions) Use(ctx context.Context, kind OAuthTokenKind, signature string) error {
	return s.repo.UpdateOAuthTokenSessionInactive(ctx, kind, signature)
}

// Rotate は、更新トークンを入れ替え済みにし、同じ系列の古いアクセストークンを使えなくする。
// 古い更新トークンの記録は、再利用を検知するために残る。
func (s *OAuthTokenSessions) Rotate(ctx context.Context, requestID, signature string) error {
	return s.repo.UpdateOAuthRefreshRotated(ctx, requestID, signature)
}

// Forget は、kind と signature の記録を削除する(PKCE の情報や、使い終えたトークンの片付けに使う)。
func (s *OAuthTokenSessions) Forget(ctx context.Context, kind OAuthTokenKind, signature string) error {
	return s.repo.DiscardOAuthTokenSession(ctx, kind, signature)
}

// RevokeRequest は、requestID の系列のトークンをすべて使えなくする。アクセストークンは削除し、
// 更新トークンは(再利用の検知のために記録を残して)無効にする。この 2 つは、1 つの操作(すべて成功するか、
// 何も変わらないか)で行う。認可コードの再利用や、入れ替え済みの更新トークンの再利用を検知したときに使う。
func (s *OAuthTokenSessions) RevokeRequest(ctx context.Context, requestID string) error {
	return s.repo.UpdateOAuthRequestRevoked(ctx, requestID)
}

// DiscardExpired は、期限切れの記録を最大 limit 件まで削除する。
func (s *OAuthTokenSessions) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredOAuthTokenSessions(ctx, limit)
}
