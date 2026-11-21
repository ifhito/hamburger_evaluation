package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// DefaultUserSessionTTL は、発行元が有効期間を返さない代役(単体テストなど)のときに使う既定のセッション有効期間である。
// 本番および JWT を使う統合テストでは、JWT の有効期間と同じ値が渡される。
const DefaultUserSessionTTL = 24 * time.Hour

// ErrUserSessionNotFound は、セッショントークンに対応する有効な記録が存在しない(未登録・ログアウト済み・
// 期限切れ・持ち主が退会済み)ことを表す。
var ErrUserSessionNotFound = errors.New("user session is invalid or has expired")

// HashSessionToken は、生の認証トークンを保存用の SHA-256(64 文字の 16 進文字列)に変換する。
// トークンそのものは DB に保存しない。
func HashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateUserSessionParams は、セッションとして保存する項目を保持する。平文トークンではなく TokenHash(SHA-256)を渡す。
type CreateUserSessionParams struct {
	UserID    string
	TokenHash string
	TTL       time.Duration
}

// UserSessionRepository は、セッショントークンの記録の書き込み契約である。domain が宣言し、
// 呼び出すのは domain の書き込みオブジェクト(UserSessions)だけで、usecase は直接参照しない。
type UserSessionRepository interface {
	// CreateUserSession は新しいセッションの記録を保存する。
	CreateUserSession(ctx context.Context, params CreateUserSessionParams) error
	// DiscardUserSessionByTokenHash は、指定したトークンハッシュのセッションを削除して失効させる。
	DiscardUserSessionByTokenHash(ctx context.Context, tokenHash string) error
	// DiscardUserSessionsByUser は、指定した利用者のすべてのセッションを削除して失効させる。
	DiscardUserSessionsByUser(ctx context.Context, userID string) error
	// DiscardExpiredUserSessions は、期限切れのセッションを最大 limit 件まで削除し、削除件数を返す。
	DiscardExpiredUserSessions(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// UserSessions は、セッショントークンの記録の書き込みオブジェクトである。平文トークンから保存用ハッシュへの
// 変換をここで行い、平文を repository へ渡さない。
type UserSessions struct {
	repo UserSessionRepository
}

// NewUserSessions は repo を使う UserSessions を返す。
func NewUserSessions(repo UserSessionRepository) *UserSessions {
	return &UserSessions{repo: repo}
}

// Create は、発行した生トークンのハッシュを利用者 ID と有効期間に結び付けて保存する。
func (s *UserSessions) Create(ctx context.Context, userID, rawToken string, ttl time.Duration) error {
	if userID == "" || rawToken == "" {
		return fmt.Errorf("create user session: user id and token are required")
	}
	if ttl < 0 {
		return fmt.Errorf("create user session: ttl must not be negative")
	}
	if ttl == 0 {
		ttl = DefaultUserSessionTTL
	}
	return s.repo.CreateUserSession(ctx, CreateUserSessionParams{
		UserID:    userID,
		TokenHash: HashSessionToken(rawToken),
		TTL:       ttl,
	})
}

// DiscardByToken は、生トークンに対応するセッションの記録を削除して即座に失効させる。
func (s *UserSessions) DiscardByToken(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.repo.DiscardUserSessionByTokenHash(ctx, HashSessionToken(rawToken))
}

// DiscardAllByUser は、利用者のすべてのセッションを削除する(退会時に同じトランザクションで呼ぶ)。
func (s *UserSessions) DiscardAllByUser(ctx context.Context, userID string) error {
	return s.repo.DiscardUserSessionsByUser(ctx, userID)
}

// DiscardExpired は、期限切れのセッションを最大 limit 件まで削除する。
func (s *UserSessions) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredUserSessions(ctx, limit)
}
