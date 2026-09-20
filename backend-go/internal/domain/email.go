package domain

import (
	"net/mail"
	"unicode"
)

// ValidateEmail は email の形式を検証し、違反の Rails 形式 full message を返す。
// 有効なら nil を返す。メッセージは API の外部契約なので英語のままである。
//
// 空文字列は "Email can't be blank" だけを返す。それ以外は、net/mail の解析が成功し、
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
	if email == "" {
		return []string{"Email can't be blank"}
	}
	for _, r := range email {
		if !unicode.IsPrint(r) {
			return []string{"Email is invalid"}
		}
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return []string{"Email is invalid"}
	}
	return nil
}
