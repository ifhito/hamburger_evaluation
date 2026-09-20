package domain

import "fmt"

const (
	// MinPasswordBytes はパスワードの最小バイト数である。
	// 規則の判定はこの domain だけが持つ。frontend の入力欄の説明文（locale の passwordHint）に
	// 数値が書かれているので、変えるときは説明文も直す。
	MinPasswordBytes = 8
	// MaxPasswordBytes はパスワードの最大バイト数である。bcrypt の入力上限（72 バイト）に合わせる。
	MaxPasswordBytes = 72
)

// ValidatePassword は password の強度ルールを検証し、違反ごとの Rails 形式 full message を返す。
// 有効なら nil を返す。
//
// 長さは文字数ではなくバイト数で数える（bcrypt の入力上限に合わせるため）。
// 空文字列は "can't be blank" だけを返す。それ以外は該当する違反を
// 「短い → 長い → 文字種」の順にすべて返す。メッセージは API の外部契約なので英語のまま。
//
// この規則の判定は domain だけが持ち、frontend は判定を持たない（説明文の表示と、
// サーバーの 422 メッセージの表示だけを行う）。
func ValidatePassword(password string) []string {
	if password == "" {
		return []string{"Password can't be blank"}
	}
	var msgs []string
	if len(password) < MinPasswordBytes {
		msgs = append(msgs, fmt.Sprintf("Password is too short (minimum is %d characters)", MinPasswordBytes))
	}
	if len(password) > MaxPasswordBytes {
		msgs = append(msgs, fmt.Sprintf("Password is too long (maximum is %d characters)", MaxPasswordBytes))
	}
	if !hasAllCharKinds(password) {
		msgs = append(msgs, "Password must include letters, numbers and symbols")
	}
	return msgs
}

// hasAllCharKinds は s が半角英字・半角数字・記号をそれぞれ 1 文字以上含むかを返す。
// 判定はバイト単位の ASCII 判定で、正規化はしない。英字と数字を先に除くと、残る
// 印字可能文字（0x21-0x7E）がちょうど記号になる。空白（0x20）・制御文字・DEL・
// 非 ASCII（0x80 以上）は、どの種別にも数えない。
func hasAllCharKinds(s string) bool {
	var letter, digit, symbol bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z':
			letter = true
		case '0' <= c && c <= '9':
			digit = true
		case 0x21 <= c && c <= 0x7E:
			symbol = true
		}
	}
	return letter && digit && symbol
}
