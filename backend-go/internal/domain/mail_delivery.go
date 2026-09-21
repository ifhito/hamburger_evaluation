package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MailKind は、送るメールの種類である。
type MailKind string

const (
	// MailKindSignupConfirmation は、signup の確認リンクつきのメールである。
	MailKindSignupConfirmation MailKind = "signup_confirmation"
	// MailKindAlreadyRegistered は、登録済みの email に送る「すでに登録済み」の通知である。
	MailKindAlreadyRegistered MailKind = "already_registered"
)

// MailFailure は、メールの送信に失敗したときの、失敗の種類である。送信の実装（SMTP など）の
// エラーは、この 2 つの言葉に翻訳して記録する。実装の形式・エラーの型は、外へ出さない。
type MailFailure string

const (
	// MailFailureTemporary は、時間をおけば直りうる失敗（接続できない・タイムアウト・一時的な拒否）である。
	MailFailureTemporary MailFailure = "temporary"
	// MailFailurePermanent は、同じ要求を繰り返しても直らない失敗（宛先の拒否・認証の失敗・設定の誤り）である。
	MailFailurePermanent MailFailure = "permanent"
)

// MailResendInterval は、同じ宛先への同じ種類のメールを、続けて出さない間隔である。冪等キーの
// 時間の窓（AlreadyRegisteredMailKey）の幅で、第三者の email へメールを大量に送らせる悪用を抑える。
const MailResendInterval = 60 * time.Second

// MaxMailErrorLength は、記録する失敗の理由（last_error）の最大の文字数である。
const MaxMailErrorLength = 200

// SignupConfirmationMailKey は、確認メールの冪等キーである。確認待ちの id と、その送信の世代
// （置き換えるたびに増える）で決まるので、同じ確認待ちの同じ世代の要求は、何度来ても 1 通だけ出る。
func SignupConfirmationMailKey(verificationID string, generation int) string {
	return fmt.Sprintf("%s:%s:%d", MailKindSignupConfirmation, verificationID, generation)
}

// AlreadyRegisteredMailKey は、登録済みの email への通知の冪等キーである。email（小文字）と、
// MailResendInterval の幅の時間の窓で決まるので、同じ窓の中の同じ宛先への通知は 1 通だけ出る。
// 窓は固定なので、窓をまたぐ 2 つの要求は、続けて 2 通出ることがある（どの 60 秒の間でも、最大 2 通）。
func AlreadyRegisteredMailKey(email string, at time.Time) string {
	window := at.Unix() / int64(MailResendInterval/time.Second)
	return fmt.Sprintf("%s:%s:%d", MailKindAlreadyRegistered, strings.ToLower(email), window)
}

// TruncateMailError は、失敗の理由を、記録できる形にする。改行・連続する空白を 1 つの空白にまとめ、
// MaxMailErrorLength 文字（rune）で切る。理由に、メールの本文・確認トークン・パスワードを入れては
// ならない（呼び出し側の責任）。
func TruncateMailError(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if utf8.RuneCountInString(reason) <= MaxMailErrorLength {
		return reason
	}
	return string([]rune(reason)[:MaxMailErrorLength])
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateMailDeliveryParams は、メール送信の記録として保存するフィールドを保持する。
// 本文・確認トークン・パスワードは含めない（記録するのは、送信の履歴と冪等キーだけ）。
type CreateMailDeliveryParams struct {
	Kind           MailKind
	Recipient      string
	IdempotencyKey string
}

// MailDeliveryRepository は、メール送信の記録の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの MailDeliveries）だけで、usecase は呼ばない。
// 書き込み専用で、読み取りのメソッドは置かない。
type MailDeliveryRepository interface {
	// CreateMailDelivery は、送信の記録を pending で作り、その id を返す。同じ冪等キーの記録が
	// すでにあるときは、何も変えず、created=false を返す（同じ要求は、すでに扱った）。
	CreateMailDelivery(ctx context.Context, params CreateMailDeliveryParams) (id string, created bool, err error)
	// UpdateMailDeliverySent は、id の記録を sent にし、試行の回数を 1 増やす。
	UpdateMailDeliverySent(ctx context.Context, id string) error
	// UpdateMailDeliveryFailed は、id の記録を failed にし、試行の回数を 1 増やして、失敗の種類と
	// 理由を記録する。理由は MaxMailErrorLength 文字までである。
	UpdateMailDeliveryFailed(ctx context.Context, id string, failure MailFailure, reason string) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// MailDeliveries は、メール送信の記録の集約の書き込みオブジェクトである。MailDeliveryRepository を
// 持つのはこの型だけである。メールの送信そのもの（SMTP など）は、この型の外（infra）にあり、ここは
// 送信の前後の記録だけを担う。
type MailDeliveries struct {
	repo MailDeliveryRepository
}

// NewMailDeliveries は repo を使う MailDeliveries を返す。
func NewMailDeliveries(repo MailDeliveryRepository) *MailDeliveries {
	return &MailDeliveries{repo: repo}
}

// Record は、送信の前に、記録を pending で作る。冪等キーが同じ要求はすでに扱っているので、
// recorded=false を返す（呼び出し側は、メールを送らない）。
func (m *MailDeliveries) Record(ctx context.Context, params CreateMailDeliveryParams) (id string, recorded bool, err error) {
	return m.repo.CreateMailDelivery(ctx, params)
}

// MarkSent は、id のメールが送れたことを記録する。
func (m *MailDeliveries) MarkSent(ctx context.Context, id string) error {
	return m.repo.UpdateMailDeliverySent(ctx, id)
}

// MarkFailed は、id のメールの送信に失敗したことを、失敗の種類と切り詰めた理由とともに記録する。
// 未知の失敗の種類は、MailFailureTemporary として記録する。
func (m *MailDeliveries) MarkFailed(ctx context.Context, id string, failure MailFailure, reason string) error {
	if failure != MailFailureTemporary && failure != MailFailurePermanent {
		failure = MailFailureTemporary
	}
	return m.repo.UpdateMailDeliveryFailed(ctx, id, failure, TruncateMailError(reason))
}
