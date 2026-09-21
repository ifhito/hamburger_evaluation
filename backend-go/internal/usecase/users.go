package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// Users は、ユーザー管理の use case を実装する。viewer から見えるビューでの
// 詳細、および本人のみが行えるプロフィールの更新とアカウントの削除である。
// 読み取りは query、書き込みは domain の書き込みオブジェクト（domain.Users）だけを
// 通し、repository には依存しない。アカウントの削除と、そのユーザーの review が付く
// burger の統計の再計算は、1 つの UnitOfWork.Do（同一トランザクション）の中で組み立てる。
type Users struct {
	query  UserQuery
	users  *domain.Users
	uow    UnitOfWork
	recalc *BurgerStatsRecalculator
	hasher PasswordHasher
}

func NewUsers(query UserQuery, users *domain.Users, uow UnitOfWork, recalc *BurgerStatsRecalculator, hasher PasswordHasher) *Users {
	return &Users{query: query, users: users, uow: uow, recalc: recalc, hasher: hasher}
}

// Get は、discard されていないユーザー 1 人を、viewer（nil = 匿名）から見える
// ビューにして返す。存在しないユーザーと discard 済みのユーザーは、どちらも
// domain.ErrUserNotFound になる。
func (s *Users) Get(ctx context.Context, viewer *domain.User, id string) (domain.UserProfile, error) {
	user, err := s.query.GetActiveUserByID(ctx, id)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("get user: %w", err)
	}
	return user.ProfileFor(viewer), nil
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
// メッセージは username、email（domain.ValidateEmail）、password
// （domain.ValidatePassword）、confirmation の順に並ぶ。email を送らない入力（nil）、
// 現在の値と同じ email を送る入力、パスワードを変更しない入力（nil と ""）には、
// それぞれの規則を適用しない。email の形式の規則は、新しく設定するときだけ判定する
// （規則ができる前の、形式が合わない email を持つ既存ユーザーが、同じ値を含めた
// 更新で 422 になって締め出されないため）。
func (in UpdateUserInput) validate(currentEmail string) []string {
	var msgs []string
	if in.Username != nil {
		msgs = append(msgs, domain.ValidateUsername(*in.Username)...)
	}
	if in.Email != nil && *in.Email != currentEmail {
		msgs = append(msgs, domain.ValidateEmail(*in.Email)...)
	}
	if in.passwordPresent() {
		msgs = append(msgs, domain.ValidatePassword(*in.Password)...)
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
// （422）、そしてカラム限定の書き込みの順である。すでに使われている email は
// *domain.ValidationError として返される（signup と違い、email の変更には確認メールを挟まない
// ので、認証済みのユーザーには登録の有無が分かる。既知の残課題として CLAUDE.md に記録している）。
func (s *Users) Update(ctx context.Context, viewer domain.User, targetID string, input UpdateUserInput) (domain.User, error) {
	target, err := s.query.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.User{}, domain.ErrForbidden
	}
	if msgs := input.validate(target.Email); len(msgs) > 0 {
		return domain.User{}, &domain.ValidationError{Messages: msgs}
	}
	changes := domain.ProfileChanges{Username: input.Username, Email: input.Email}
	if input.passwordPresent() {
		digest, err := s.hasher.Hash(*input.Password)
		if err != nil {
			return domain.User{}, fmt.Errorf("hash password: %w", err)
		}
		changes.PasswordDigest = &digest
	}
	updated, err := s.users.UpdateProfile(ctx, targetID, changes)
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
// 行い、hard DELETE は決して行わない。discard と、そのユーザーの kept な review が付く
// すべての burger の統計の再計算（burger_id の昇順）は、1 つのトランザクションで行う。
// ユーザーの review 自体は kept のままで（reviews.discarded_at は書き込まれない。Rails parity。
// 非表示化は読み取り側の u.discarded_at フィルタで行う）、再計算が、discard 済みの
// ユーザーの review を統計から外す。
func (s *Users) Delete(ctx context.Context, viewer domain.User, targetID string) error {
	target, err := s.query.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.ErrForbidden
	}
	err = s.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Users.Discard(ctx, targetID); err != nil {
			return err
		}
		return s.recalc.RecalculateReviewedBy(ctx, tx, targetID)
	})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
