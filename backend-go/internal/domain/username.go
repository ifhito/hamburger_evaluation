package domain

// MaxUsernameChars はユーザー名の文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 users_username_max_length（マイグレーション 000009）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、新しい
// マイグレーションの CHECK の両方を直す。
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
