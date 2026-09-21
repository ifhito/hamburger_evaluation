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

// SignupTokenTTL は、signup の確認トークンの有効期間である。有効期間は業務のルールで、
// 環境ごとに変える設定ではないので、ここに置く（確認メールの本文の「有効期間」も、この値を渡す）。
const SignupTokenTTL = 24 * time.Hour

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
// 有効期間は SignupTokenTTL で、repository が使う。
type CreateSignupVerificationParams struct {
	Email          string
	Username       string
	PasswordDigest string
	TokenHash      string
}

// SignupVerificationReceipt は、確認待ちの保存の結果である。Accepted=false は、前回の送信から
// SignupResendInterval 以内で、何も変えなかったことを表す（ID・Generation は空）。Accepted のときの
// ID と Generation は、確認メールの冪等キー（SignupConfirmationMailKey）の材料になる。
type SignupVerificationReceipt struct {
	Accepted   bool
	ID         string
	Generation int
}

// PendingSignup は、確認待ちの signup の内容である。確認のときに、この内容でユーザーを作る。
// パスワードはハッシュ化済みで、平文は保存していない。
type PendingSignup struct {
	ID             string
	Email          string
	Username       string
	PasswordDigest string
}

// SignupVerificationRepository は、確認待ちの signup の書き込みの契約である。domain が
// 宣言し、呼び出すのは domain のコード（書き込みオブジェクトの SignupVerifications）だけで、
// usecase は呼ばない。書き込み専用で、読み取りのメソッドは置かない。
type SignupVerificationRepository interface {
	// CreateSignupVerification は、params の email（大文字小文字を区別しない）の確認待ちを、
	// 最新の入力で置き換えて保存する（なければ作る）。前回の送信から SignupResendInterval
	// 以内なら何も変えず、Accepted=false を返す（メールを送らない合図）。判定と書き込みは
	// 1 つの文で行うので、並行しても間隔は破られない。置き換えるたびに Generation が 1 増える。
	CreateSignupVerification(ctx context.Context, params CreateSignupVerificationParams) (SignupVerificationReceipt, error)
	// LockSignupVerification は、tokenHash の期限内の確認待ちを排他ロックする(トランザクションの中で
	// 呼ぶと、そのトランザクションが終わるまで、同じ行を扱うほかの処理は待たされる)。データは返さず、
	// 行も変えない。期限切れ・存在しない・すでに使われた(削除された)ものは、(wrap された)
	// ErrSignupTokenInvalid を返す。同じトークンでの並行する確認を、1 件ずつに直列にするために、
	// 確認の手順の先頭で使う。
	LockSignupVerification(ctx context.Context, tokenHash string) error
	// DiscardSignupVerification は、id の確認待ちを削除する。確認が終わって、同じトークンで
	// 再び確認できないようにするために使う。
	DiscardSignupVerification(ctx context.Context, id string) error
	// DiscardExpiredSignupVerifications は、期限切れの確認待ちを、最大 limit 件まで削除し、
	// 削除した件数を返す。
	DiscardExpiredSignupVerifications(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// SignupVerifications は、確認待ちの signup 集約の書き込みオブジェクトである。
// SignupVerificationRepository を持つのはこの型だけで、usecase は repository に依存しない。
// 確認(確認待ちのロック → 内容の読み取り → ユーザーの作成 → 確認待ちの削除)は、読み取りを
// 途中にはさみ、確認待ちとユーザーの 2 つの集約を更新する手順なので、この型には置かない。
// トランザクションを持つ usecase が、UnitOfWork(ここからここまでを 1 つのトランザクションにする
// 範囲を指定する仕組み)の中で、この型の Lock・Discard と、ユーザーの書き込みオブジェクトを組み合わせて
// 組み立てる。
type SignupVerifications struct {
	repo SignupVerificationRepository
}

// NewSignupVerifications は repo を使う SignupVerifications を返す。
func NewSignupVerifications(repo SignupVerificationRepository) *SignupVerifications {
	return &SignupVerifications{repo: repo}
}

// Create は確認待ちを保存する。前回の送信から SignupResendInterval 以内なら、何も変えず、
// Accepted=false の結果を返す。
func (s *SignupVerifications) Create(ctx context.Context, params CreateSignupVerificationParams) (SignupVerificationReceipt, error) {
	return s.repo.CreateSignupVerification(ctx, params)
}

// Lock は、平文の確認トークンに対応する期限内の確認待ちを排他ロックする。トークンの保存の形
// への変換(HashSignupToken)はここで行うので、呼び出し側は平文を repository に渡さない。
// 期限切れ・存在しない・使用済みのトークンは、(wrap された)ErrSignupTokenInvalid を返す。
func (s *SignupVerifications) Lock(ctx context.Context, rawToken string) error {
	return s.repo.LockSignupVerification(ctx, HashSignupToken(rawToken))
}

// Discard は、id の確認待ちを削除する(確認が終わった確認待ちを、再利用できなくする)。
func (s *SignupVerifications) Discard(ctx context.Context, id string) error {
	return s.repo.DiscardSignupVerification(ctx, id)
}

// DiscardExpired は、期限切れの確認待ちを最大 limit 件まで削除する。
func (s *SignupVerifications) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredSignupVerifications(ctx, limit)
}
