package domain

// テストで、規則の結果を英語の文字列(API の文言の契約)で比べるための短い呼び方。
// 本体のコードは、言語に依らない文言(XIssues の []Message)を返し、文字列にするのは handler である。

func ValidateBio(bio string) []string           { return Texts(LangEN, BioIssues(bio)) }
func ValidateEmail(email string) []string       { return Texts(LangEN, EmailIssues(email)) }
func ValidatePassword(password string) []string { return Texts(LangEN, PasswordIssues(password)) }
func ValidateUsername(username string) []string { return Texts(LangEN, UsernameIssues(username)) }
func ValidateCredentials(email, password string) []string {
	return Texts(LangEN, CredentialsIssues(email, password))
}
