package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// LoginHandoffTTL は、外部のサービスでのサインインの結果を、画面(SPA)へ渡すコードの有効期間である。
// コードは、画面の URL に載って渡るので、短く保ち、1 回しか使えないようにする(有効期間は業務のルールで、
// 環境ごとに変える設定ではない)。
const LoginHandoffTTL = 60 * time.Second

// loginHandoffCodeBytes は、コードの乱数のバイト数である(256 ビット)。
const loginHandoffCodeBytes = 32

// ErrLoginHandoffInvalid は、画面へ渡すコードが、期限切れ・存在しない・使用済み・用途が違う、の
// いずれかであることを表す。これらは意図的に同一で、呼び出し側は、どれなのかを区別できない。
var ErrLoginHandoffInvalid = errors.New("login handoff code is invalid or has expired")

// LoginHandoffOutcome は、サインインの手続きの結果の種類である。
type LoginHandoffOutcome string

const (
	// OutcomeSignedIn は、サインインに成功した(新規登録を含む)ことを表す。利用者の ID を伴う。
	OutcomeSignedIn LoginHandoffOutcome = "signed_in"
	// OutcomeLinked は、ログイン済みの利用者に、外部のアカウントを結び付けたことを表す。利用者の ID を伴う。
	OutcomeLinked LoginHandoffOutcome = "linked"
	// OutcomeLinkIntent は、ログイン済みの利用者が、外部のアカウントを結び付けたい、という意思である。
	// 利用者の ID を伴い、結び付けの手続きを、その利用者のものとして始めるためだけに使う(サインインには使えない)。
	OutcomeLinkIntent LoginHandoffOutcome = "link_intent"
	// OutcomeAccountExists は、同じメールの利用者がすでにいて、自動では結び付けなかったことを表す。
	OutcomeAccountExists LoginHandoffOutcome = "account_exists"
	// OutcomeIdentityTaken は、その外部のアカウントが、すでにほかの利用者に結び付いていることを表す。
	OutcomeIdentityTaken LoginHandoffOutcome = "identity_taken"
	// OutcomeAlreadyLinked は、その利用者が、すでに同じサービスの別のアカウントと結び付いていることを表す。
	OutcomeAlreadyLinked LoginHandoffOutcome = "already_linked"
	// OutcomeFailed は、上のどれでもない失敗(拒否・確認の失敗など)を表す。原因の詳細は、持たない。
	OutcomeFailed LoginHandoffOutcome = "failed"
)

// NeedsUser は、この結果が、利用者の ID を伴うか(DB の CHECK 制約 login_handoffs_user_check と同じ規則)を返す。
func (o LoginHandoffOutcome) NeedsUser() bool {
	return o == OutcomeSignedIn || o == OutcomeLinked || o == OutcomeLinkIntent
}

// LoginHandoff は、画面へ渡すコードの中身である。コードを 1 回使うと、この内容を返して、消える。
type LoginHandoff struct {
	ID      string
	Outcome LoginHandoffOutcome
	// UserID は、結果が利用者を伴うとき(NeedsUser)の、その利用者の ID である。ほかは空である。
	UserID string
	// ReturnTo は、手続きのあとに戻る先(アプリの中のパス。SanitizeReturnTo を通ったもの)である。空は既定の画面。
	ReturnTo string
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateLoginHandoffParams は、コードの中身として保存するフィールドを保持する。コードは CodeHash
// (SHA-256。平文は渡さない)である。有効期間は LoginHandoffTTL で、repository が使う。
type CreateLoginHandoffParams struct {
	CodeHash string
	Outcome  LoginHandoffOutcome
	UserID   string
	ReturnTo string
}

// LoginHandoffRepository は、画面へ渡すコードの書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード(書き込みオブジェクトの LoginHandoffs)だけで、usecase は呼ばない。書き込み専用で、
// 読み取りのメソッドは置かない。
type LoginHandoffRepository interface {
	// CreateLoginHandoff はコードの中身を保存する。
	CreateLoginHandoff(ctx context.Context, params CreateLoginHandoffParams) error
	// DiscardLoginHandoff は、codeHash の、期限内のコードの中身を削除して、その内容を返す。削除と
	// 読み取りは 1 つの文で行うので、同じコードを並行して使っても、成功するのは 1 回だけである。
	// 期限切れ・存在しない・すでに使われたものは、(wrap された)ErrLoginHandoffInvalid を返す。
	DiscardLoginHandoff(ctx context.Context, codeHash string) (LoginHandoff, error)
	// DiscardExpiredLoginHandoffs は、期限切れのコードの中身を、最大 limit 件まで削除し、削除した件数を返す。
	DiscardExpiredLoginHandoffs(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// LoginHandoffs は、画面へ渡すコードの書き込みオブジェクトである。LoginHandoffRepository を持つのは
// この型だけで、usecase は repository に依存しない。コードの作成と、その保存の形(SHA-256)への
// 変換はここで行い、平文のコードは repository に渡さない。
type LoginHandoffs struct {
	repo LoginHandoffRepository
}

// NewLoginHandoffs は repo を使う LoginHandoffs を返す。
func NewLoginHandoffs(repo LoginHandoffRepository) *LoginHandoffs {
	return &LoginHandoffs{repo: repo}
}

// Issue は、結果を画面へ渡すコードを作って保存し、平文のコードを返す。平文は、画面の URL にだけ
// 入れる値で、保存しない。userID は、結果が利用者を伴うとき(NeedsUser)だけ、伴わないときは空にする。
func (s *LoginHandoffs) Issue(ctx context.Context, outcome LoginHandoffOutcome, userID, returnTo string) (string, error) {
	if outcome.NeedsUser() != (userID != "") {
		return "", fmt.Errorf("issue login handoff: outcome %q and user id do not match", outcome)
	}
	buf := make([]byte, loginHandoffCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate login handoff code: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	if err := s.repo.CreateLoginHandoff(ctx, CreateLoginHandoffParams{
		CodeHash: hashLoginHandoffCode(raw),
		Outcome:  outcome,
		UserID:   userID,
		ReturnTo: returnTo,
	}); err != nil {
		return "", fmt.Errorf("create login handoff: %w", err)
	}
	return raw, nil
}

// Redeem は、平文のコードを 1 回だけ使って、その中身を返す。期限切れ・存在しない・使用済みは、
// (wrap された)ErrLoginHandoffInvalid を返す。
func (s *LoginHandoffs) Redeem(ctx context.Context, rawCode string) (LoginHandoff, error) {
	return s.repo.DiscardLoginHandoff(ctx, hashLoginHandoffCode(rawCode))
}

// DiscardExpired は、期限切れのコードの中身を、最大 limit 件まで削除する。
func (s *LoginHandoffs) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredLoginHandoffs(ctx, limit)
}

// hashLoginHandoffCode は、平文のコードを、保存する形(SHA-256 の 16 進)に変換する。コードは
// 256 ビットの乱数なので、パスワードと違い、遅いハッシュは要らない。
func hashLoginHandoffCode(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
