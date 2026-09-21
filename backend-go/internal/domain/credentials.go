package domain

// ValidateCredentials は、認証情報（email とパスワード）の規則を検証し、違反ごとの
// Rails 形式 full message を返す。有効なら nil を返す。
//
// signup とログインが同じ関数を使い、判定を 1 か所に揃える。email の規則
// （ValidateEmail）、パスワードの規則（ValidatePassword）の順に、結果を連結する。
// 新しい規則はここには足さず、それぞれの検証関数に置く。
func ValidateCredentials(email, password string) []string {
	return Texts(LangEN, CredentialsIssues(email, password))
}

// CredentialsIssues は、ValidateCredentials の判定を、言語に依らない文言(Message)で返す。有効なら nil を返す。
func CredentialsIssues(email, password string) []Message {
	return append(EmailIssues(email), PasswordIssues(password)...)
}
