package domain

import (
	"context"
	"fmt"
	"time"
)

// OAuthGrant は、利用者が、あるアプリ(クライアント)に許可した範囲の記録である。利用者と
// アプリの組ごとに 1 つで、許可した範囲(Scopes)は、要求が広がって許可し直すたびに、広がる方向にだけ
// 更新される。記録を消す(取り消す)と、そのアプリに発行したトークンは、すべて使えなくなる。
type OAuthGrant struct {
	ID         string
	UserID     string
	ClientID   string
	ClientName string
	Scopes     []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Permits は、この許可の記録で、利用者 userID に、アプリ clientID への、範囲 scopes の認可コードを発行してよいかを
// 判断する。記録が別の利用者・別のアプリのものなら(wrap された)ErrOAuthGrantNotFound、許可済みの範囲に
// 収まらないなら(wrap された)ErrOAuthInvalidScope を返す。認可コードには、この判断を通した範囲だけを付与する。
func (g OAuthGrant) Permits(userID, clientID string, scopes []string) error {
	if g.UserID != userID || g.ClientID != clientID {
		return fmt.Errorf("%w: the grant belongs to another user or app", ErrOAuthGrantNotFound)
	}
	if OAuthConsentRequired(g.Scopes, scopes) {
		return fmt.Errorf("%w: the requested scope exceeds the granted scope", ErrOAuthInvalidScope)
	}
	return nil
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateOAuthGrantParams は、許可の記録として保存するフィールドを保持する。
type CreateOAuthGrantParams struct {
	UserID     string
	ClientID   string
	ClientName string
	Scopes     []string
}

// OAuthGrantRepository は、許可の記録の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード(書き込みオブジェクトの OAuthGrants)だけで、usecase は呼ばない。書き込み専用で、
// 読み取りのメソッドは置かない。
type OAuthGrantRepository interface {
	// CreateOAuthGrant は、利用者とアプリの組の許可を保存し、その id を返す。すでに記録があるときは、
	// 許可の範囲を、既存の範囲と params の範囲を合わせたものに更新し(狭めない)、アプリの表示名を
	// 最新にして、同じ id を返す。
	CreateOAuthGrant(ctx context.Context, params CreateOAuthGrantParams) (id string, err error)
	// DiscardOAuthGrant は、userID の grantID の許可の記録を削除する。その記録から発行された
	// すべてのトークン(認可コード・アクセストークン・更新トークン)も、同時に使えなくなる。
	// 記録がない、または別の利用者のものなら、(wrap された)ErrOAuthGrantNotFound を返す。
	DiscardOAuthGrant(ctx context.Context, userID, grantID string) error
	// DiscardOAuthGrantsByUser は、userID のすべての許可の記録を削除する(存在しなくてもエラーにしない)。
	// 発行されたすべてのトークンも、同時に使えなくなる。ユーザーが退会するときに使う。
	DiscardOAuthGrantsByUser(ctx context.Context, userID string) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// OAuthGrants は、許可の記録の集約の書き込みオブジェクトである。OAuthGrantRepository を持つのは
// この型だけである。
type OAuthGrants struct {
	repo OAuthGrantRepository
}

// NewOAuthGrants は repo を使う OAuthGrants を返す。
func NewOAuthGrants(repo OAuthGrantRepository) *OAuthGrants {
	return &OAuthGrants{repo: repo}
}

// Approve は、利用者が client に scopes を許可したことを記録し、許可の記録の id を返す。
// 範囲が空、または知らない範囲を含むときは、何も保存せず、(wrap された)ErrOAuthInvalidScope を返す。
// アプリの表示名は、規則(最大の文字数)で切り詰めて保存する。
func (g *OAuthGrants) Approve(ctx context.Context, userID string, client OAuthClient, scopes []string) (string, error) {
	if err := ValidateOAuthScopes(scopes); err != nil {
		return "", err
	}
	name := client.Name
	if r := []rune(name); len(r) > MaxOAuthClientNameLength {
		name = string(r[:MaxOAuthClientNameLength])
	}
	return g.repo.CreateOAuthGrant(ctx, CreateOAuthGrantParams{
		UserID:     userID,
		ClientID:   client.ID,
		ClientName: name,
		Scopes:     MergeOAuthScopes(nil, scopes),
	})
}

// Revoke は、userID の grantID の許可を取り消す。そのアプリのトークンは、すべて使えなくなる。
func (g *OAuthGrants) Revoke(ctx context.Context, userID, grantID string) error {
	return g.repo.DiscardOAuthGrant(ctx, userID, grantID)
}

// RevokeAll は、userID のすべての許可を取り消す。そのユーザーに発行されたトークンは、すべて使えなくなる。
// ユーザーが退会するときに、退会と同じトランザクションで呼ぶ。
func (g *OAuthGrants) RevokeAll(ctx context.Context, userID string) error {
	return g.repo.DiscardOAuthGrantsByUser(ctx, userID)
}
