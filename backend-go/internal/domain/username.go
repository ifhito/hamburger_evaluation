package domain

import (
	"strings"
	"unicode"
)

// MaxUsernameChars はユーザー名の文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 users_username_max_length（000001_create_users）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxUsernameChars = 50

// ValidateUsername は username の規則を検証し、違反の Rails 形式 full message を返す。
// 有効なら nil を返す。空文字列は "Username can't be blank" だけを返し、
// 上限（MaxUsernameChars 文字）を超えるときは "Username is too long ..." だけを返す。
//
// この規則の判定は domain だけが持つ。signup と、プロフィール更新（送られたときだけ）が
// 同じ関数を通る。
func ValidateUsername(username string) []string {
	if username == "" {
		return []string{"Username can't be blank"}
	}
	if exceedsChars(username, MaxUsernameChars) {
		return []string{tooLongMessage("Username", MaxUsernameChars)}
	}
	return nil
}

// usernameFallback は、外部のサービスから、ユーザー名の材料が何も得られなかったときの、ユーザー名である。
const usernameFallback = "user"

// UsernameFromProfile は、外部のサービスで新しくアカウントを作るときの、ユーザー名を決める。
// 表示名(name)を使い、空ならメールの "@" より前を使い、それも空なら usernameFallback を使う。
// 空白の連続は 1 つにまとめ、制御文字は取り除き、上限(MaxUsernameChars 文字)までに切り詰める。
// 結果は、ValidateUsername の規則を必ず満たす。ユーザー名は重複してよい(一意の制約はない)ので、
// 重複の確認や番号の付け足しはしない。利用者は、あとでプロフィールから変更できる。
func UsernameFromProfile(name, email string) string {
	pick := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, s)
		return truncateChars(strings.Join(strings.Fields(s), " "), MaxUsernameChars)
	}
	if u := pick(name); u != "" {
		return u
	}
	if at := strings.LastIndex(email, "@"); at > 0 {
		if u := pick(email[:at]); u != "" {
			return u
		}
	}
	return usernameFallback
}
