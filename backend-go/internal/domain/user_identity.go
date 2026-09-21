package domain

import (
	"context"
	"errors"
	"time"
)

// ProviderGoogle は、Google のアカウントでのサインインを表す、サインイン方法の名前である。
// DB の CHECK 制約 user_identities_provider_check(000013_create_user_identities)と同じ値でなければ
// ならない。サービスを足すときは、ここと CHECK の両方に足す。
const ProviderGoogle = "google"

var (
	// ErrIdentityNotFound は、条件に一致する、外部のアカウントとの結び付きがないことを表す。
	ErrIdentityNotFound = errors.New("identity not found")
	// ErrIdentityTaken は、その外部のアカウント(サービスの名前と、そのサービスでの利用者の ID の組)が、すでにどれかのアカウントに
	// 結び付いていることを表す。
	ErrIdentityTaken = errors.New("identity is already linked to an account")
	// ErrIdentityAlreadyLinked は、そのアカウントが、同じサービスの、すでに別の外部のアカウントと
	// 結び付いていることを表す(1 つのアカウントに、同じサービスは 1 つだけ結び付けられる)。
	ErrIdentityAlreadyLinked = errors.New("account is already linked to an identity of this provider")
	// ErrCannotUnlinkIdentity は、結び付きを解除すると、そのアカウントで、サインインする方法が
	// なくなることを表す。
	ErrCannotUnlinkIdentity = errors.New("cannot unlink the only way to sign in")
	// ErrExternalIdentityRejected は、外部のサービスが確かめた利用者の情報が、サインインに使えない
	// (メールが確認済みでない・空・形が不正)ことを表す。
	ErrExternalIdentityRejected = errors.New("external identity cannot be used to sign in")
)

// UserIdentity は、このアプリのアカウントと、外部のサービス(Google など)のアカウントとの結び付きである。
// 利用者の識別には、メールアドレスではなく、外部のサービスが付ける、変わらない ID(ProviderUserID)を使う。
// 外部のサービスのトークンは、保存しない。
type UserIdentity struct {
	ID       string
	UserID   string
	Provider string
	// ProviderUserID は、外部のサービスが付ける、その利用者の変わらない ID である(Google の `sub`)。
	ProviderUserID string
	// Email は、結び付けたときの、外部のサービスのメールアドレスである(画面に見せるためだけで、
	// サインインの判断には使わない)。
	Email     string
	CreatedAt time.Time
}

// ExternalIdentity は、外部のサービスが確かめて、返してきた利用者の情報である(サインインの結果)。
type ExternalIdentity struct {
	Provider       string
	ProviderUserID string
	Email          string
	// EmailVerified は、外部のサービスが、そのメールアドレスの持ち主だと確かめたかである。
	EmailVerified bool
	// Name は、外部のサービスの表示名である(空のことがある)。ユーザー名を決める材料にだけ使う。
	Name string
}

// Validate は、この情報でサインイン・新規登録・連携をしてよいかを判断する。識別の ID とメールがあり、
// メールが確認済みで、メールの形が規則(ValidateEmail)を満たすときだけ nil を返す。メールを、外部の
// サービスが確かめていないと、他人のメールを名乗って、その人のアカウントに入れるため、確認済みでない
// ものは受け付けない。満たさないときは(wrap された)ErrExternalIdentityRejected を返す。
func (e ExternalIdentity) Validate() error {
	if e.Provider == "" || e.ProviderUserID == "" || e.Email == "" {
		return errors.Join(ErrExternalIdentityRejected, errors.New("missing provider, provider user id or email"))
	}
	if !e.EmailVerified {
		return errors.Join(ErrExternalIdentityRejected, errors.New("email is not verified"))
	}
	if msgs := ValidateEmail(e.Email); len(msgs) > 0 {
		return errors.Join(ErrExternalIdentityRejected, errors.New("email is invalid"))
	}
	return nil
}

// CanUnlinkIdentity は、外部のアカウントとの結び付きを解除してよいかを判断する。解除したあとも、
// サインインする方法が残る(パスワードがある、または、ほかの結び付きがある)ときだけ許す。
// 満たさないときは(wrap された)ErrCannotUnlinkIdentity を返す。
func CanUnlinkIdentity(hasPassword bool, identityCount int) error {
	if hasPassword || identityCount > 1 {
		return nil
	}
	return ErrCannotUnlinkIdentity
}

// ---- repository の契約(実装は adapter/repository) ----

// CreateUserIdentityParams は、結び付きとして保存するフィールドを保持する。
type CreateUserIdentityParams struct {
	UserID         string
	Provider       string
	ProviderUserID string
	Email          string
}

// UserIdentityRepository は、外部のアカウントとの結び付きの書き込みの契約である。domain が宣言し、
// 呼び出すのは domain のコード(書き込みオブジェクトの UserIdentities)だけで、usecase は呼ばない。
// 書き込み専用で、読み取りのメソッドは置かない(読み取りは usecase の IdentityQuery)。
type UserIdentityRepository interface {
	// CreateUserIdentity は結び付きを保存して返す。その外部のアカウントが、すでに結び付いていれば
	// (wrap された)ErrIdentityTaken、そのアカウントに、同じサービスの結び付きがすでにあれば
	// (wrap された)ErrIdentityAlreadyLinked を返す。
	CreateUserIdentity(ctx context.Context, params CreateUserIdentityParams) (UserIdentity, error)
	// DiscardUserIdentity は、userID の、provider の結び付きを削除する。なければ(wrap された)
	// ErrIdentityNotFound を返す。
	DiscardUserIdentity(ctx context.Context, userID, provider string) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// UserIdentities は、結び付きの書き込みオブジェクトである。UserIdentityRepository を持つのは
// この型だけで、usecase は repository に依存しない。アカウントの作成と結び付きの記録を 1 つの
// トランザクションで行う手順は、複数の集約を跨ぐので、この型には置かず、トランザクションを持つ
// usecase が UnitOfWork の中で組み立てる。
type UserIdentities struct {
	repo UserIdentityRepository
}

// NewUserIdentities は repo を使う UserIdentities を返す。
func NewUserIdentities(repo UserIdentityRepository) *UserIdentities {
	return &UserIdentities{repo: repo}
}

// Create は結び付きを保存して返す。
func (s *UserIdentities) Create(ctx context.Context, params CreateUserIdentityParams) (UserIdentity, error) {
	return s.repo.CreateUserIdentity(ctx, params)
}

// Discard は、userID の、provider の結び付きを削除する。
func (s *UserIdentities) Discard(ctx context.Context, userID, provider string) error {
	return s.repo.DiscardUserIdentity(ctx, userID, provider)
}
