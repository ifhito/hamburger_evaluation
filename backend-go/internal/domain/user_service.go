package domain

import "context"

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
	Email          *string
	PasswordDigest *string
}

// UserRepository は、ユーザーの書き込みの契約である。domain が宣言し、呼び出すのは
// domain の UserService だけで、usecase は呼ばない（読み取りは usecase の
// UserQuery）。実装は storage のエラーを domain のエラーに対応させる。
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
	// UpdateUserProfile が呼ばれ、その戻り値が応答になる（UserService は
	// 読み取りのメソッドを repository に持たない）。
	UpdateUserProfile(ctx context.Context, id int64, changes ProfileChanges) (User, error)
	// DiscardUser はユーザーを soft delete し（hard DELETE は決して行わない）、
	// 導出された burger の stats の整合性を保つ。
	DiscardUser(ctx context.Context, id int64) error
}

// UserService はユーザーの書き込みの窓口である。UserRepository を呼ぶのは domain の
// この型だけで、usecase は repository に依存せず、書き込みをここに任せる。
// 現時点では repository の書き込みを 1 対 1 で包む薄い層であり、書き込みに付随する
// domain の手順は、usecase ではなくここに置く。
type UserService struct {
	repo UserRepository
}

// NewUserService は repo を使う UserService を返す。
func NewUserService(repo UserRepository) *UserService {
	return &UserService{repo: repo}
}

// Create は新しいユーザーを永続化して返す。email が使用済みなら
// （wrap された）ErrEmailTaken を返す。
func (s *UserService) Create(ctx context.Context, params CreateUserParams) (User, error) {
	return s.repo.CreateUser(ctx, params)
}

// UpdateProfile は、id の、まだ kept なユーザーに changes の存在するフィールドを
// atomic に適用し、保存されたユーザーを返す。
func (s *UserService) UpdateProfile(ctx context.Context, id int64, changes ProfileChanges) (User, error) {
	return s.repo.UpdateUserProfile(ctx, id, changes)
}

// Discard はユーザーを soft delete する。
func (s *UserService) Discard(ctx context.Context, id int64) error {
	return s.repo.DiscardUser(ctx, id)
}
