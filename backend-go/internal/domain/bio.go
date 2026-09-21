package domain

// Bio は自己紹介文（biography の略）である。プロフィールに載せる、利用者が自由に書く短い文章。

// MaxBioChars は自己紹介文の文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 users_bio_max_length（000001_create_users）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxBioChars = 500

// ValidateBio は自己紹介文の規則を検証し、違反の Rails 形式 full message を返す。
// 有効なら nil を返す。空文字列は「未設定」として有効で（書いた自己紹介文を消せる）、
// 上限（MaxBioChars 文字）を超えるときは "Bio is too long ..." だけを返す。
// メッセージは API の外部契約なので英語のままである。
//
// この規則の判定は domain だけが持ち、frontend は判定を持たない（サーバーの 422
// メッセージの表示だけを行う）。プロフィール更新で、送られたときだけ判定する。
func ValidateBio(bio string) []string {
	if exceedsChars(bio, MaxBioChars) {
		return []string{tooLongMessage("Bio", MaxBioChars)}
	}
	return nil
}
