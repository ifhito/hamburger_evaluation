package domain

import (
	"crypto/rand"
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

// usernameGeneratedPrefix は、外部のサービスから、使えるユーザー名の材料が得られなかったときに作る、中立な
// ユーザー名の前置きである。
const usernameGeneratedPrefix = "user-"

// UsernameFromProfile は、外部のサービスで新しくアカウントを作るときの、ユーザー名を決める。
// 表示名(name)を使い、使えなければ、メールとは無関係な、中立の名前(usernameGeneratedPrefix + 乱数)を作る。
// **メールアドレスの一部は、ユーザー名の材料にしない**(ユーザー名は誰にでも公開されるが、メールは本人にしか
// 見せない。「john.smith1985@…」から「john.smith1985」を公開してしまわないため)。
//
// 表示名は、次のように整える: 書式の文字(Cf。双方向の上書き・ゼロ幅の文字など、表示を逆転させたり、見えない
// 文字で別人に見せかけたりできるもの)は取り除き、制御文字(Cc)は空白として扱い、空白(行・段落の区切り Zl・Zp と
// 全角の空白を含む)の連続は 1 つにまとめ、上限(MaxUsernameChars 文字)までに切り詰めて、切り詰めた末尾の空白も取り除く。
// 結果は、ValidateUsername の規則を必ず満たす。ユーザー名は重複してよい(一意の制約はない)ので、重複の確認や
// 番号の付け足しはしない。利用者は、あとでプロフィールから変更できる。
func UsernameFromProfile(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.Is(unicode.Cf, r):
			return -1
		case unicode.IsControl(r):
			return ' '
		}
		return r
	}, name)
	if u := strings.TrimSpace(truncateChars(strings.Join(strings.Fields(name), " "), MaxUsernameChars)); u != "" {
		return u
	}
	return usernameGeneratedPrefix + strings.ToLower(rand.Text()[:8])
}
