package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// CreateUserParams は、新しいユーザーとして永続化するフィールドを保持する。
// パスワードはハッシュ化済みの状態で渡される。repository が平文を目にする
// ことはない。
type CreateUserParams struct {
	Username       string
	Email          string
	PasswordDigest string
	Admin          bool
}

// UserCredentials は、ログインのチェックのために、domain のユーザーと
// そのパスワードの digest を組にしたものである。digest は意図的に
// domain.User 上には決して置かれない。
type UserCredentials struct {
	User           domain.User
	PasswordDigest string
}

// UserRepository は auth 向けの consumer 側の永続化の契約である。
// 実装は storage のエラーを domain のエラーに対応させる。
// CreateUser は email の unique violation に対して（wrap された）
// domain.ErrEmailTaken を返し、lookup は、active な（discard されていない）
// ユーザーが一致しないとき domain.ErrUserNotFound を返す。
type UserRepository interface {
	CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error)
	GetActiveUserByEmail(ctx context.Context, email string) (UserCredentials, error)
	GetActiveUserByID(ctx context.Context, id int64) (domain.User, error)
}

// PasswordHasher はパスワードのハッシュ化と検証を行う。
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(digest, password string) error
}

// TokenIssuer は、ユーザー ID に対する認証トークンを発行する。
type TokenIssuer interface {
	Issue(userID int64) (string, error)
}

// TokenVerifier は、生のトークンを検証し、それが持つユーザー ID を返す。
type TokenVerifier interface {
	Verify(token string) (int64, error)
}

// Auth は signup、login、トークン認証の use case を実装する。
// 認証に関する判断は HTTP handler ではなく、ここにある。
type Auth struct {
	users    UserRepository
	hasher   PasswordHasher
	issuer   TokenIssuer
	verifier TokenVerifier
}

func NewAuth(users UserRepository, hasher PasswordHasher, issuer TokenIssuer, verifier TokenVerifier) *Auth {
	return &Auth{users: users, hasher: hasher, issuer: issuer, verifier: verifier}
}

// maxPasswordBytes は bcrypt の 72 バイトという入力上限を再現する。この上限は
// Rails の has_secure_password も強制している。
const maxPasswordBytes = 72

// SignupInput は signup use case の入力である。PasswordConfirmation は
// 任意であり（リクエストにそのフィールドがなかった場合は nil）、存在する
// ときは、Rails の has_secure_password に合わせて Password と等しくなければ
// ならない。
type SignupInput struct {
	Username             string
	Email                string
	Password             string
	PasswordConfirmation *string
}

// validate は Rails parity の full message を返す。valid なら空である。
func (in SignupInput) validate() []string {
	var msgs []string
	if in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	if in.Email == "" {
		msgs = append(msgs, "Email can't be blank")
	}
	if in.Password == "" {
		msgs = append(msgs, "Password can't be blank")
	}
	if len(in.Password) > maxPasswordBytes {
		msgs = append(msgs, "Password is too long (maximum is 72 characters)")
	}
	if in.PasswordConfirmation != nil && *in.PasswordConfirmation != in.Password {
		msgs = append(msgs, "Password confirmation doesn't match Password")
	}
	return msgs
}

// Signup は入力を validate し、新しいユーザーを保存し（常に admin=false）、
// 新しい認証トークンとともにそのユーザーを返す。validation の失敗と email の
// 重複は *domain.ValidationError として返される。
func (a *Auth) Signup(ctx context.Context, input SignupInput) (domain.User, string, error) {
	if msgs := input.validate(); len(msgs) > 0 {
		return domain.User{}, "", &domain.ValidationError{Messages: msgs}
	}
	digest, err := a.hasher.Hash(input.Password)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("hash password: %w", err)
	}
	user, err := a.users.CreateUser(ctx, CreateUserParams{
		Username:       input.Username,
		Email:          input.Email,
		PasswordDigest: digest,
		// 新しいユーザーは決して admin にならない。
		// 昇格は signup の範囲外である。
		Admin: false,
	})
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, "", &domain.ValidationError{Messages: []string{"Email has already been taken"}}
		}
		return domain.User{}, "", fmt.Errorf("create user: %w", err)
	}
	token, err := a.issuer.Issue(user.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return user, token, nil
}

// dummyPasswordDigest は、固定の有効な bcrypt の digest（任意の使い捨て文字列の
// もので、infra の hasher と同様に cost 10）であり、Login でのダミーの
// compare にだけ使う。このアプリが保存するどの実在のパスワードにも一致しない。
const dummyPasswordDigest = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Login は email とパスワードで active なユーザーを認証し、新しいトークンと
// ともにそのユーザーを返す。未知の email と誤ったパスワードは、どちらも
// domain.ErrInvalidCredentials を返す。
func (a *Auth) Login(ctx context.Context, email, password string) (domain.User, string, error) {
	creds, err := a.users.GetActiveUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// hash の比較を 1 回分あえて消費して、未知の email の経路が
			// 誤ったパスワードの経路とほぼ同じ時間になるようにする。
			// そうしないと、応答時間の差から、どの email にアカウントが
			// あるかを呼び出し側が列挙できてしまう。
			_ = a.hasher.Compare(dummyPasswordDigest, password)
			return domain.User{}, "", domain.ErrInvalidCredentials
		}
		return domain.User{}, "", fmt.Errorf("get user by email: %w", err)
	}
	if err := a.hasher.Compare(creds.PasswordDigest, password); err != nil {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	token, err := a.issuer.Issue(creds.User.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return creds.User, token, nil
}

// AuthenticateToken は rawToken を検証し、その active なユーザーを解決する。
// 不正または期限切れのトークン、未知または discard 済みのユーザーは、いずれも
// domain.ErrUnauthenticated を返す。インフラの障害はそのまま伝播する。
func (a *Auth) AuthenticateToken(ctx context.Context, rawToken string) (domain.User, error) {
	userID, err := a.verifier.Verify(rawToken)
	if err != nil {
		return domain.User{}, domain.ErrUnauthenticated
	}
	user, err := a.users.GetActiveUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUnauthenticated
		}
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}
