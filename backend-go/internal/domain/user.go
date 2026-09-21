package domain

import "context"

// User は account の domain 表現である。password digest は意図的に含めない。
// 認証情報が entity 上を流れることは決してなく、persistence/usecase の境界の
// 内側にとどまる。
type User struct {
	// ID は UUID の正規形（小文字・ハイフン区切り）である。DB が生成し、形式の判定は IsUUID が持つ。
	ID       string
	Username string
	// Bio は自己紹介文（biography の略）である。空文字は未設定を表す。誰にでも公開される。
	Bio   string
	Email string
	Admin bool
}

// CanModerate は、shop の moderation（一覧・名称変更・承認・却下）を行ってよいかを返す。
// 権限の判断はこの 1 か所に置き、usecase の認可と、API が返す can_moderate が同じ値を使う。
func (u User) CanModerate() bool { return u.Admin }

// Manages は、ユーザーが指定された id の account を管理（編集または削除）
// してよいかどうかを返す。自分自身の管理のみ可能で、admin にも例外はない
// （issue #16 R2/R3）。
func (u User) Manages(id string) bool { return u.ID == id }

// UserProfile は、viewer から見えるユーザーのビューである。ID・ユーザー名・自己紹介文は
// 誰にでも公開され、Email と Admin は本人にだけ入る（それ以外は nil）。
// 他人や匿名に渡しうる user のレスポンスは、User そのものではなく、この型
// （または {id, username} だけの UserRef）から組み立てる。こうして email と
// admin が誤って漏れる経路を作らない。
type UserProfile struct {
	ID       string
	Username string
	// Bio は自己紹介文で、他人や匿名にも見える（空文字は未設定）。
	Bio   string
	Email *string
	Admin *bool
	// CanEdit は、viewer がこのプロフィールを編集・削除できるか（Manages）である。
	// 匿名は false。frontend は、編集リンクの出し分けにこの値を使う。
	CanEdit bool
}

// ProfileFor は、viewer（nil = 匿名）から見た u のビューを返す。Email と Admin
// が入るのは viewer が u 本人のときだけである。admin であっても他人の email と
// admin は見えない（管理者による他人 email の閲覧は対象外。Manages と同じく
// admin に例外はない）。
func (u User) ProfileFor(viewer *User) UserProfile {
	p := UserProfile{ID: u.ID, Username: u.Username, Bio: u.Bio}
	if viewer != nil && viewer.ID == u.ID {
		p.Email = &u.Email
		p.Admin = &u.Admin
	}
	p.CanEdit = viewer != nil && viewer.Manages(u.ID)
	return p
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateUserParams は、新しいユーザーとして永続化するフィールドを保持する。
// パスワードはハッシュ化済みの状態で渡される。repository が平文を目にする
// ことはない。
type CreateUserParams struct {
	Username       string
	Email          string
	PasswordDigest string
	Admin          bool
}

// ProfileChanges は、UserRepository.UpdateUserProfile の、任意のカラム限定の
// プロフィール更新を保持する。nil のフィールドは変更されない。パスワードは、
// CreateUserParams と同様にハッシュ化済みの状態で渡される。repository が平文を
// 目にすることはない。
type ProfileChanges struct {
	Username       *string
	Bio            *string
	Email          *string
	PasswordDigest *string
}

// UserRepository は、ユーザーの書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの Users）だけで、usecase は呼ばない
// （読み取りは usecase の UserQuery）。実装は storage のエラーを domain のエラーに対応させる。
// CreateUser と UpdateUserProfile は email の unique violation に対して
// （wrap された）ErrEmailTaken を返し、UpdateUserProfile と DiscardUser は、
// active な（discard されていない）ユーザーが一致しないとき（wrap された）
// ErrUserNotFound を返す。書き込み専用で、読み取りのメソッドは置かない。
type UserRepository interface {
	// CreateUser は新しいユーザーを永続化して返す。
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	// UpdateUserProfile は、id の、まだ kept なユーザーに changes の存在する
	// フィールドを atomic に適用し、保存されたユーザーを返す。存在する
	// フィールドがゼロ個なら単なる lookup になる
	// （200 の no-op、Rails parity）。存在するフィールドがゼロ個のときの
	// lookup は repository の実装の内部で行われる。変更が空でも
	// UpdateUserProfile が呼ばれ、その戻り値が応答になる（Users は
	// 読み取りのメソッドを repository に持たない）。
	UpdateUserProfile(ctx context.Context, id string, changes ProfileChanges) (User, error)
	// DiscardUser はユーザーを soft delete し（hard DELETE は決して行わない）、
	// 導出された burger の stats の整合性を保つ。
	DiscardUser(ctx context.Context, id string) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// Users はユーザー集約の書き込みオブジェクトである。UserRepository を持つのは
// この型だけで、usecase は repository に依存せず、ユーザーの書き込みをここに任せる。
// ユーザーだけを更新する書き込みは、Service ではなくこの型に置く（Service は複数の
// 集約を跨ぐ更新だけに使う。domain/doc.go を参照）。現時点では repository の
// 書き込みを 1 対 1 で包んでいる。ユーザーに関する domain の手順が増えたときは、
// usecase ではここへ置く。
type Users struct {
	repo UserRepository
}

// NewUsers は repo を使う Users を返す。
func NewUsers(repo UserRepository) *Users {
	return &Users{repo: repo}
}

// Create は新しいユーザーを永続化して返す。email が使用済みなら
// （wrap された）ErrEmailTaken を返す。
func (s *Users) Create(ctx context.Context, params CreateUserParams) (User, error) {
	return s.repo.CreateUser(ctx, params)
}

// UpdateProfile は、id の、まだ kept なユーザーに changes の存在するフィールドを
// atomic に適用し、保存されたユーザーを返す。
func (s *Users) UpdateProfile(ctx context.Context, id string, changes ProfileChanges) (User, error) {
	return s.repo.UpdateUserProfile(ctx, id, changes)
}

// Discard はユーザーを soft delete する。
func (s *Users) Discard(ctx context.Context, id string) error {
	return s.repo.DiscardUser(ctx, id)
}
