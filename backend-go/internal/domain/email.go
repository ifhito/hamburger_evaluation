package domain

import "net/mail"

// ValidateEmail は email の形式を検証し、違反の Rails 形式 full message を返す。
// 有効なら nil を返す。メッセージは API の外部契約なので英語のままである。
//
// 空文字列は "Email can't be blank" だけを返す。それ以外は、net/mail の解析が成功し、
// アドレスだけ（表示名・コメント・引用・複数のアドレスを含まない）で、入力と
// 完全に一致するとき有効とする。前後の空白も許さない（保存する値を黙って
// 加工しないため）。RFC の完全準拠は目指さない（ドメイン部にドットがなくても有効）。
//
// この規則の判定は domain だけが持ち、frontend は判定を持たない（サーバーの 422
// メッセージの表示だけを行う）。
func ValidateEmail(email string) []string {
	if email == "" {
		return []string{"Email can't be blank"}
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return []string{"Email is invalid"}
	}
	return nil
}
