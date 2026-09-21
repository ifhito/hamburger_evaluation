package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// SignupResendInterval は、同じ email への確認メールを送る最小の間隔である。
// この間隔より短い再 signup は、応答は同じでも、確認待ちの行を変えず、メールも送らない
// （第三者の email に確認メールを大量に送らせる悪用を抑えるため）。
const SignupResendInterval = 60 * time.Second

// signupTokenBytes は、確認トークンの乱数のバイト数である（256 ビット）。
const signupTokenBytes = 32

// SignupToken は、signup の確認トークンである。Raw は確認メールのリンクにだけ入れる
// 平文で、保存しない。Hash は DB に保存する SHA-256（16 進）で、確認のときに、
// リンクから受け取った Raw を同じ関数（HashSignupToken）で変換して照合する。
type SignupToken struct {
	Raw  string
	Hash string
}

// NewSignupToken は、暗号乱数（crypto/rand）の 32 バイトを base64url にした確認トークンを作る。
func NewSignupToken() (SignupToken, error) {
	buf := make([]byte, signupTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return SignupToken{}, fmt.Errorf("generate signup token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return SignupToken{Raw: raw, Hash: HashSignupToken(raw)}, nil
}

// HashSignupToken は、平文の確認トークンを、保存する形（SHA-256 の 16 進）に変換する。
// トークンは 256 ビットの乱数なので、パスワードと違い、遅いハッシュは要らない。
func HashSignupToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateSignupVerificationParams は、確認待ちの signup として保存するフィールドを
// 保持する。パスワードはハッシュ化済みで、トークンは Hash（平文は渡さない）である。
// TTL は、確認トークンの有効期間である。
type CreateSignupVerificationParams struct {
	Email          string
	Username       string
	PasswordDigest string
	TokenHash      string
	TTL            time.Duration
}

// SignupVerificationRepository は、確認待ちの signup の書き込みの契約である。domain が
// 宣言し、呼び出すのは domain のコード（書き込みオブジェクトの SignupVerifications）だけで、
// usecase は呼ばない。書き込み専用で、読み取りのメソッドは置かない。
type SignupVerificationRepository interface {
	// CreateSignupVerification は、params の email（大文字小文字を区別しない）の確認待ちを、
	// 最新の入力で置き換えて保存する（なければ作る）。前回の送信から SignupResendInterval
	// 以内なら何も変えず、accepted=false を返す（メールを送らない合図）。判定と書き込みは
	// 1 つの文で行うので、並行しても間隔は破られない。
	CreateSignupVerification(ctx context.Context, params CreateSignupVerificationParams) (accepted bool, err error)
	// CreateUserFromSignupVerification は、tokenHash の確認待ちをロックし、その内容で
	// users を作成し、確認待ちを削除する。すべて 1 つの transaction で行う。期限切れ・存在しない・
	// 使用済みの確認待ちは（wrap された）ErrSignupTokenInvalid を返し、確認までの間に同じ email の
	// users が作られていたときは（wrap された）ErrEmailTaken を返す（どちらの場合も何も書かない）。
	CreateUserFromSignupVerification(ctx context.Context, tokenHash string) (User, error)
	// DiscardExpiredSignupVerifications は、期限切れの確認待ちを、最大 limit 件まで削除し、
	// 削除した件数を返す。
	DiscardExpiredSignupVerifications(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// SignupVerifications は、確認待ちの signup 集約の書き込みオブジェクトである。
// SignupVerificationRepository を持つのはこの型だけで、usecase は repository に依存しない。
// 確認（CreateUserFromSignupVerification）は、確認待ちと users の 2 つのテーブルを 1 つの
// transaction で書くが、domain のコードとしては、確認待ちの集約の 1 つの書き込みである
// （transaction を usecase に持ち上げるのは S17 で扱う）。
type SignupVerifications struct {
	repo SignupVerificationRepository
}

// NewSignupVerifications は repo を使う SignupVerifications を返す。
func NewSignupVerifications(repo SignupVerificationRepository) *SignupVerifications {
	return &SignupVerifications{repo: repo}
}

// Create は確認待ちを保存する。前回の送信から SignupResendInterval 以内なら、何も変えず、
// accepted=false を返す。
func (s *SignupVerifications) Create(ctx context.Context, params CreateSignupVerificationParams) (bool, error) {
	return s.repo.CreateSignupVerification(ctx, params)
}

// Confirm は、平文の確認トークンで確認を完了し、作られたユーザーを返す。トークンの保存の形
// への変換（HashSignupToken）はここで行うので、呼び出し側は平文を repository に渡さない。
// 期限切れ・存在しない・使用済みのトークンは（wrap された）ErrSignupTokenInvalid を返す。
func (s *SignupVerifications) Confirm(ctx context.Context, rawToken string) (User, error) {
	return s.repo.CreateUserFromSignupVerification(ctx, HashSignupToken(rawToken))
}

// DiscardExpired は、期限切れの確認待ちを最大 limit 件まで削除する。
func (s *SignupVerifications) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredSignupVerifications(ctx, limit)
}
