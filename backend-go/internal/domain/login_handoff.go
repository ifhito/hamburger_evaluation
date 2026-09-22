package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

// loginHandoffCodeBytes は、コードと結び付けの値の乱数のバイト数である(256 ビット)。
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
	return o == OutcomeSignedIn || o == OutcomeLinked
}

// LoginHandoff は、画面へ渡すコードの中身である。コードを 1 回使うと、この内容を返して、消える。
type LoginHandoff struct {
	ID      string
	Outcome LoginHandoffOutcome
	// UserID は、結果が利用者を伴うとき(NeedsUser)の、その利用者の ID である。ほかは空である。
	UserID string
	// ReturnTo は、手続きのあとに戻る先(アプリの中のパス。SanitizeReturnTo を通ったもの)である。空は既定の画面。
	// 失敗の結果にも入る(失敗のあと、元の画面へ戻れるように)。
	ReturnTo string
	// BinderHash は、このコードを使える相手(手続きを終えたブラウザ)を確かめる値の、保存の形(SHA-256)である。
	// コードだけでは使えない(BoundTo)。
	BinderHash string
}

// BoundTo は、binder(平文)が、このコードを発行したときに渡した、結び付けの値かを返す(定数時間で比べる)。
// 空の binder は、常に false である。コードは、手続きを終えたブラウザだけが持つ結び付けの値と一緒に使うときだけ
// 有効で、コードだけを別のブラウザへ持ち込んでも(たとえば、攻撃者が自分の Google で進めた手続きの URL を、
// 被害者に踏ませても)、使えない(被害者が、攻撃者のアカウントでログインした状態になる、ログイン CSRF を防ぐ)。
func (h LoginHandoff) BoundTo(binder string) bool {
	if binder == "" || h.BinderHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(HashLoginHandoffBinder(binder)), []byte(h.BinderHash)) == 1
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateLoginHandoffParams は、コードの中身として保存するフィールドを保持する。コードは CodeHash・結び付けの値は
// BinderHash(どちらも SHA-256。平文は渡さない)である。有効期間は LoginHandoffTTL で、repository が使う。
type CreateLoginHandoffParams struct {
	CodeHash   string
	BinderHash string
	Outcome    LoginHandoffOutcome
	UserID     string
	ReturnTo   string
}

// LoginHandoffRepository は、画面へ渡すコードの書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード(書き込みオブジェクトの LoginHandoffs)だけで、usecase は呼ばない。書き込み専用で、
// 読み取りのメソッドは置かない(読み取りは usecase の LoginHandoffQuery)。
type LoginHandoffRepository interface {
	// CreateLoginHandoff はコードの中身を保存する。
	CreateLoginHandoff(ctx context.Context, params CreateLoginHandoffParams) error
	// LockLoginHandoff は、codeHash の、期限内のコードの中身を排他ロックする(トランザクションの中で呼ぶと、
	// そのトランザクションが終わるまで、同じコードを使うほかの処理は待たされる)。データは返さず、行も変えない。
	// 期限切れ・存在しない・すでに使われた(削除された)ものは、(wrap された)ErrLoginHandoffInvalid を返す。
	// 後続の処理(利用者の取得・トークンの発行)が成功してから削除する手順の先頭で使い、同じコードの
	// 並行する交換を 1 件ずつに直列にする(負けた側は、行が消えているのを見て ErrLoginHandoffInvalid になる)。
	LockLoginHandoff(ctx context.Context, codeHash string) error
	// DiscardLoginHandoff は、id のコードの中身を削除する(使ったコードを、再び使えなくする)。
	DiscardLoginHandoff(ctx context.Context, id string) error
	// DiscardExpiredLoginHandoffs は、期限切れのコードの中身を、最大 limit 件まで削除し、削除した件数を返す。
	DiscardExpiredLoginHandoffs(ctx context.Context, limit int) (int64, error)
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// LoginHandoffs は、画面へ渡すコードの書き込みオブジェクトである。LoginHandoffRepository を持つのは
// この型だけで、usecase は repository に依存しない。コードの作成と、その保存の形(SHA-256)への
// 変換はここで行い、平文のコードは repository に渡さない。コードを使う手順(ロック → 内容の読み取り →
// 後続の処理 → 削除)は、途中に読み取りが入るので、この型には置かず、トランザクションを持つ usecase が
// UnitOfWork の中で、この型の Lock・Discard を使って組み立てる。
type LoginHandoffs struct {
	repo LoginHandoffRepository
}

// NewLoginHandoffs は repo を使う LoginHandoffs を返す。
func NewLoginHandoffs(repo LoginHandoffRepository) *LoginHandoffs {
	return &LoginHandoffs{repo: repo}
}

// IssuedLoginHandoff は、発行したコードと、その結び付けの値(どちらも平文)である。コードは画面の URL に載せて渡し、
// 結び付けの値は、手続きを終えたブラウザの cookie にだけ持たせる。どちらも保存しない(保存するのは、SHA-256 だけ)。
type IssuedLoginHandoff struct {
	Code   string
	Binder string
}

// Issue は、結果を画面へ渡すコードと、そのコードを使える相手を確かめる結び付けの値を作って保存し、平文で返す。
// userID は、結果が利用者を伴うとき(NeedsUser)だけ、伴わないときは空にする。
func (s *LoginHandoffs) Issue(ctx context.Context, outcome LoginHandoffOutcome, userID, returnTo string) (IssuedLoginHandoff, error) {
	if outcome.NeedsUser() != (userID != "") {
		return IssuedLoginHandoff{}, fmt.Errorf("issue login handoff: outcome %q and user id do not match", outcome)
	}
	code, err := randomLoginHandoffSecret()
	if err != nil {
		return IssuedLoginHandoff{}, fmt.Errorf("generate login handoff code: %w", err)
	}
	binder, err := randomLoginHandoffSecret()
	if err != nil {
		return IssuedLoginHandoff{}, fmt.Errorf("generate login handoff binder: %w", err)
	}
	if err := s.repo.CreateLoginHandoff(ctx, CreateLoginHandoffParams{
		CodeHash:   HashLoginHandoffCode(code),
		BinderHash: HashLoginHandoffBinder(binder),
		Outcome:    outcome,
		UserID:     userID,
		ReturnTo:   returnTo,
	}); err != nil {
		return IssuedLoginHandoff{}, fmt.Errorf("create login handoff: %w", err)
	}
	return IssuedLoginHandoff{Code: code, Binder: binder}, nil
}

func randomLoginHandoffSecret() (string, error) {
	buf := make([]byte, loginHandoffCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Lock は、平文のコードに対応する、期限内のコードの中身を排他ロックする。コードの保存の形への変換
// (HashLoginHandoffCode)はここで行うので、呼び出し側は平文を repository に渡さない。期限切れ・存在しない・
// 使用済みは、(wrap された)ErrLoginHandoffInvalid を返す。
func (s *LoginHandoffs) Lock(ctx context.Context, rawCode string) error {
	return s.repo.LockLoginHandoff(ctx, HashLoginHandoffCode(rawCode))
}

// Discard は、id のコードの中身を削除する(使ったコードを、再び使えなくする)。
func (s *LoginHandoffs) Discard(ctx context.Context, id string) error {
	return s.repo.DiscardLoginHandoff(ctx, id)
}

// DiscardExpired は、期限切れのコードの中身を、最大 limit 件まで削除する。
func (s *LoginHandoffs) DiscardExpired(ctx context.Context, limit int) (int64, error) {
	return s.repo.DiscardExpiredLoginHandoffs(ctx, limit)
}

// HashLoginHandoffCode は、平文のコードを、保存する形(SHA-256 の 16 進)に変換する。コードは
// 256 ビットの乱数なので、パスワードと違い、遅いハッシュは要らない。
func HashLoginHandoffCode(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HashLoginHandoffBinder は、平文の結び付けの値を、保存する形(SHA-256 の 16 進)に変換する(コードと同じ理由で、
// 遅いハッシュは要らない。コードのハッシュと取り違えないよう、別の名前にしてある)。
func HashLoginHandoffBinder(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
