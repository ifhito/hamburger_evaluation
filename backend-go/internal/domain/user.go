package domain

// User は account の domain 表現である。password digest は意図的に含めない。
// 認証情報が entity 上を流れることは決してなく、persistence/usecase の境界の
// 内側にとどまる。
type User struct {
	ID       int64
	Username string
	Email    string
	Admin    bool
}

// Manages は、ユーザーが指定された id の account を管理（編集または削除）
// してよいかどうかを返す。自分自身の管理のみ可能で、admin にも例外はない
// （issue #16 R2/R3）。
func (u User) Manages(id int64) bool { return u.ID == id }
