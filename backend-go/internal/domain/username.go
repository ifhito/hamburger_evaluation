package domain

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
