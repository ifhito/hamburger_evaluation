package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ProfileChanges は、UsersRepository.UpdateUserProfile の、任意のカラム限定の
// プロフィール更新を保持する。nil のフィールドは変更されない。パスワードは、
// CreateUserParams と同様にハッシュ化済みの状態で渡される。repository が平文を
// 目にすることはない。
type ProfileChanges struct {
	Username       *string
	Email          *string
	PasswordDigest *string
}

// UsersRepository は、ユーザー管理の use case 向けの consumer 側の永続化の
// 契約である。実装は storage のエラーを domain のエラーに対応させる。
// lookup と書き込みは、active な（discard されていない）ユーザーが一致しない
// とき（wrap された）domain.ErrUserNotFound を返し、UpdateUserProfile は
// email の unique violation に対して（wrap された）domain.ErrEmailTaken を
// 返す。
type UsersRepository interface {
	// ListActiveUsers は kept なユーザーをすべて id の昇順で返す
	// （ページネーションなし。Rails parity：index は kept なユーザーを
	// すべて返す）。
	ListActiveUsers(ctx context.Context) ([]domain.User, error)
	// GetActiveUserByID は、指定された id の、discard されていないユーザーを
	// 返す。
	GetActiveUserByID(ctx context.Context, id int64) (domain.User, error)
	// UpdateUserProfile は、id の、まだ kept なユーザーに changes の存在する
	// フィールドを atomic に適用し、保存されたユーザーを返す。存在する
	// フィールドがゼロ個なら単なる lookup になる
	// （200 の no-op、Rails parity）。
	UpdateUserProfile(ctx context.Context, id int64, changes ProfileChanges) (domain.User, error)
	// DiscardUser はユーザーを soft delete し（hard DELETE は決して行わない）、
	// 導出された burger の stats の整合性を保つ。
	DiscardUser(ctx context.Context, id int64) error
}

// Users は、ユーザー管理の use case を実装する。公開の index と、本人のみが
// 行えるプロフィールの更新およびアカウントの削除である。
type Users struct {
	repo   UsersRepository
	hasher PasswordHasher
}

func NewUsers(repo UsersRepository, hasher PasswordHasher) *Users {
	return &Users{repo: repo, hasher: hasher}
}

// List は kept なユーザーをすべて id の昇順で返す。
func (s *Users) List(ctx context.Context) ([]domain.User, error) {
	users, err := s.repo.ListActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// UpdateUserInput はプロフィール更新の入力である。すべてのフィールドは任意で
// （リクエストに含まれなければ nil）、含まれないフィールドは変更されない。
// Rails の strong params に合わせた部分更新である。
type UpdateUserInput struct {
	Username             *string
	Email                *string
	Password             *string
	PasswordConfirmation *string
}

// passwordPresent は、入力がパスワードの変更を求めているかどうかを返す。
// Rails の has_secure_password の微妙な挙動：「空文字列」で渡された
// パスワードは存在しないものとして扱われる（`password=("")` は digest に
// 触れず、エラーも起こさない）ので、nil と "" はどちらも "no change" を
// 意味する。
func (in UpdateUserInput) passwordPresent() bool {
	return in.Password != nil && *in.Password != ""
}

// validate は Rails parity の full message を返す。valid なら空である。
func (in UpdateUserInput) validate() []string {
	var msgs []string
	if in.Username != nil && *in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	if in.Email != nil && *in.Email == "" {
		msgs = append(msgs, "Email can't be blank")
	}
	if in.passwordPresent() && len(*in.Password) > maxPasswordBytes {
		msgs = append(msgs, "Password is too long (maximum is 72 characters)")
	}
	if in.PasswordConfirmation != nil {
		password := ""
		if in.Password != nil {
			password = *in.Password
		}
		if *in.PasswordConfirmation != password {
			msgs = append(msgs, "Password confirmation doesn't match Password")
		}
	}
	return msgs
}

// Update は、issue #16 AC2 のチェック順序で対象ユーザーのプロフィールを
// 編集する（退役済みの Rails の controller とは意図的に異なる。その
// controller は path の id を無視して current_user に対して動作していた）。
// load（存在しないユーザーと discard 済みのユーザーはどちらも 404。所有者で
// なくても同じ）、domain の本人管理ルール（403）、入力の validation
// （422）、そしてカラム限定の書き込みの順である。すでに使われている email は、
// signup と同様に *domain.ValidationError として返される。
func (s *Users) Update(ctx context.Context, viewer domain.User, targetID int64, input UpdateUserInput) (domain.User, error) {
	target, err := s.repo.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.User{}, domain.ErrForbidden
	}
	if msgs := input.validate(); len(msgs) > 0 {
		return domain.User{}, &domain.ValidationError{Messages: msgs}
	}
	changes := ProfileChanges{Username: input.Username, Email: input.Email}
	if input.passwordPresent() {
		digest, err := s.hasher.Hash(*input.Password)
		if err != nil {
			return domain.User{}, fmt.Errorf("hash password: %w", err)
		}
		changes.PasswordDigest = &digest
	}
	updated, err := s.repo.UpdateUserProfile(ctx, targetID, changes)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, &domain.ValidationError{Messages: []string{"Email has already been taken"}}
		}
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	return updated, nil
}

// Delete は対象ユーザーのアカウントを soft delete する。load（404。所有者で
// なくても同じ）、domain の本人管理ルール（403）、そして discard の順で
// 行い、hard DELETE は決して行わない。
func (s *Users) Delete(ctx context.Context, viewer domain.User, targetID int64) error {
	target, err := s.repo.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.ErrForbidden
	}
	if err := s.repo.DiscardUser(ctx, targetID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
