package domain

import (
	"net/mail"
	"unicode"
)

// MaxEmailChars はメールアドレスの文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 users_email_max_length（000001_create_users）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxEmailChars = 254

// ValidateEmail は email の形式を検証し、違反の Rails 形式 full message を返す。
// 有効なら nil を返す。メッセージは API の外部契約なので英語のままである。
//
// 空文字列は "Email can't be blank" だけを返し、MaxEmailChars 文字を超えるときは
// "Email is too long ..." だけを返す（形式の判定より先に上限を見る）。それ以外は、net/mail の解析が成功し、
// アドレスだけ（表示名・コメント・引用・複数のアドレスを含まない）で、入力と
// 完全に一致するとき有効とする。前後の空白も許さない（保存する値を黙って
// 加工しないため）。RFC の完全準拠は目指さない（ドメイン部にドットがなくても有効）。
//
// net/mail は ASCII より上の文字をすべて「使える文字」として通すので、全角スペース・
// NBSP・ゼロ幅スペース・BOM・制御文字のような、目に見えない文字を含む値も、そのままでは
// 有効になってしまう（貼り付けで混ざりやすい）。そのため、印字できない文字を含む値は
// 無効とする（印字できる非 ASCII の文字は、国際化されたアドレスとして許す）。
//
// この規則の判定は domain だけが持ち、frontend は判定を持たない（サーバーの 422
// メッセージの表示だけを行う）。
func ValidateEmail(email string) []string {
	return Texts(LangEN, EmailIssues(email))
}

// EmailIssues は、ValidateEmail の判定を、言語に依らない文言(Message)で返す。有効なら nil を返す。
// 利用者の言語で返す経路(handler)は、こちらを使う。
func EmailIssues(email string) []Message {
	if email == "" {
		return []Message{Msg(keyEmailBlank)}
	}
	if exceedsChars(email, MaxEmailChars) {
		return []Message{Msg(keyEmailTooLong, MaxEmailChars)}
	}
	for _, r := range email {
		if !unicode.IsPrint(r) {
			return []Message{Msg(keyEmailInvalid)}
		}
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return []Message{Msg(keyEmailInvalid)}
	}
	return nil
}
