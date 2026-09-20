package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// UserCredentials は、ログインのチェックのために、domain のユーザーと
// そのパスワードの digest を組にしたものである。digest は意図的に
// domain.User 上には決して置かれない。
type UserCredentials struct {
	User           domain.User
	PasswordDigest string
}

// UserQuery は、auth とユーザー管理の use case 向けの consumer 側の読み取りの
// 契約である。実装は storage のエラーを domain のエラーに対応させ、active な
// （discard されていない）ユーザーが一致しないとき（wrap された）
// domain.ErrUserNotFound を返す。読み取り専用で、書き込みのメソッドは
// 置かない（書き込みは domain.UserService を通す）。
type UserQuery interface {
	// GetActiveUserByEmail は、指定された email の、discard されていない
	// ユーザーを、そのパスワードの digest とともに返す。
	GetActiveUserByEmail(ctx context.Context, email string) (UserCredentials, error)
	// GetActiveUserByID は、指定された id の、discard されていないユーザーを
	// 返す。
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
// 認証に関する判断は HTTP handler ではなく、ここにある。読み取りは query、
// 書き込みは domain のサービスだけを通し、repository には依存しない。
type Auth struct {
	query    UserQuery
	users    *domain.UserService
	hasher   PasswordHasher
	issuer   TokenIssuer
	verifier TokenVerifier
}

func NewAuth(query UserQuery, users *domain.UserService, hasher PasswordHasher, issuer TokenIssuer, verifier TokenVerifier) *Auth {
	return &Auth{query: query, users: users, hasher: hasher, issuer: issuer, verifier: verifier}
}

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
// メッセージは username、email、password（domain.ValidatePassword）、
// confirmation の順に並ぶ。
func (in SignupInput) validate() []string {
	var msgs []string
	if in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	if in.Email == "" {
		msgs = append(msgs, "Email can't be blank")
	}
	msgs = append(msgs, domain.ValidatePassword(in.Password)...)
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
	user, err := a.users.Create(ctx, domain.CreateUserParams{
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
	creds, err := a.query.GetActiveUserByEmail(ctx, email)
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
	user, err := a.query.GetActiveUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUnauthenticated
		}
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}
