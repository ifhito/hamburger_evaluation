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

// UserProfile は、viewer から見えるユーザーのビューである。ID と Username は
// 誰にでも公開され、Email と Admin は本人にだけ入る（それ以外は nil）。
// 他人や匿名に渡しうる user のレスポンスは、User そのものではなく、この型
// （または {id, username} だけの UserRef）から組み立てる。こうして email と
// admin が誤って漏れる経路を作らない。
type UserProfile struct {
	ID       int64
	Username string
	Email    *string
	Admin    *bool
}

// ProfileFor は、viewer（nil = 匿名）から見た u のビューを返す。Email と Admin
// が入るのは viewer が u 本人のときだけである。admin であっても他人の email と
// admin は見えない（管理者による他人 email の閲覧は対象外。Manages と同じく
// admin に例外はない）。
func (u User) ProfileFor(viewer *User) UserProfile {
	p := UserProfile{ID: u.ID, Username: u.Username}
	if viewer != nil && viewer.ID == u.ID {
		p.Email = &u.Email
		p.Admin = &u.Admin
	}
	return p
}
