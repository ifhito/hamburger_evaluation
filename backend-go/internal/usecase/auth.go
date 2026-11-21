package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// discardExpiredSessionLimit は、ログインのたびに日和見的に削除する期限切れセッションの最大件数である。
const discardExpiredSessionLimit = 20

// UserCredentials は、ログインのチェックのために、domain のユーザーと
// そのパスワードの digest を組にしたものである。digest は意図的に
// domain.User 上には決して置かれない。
type UserCredentials struct {
	User domain.User
	// PasswordDigest は、パスワードの digest である。パスワードでサインインする方法を持たない
	// アカウント(外部のサービスだけで作ったもの)は、空文字列である。
	PasswordDigest string
}

// UserQuery は、auth とユーザー管理の use case 向けの consumer 側の読み取りの
// 契約である。実装は storage のエラーを domain のエラーに対応させ、active な
// （discard されていない）ユーザーが一致しないとき（wrap された）
// domain.ErrUserNotFound を返す。読み取り専用で、書き込みのメソッドは
// 置かない（書き込みは domain.Users を通す）。
type UserQuery interface {
	// GetActiveUserByEmail は、指定された email(大文字小文字を区別しない)の、discard されていない
	// ユーザーを、そのパスワードの digest とともに返す。
	GetActiveUserByEmail(ctx context.Context, email string) (UserCredentials, error)
	// GetActiveUserByEmailIgnoreCase は、メールが(大文字小文字を区別せずに)一致する、discard されていない
	// ユーザーを返す。メールの一意性は lower(email) で守られているので、「登録済みか」の確認(signup・
	// 外部のサービスでの新規登録)は、こちらを使う。
	GetActiveUserByEmailIgnoreCase(ctx context.Context, email string) (domain.User, error)
	// GetActiveUserByID は、指定された id の、discard されていないユーザーを
	// 返す。
	GetActiveUserByID(ctx context.Context, id string) (domain.User, error)
	// GetActiveUserHasPassword は、discard されていないユーザーが、パスワードでサインインできるか(digest が
	// あるか)を返す。ユーザーがいなければ(wrap された)domain.ErrUserNotFound を返す。
	GetActiveUserHasPassword(ctx context.Context, id string) (bool, error)
}

// UserSessionQuery は、サーバー側に保存したセッション記録から有効な利用者を解決する読み取り専用の窓口である。
// 書き込みのメソッドは置かない(書き込みは domain.UserSessions を通す)。
type UserSessionQuery interface {
	// GetActiveUserBySessionTokenHash は、tokenHash と userID に一致する期限内のセッションから、
	// 退会していない利用者を返す。存在しない・期限切れ・ログアウト済み・退会済みの場合は
	// (wrap された)domain.ErrUserSessionNotFound を返す。
	GetActiveUserBySessionTokenHash(ctx context.Context, userID, tokenHash string) (domain.User, error)
}

// PasswordHasher はパスワードのハッシュ化と検証を行う。
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(digest, password string) error
}

// TokenIssuer は、ユーザー ID に対する認証トークンを発行する。
type TokenIssuer interface {
	Issue(userID string) (string, error)
}

// TokenVerifier は、生のトークンを検証し、それが持つユーザー ID を返す。
type TokenVerifier interface {
	Verify(token string) (string, error)
}

// sessionTTLIssuer は、発行するトークンの有効期間を返せる TokenIssuer(infra.JWTCodec など)を表す。
type sessionTTLIssuer interface {
	TTL() time.Duration
}

// sessionTTLOf は、issuer が有効期間を持っていればそれを返し、持たない代役のときは既定値を返す。
func sessionTTLOf(issuer TokenIssuer) time.Duration {
	if withTTL, ok := issuer.(sessionTTLIssuer); ok {
		return withTTL.TTL()
	}
	return domain.DefaultUserSessionTTL
}

// Auth は login、logout、トークン認証の use case を実装する（signup は Signups）。
// 認証に関する判断は HTTP handler ではなく、ここにある。読み取りは query、書き込みは domain の
// 書き込みオブジェクト(domain.UserSessions)だけを通し、repository には依存しない。
type Auth struct {
	query        UserQuery
	sessionQuery UserSessionQuery
	sessions     *domain.UserSessions
	hasher       PasswordHasher
	issuer       TokenIssuer
	verifier     TokenVerifier
}

func NewAuth(query UserQuery, sessionQuery UserSessionQuery, sessions *domain.UserSessions, hasher PasswordHasher, issuer TokenIssuer, verifier TokenVerifier) *Auth {
	return &Auth{
		query:        query,
		sessionQuery: sessionQuery,
		sessions:     sessions,
		hasher:       hasher,
		issuer:       issuer,
		verifier:     verifier,
	}
}

// dummyPasswordDigest は、固定の有効な bcrypt の digest（任意の使い捨て文字列の
// もので、infra の hasher と同様に cost 10）であり、Login でのダミーの
// compare にだけ使う。このアプリが保存するどの実在のパスワードにも一致しない。
const dummyPasswordDigest = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Login は email とパスワードで active なユーザーを認証し、サーバー側にセッションを記録したうえで
// 新しいトークンとともにそのユーザーを返す。認証情報が signup と同じ規則（domain.CredentialsIssues）
// を満たさないときは、DB の検索も hash の比較も行わず *domain.ValidationError を返す。
// 規則はアカウントの有無に関係なく同じ条件で判定するので、応答から、どの email に
// アカウントがあるかは分からない。規則を満たしたうえで、未知の email と誤った
// パスワードは、どちらも domain.ErrInvalidCredentials を返す。
func (a *Auth) Login(ctx context.Context, email, password string) (domain.User, string, error) {
	if issues := domain.CredentialsIssues(email, password); len(issues) > 0 {
		return domain.User{}, "", domain.NewValidationError(issues...)
	}
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
	digest := creds.PasswordDigest
	if digest == "" {
		// パスワードでサインインする方法を持たないアカウント(外部のサービスだけで作ったもの)。
		// 未知の email と同じく、hash の比較を 1 回分消費して、同じ失敗を返す。応答の文言も時間も、
		// 「そのアカウントにパスワードがない」ことを、外から推測させない。
		digest = dummyPasswordDigest
	}
	if err := a.hasher.Compare(digest, password); err != nil || creds.PasswordDigest == "" {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	token, err := a.issuer.Issue(creds.User.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	if err := a.sessions.Create(ctx, creds.User.ID, token, sessionTTLOf(a.issuer)); err != nil {
		return domain.User{}, "", fmt.Errorf("create session: %w", err)
	}
	discardExpiredUserSessions(ctx, a.sessions)
	return creds.User, token, nil
}

// AuthenticateToken は rawToken の署名・期限を検証したうえで、サーバー側に保存された有効なセッションと
// 退会していない利用者を照合して解決する。不正・期限切れ・ログアウト済みのトークン、未知または退会済みの
// ユーザーはいずれも domain.ErrUnauthenticated を返す。インフラの障害はそのまま伝播する。
func (a *Auth) AuthenticateToken(ctx context.Context, rawToken string) (domain.User, error) {
	userID, err := a.verifier.Verify(rawToken)
	if err != nil {
		return domain.User{}, domain.ErrUnauthenticated
	}
	user, err := a.sessionQuery.GetActiveUserBySessionTokenHash(ctx, userID, domain.HashSessionToken(rawToken))
	if err != nil {
		if errors.Is(err, domain.ErrUserSessionNotFound) || errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUnauthenticated
		}
		return domain.User{}, fmt.Errorf("authenticate session: %w", err)
	}
	return user, nil
}

// Logout は rawToken に対応するサーバー側のセッション記録を削除し、以後そのトークンを使えなくする。
func (a *Auth) Logout(ctx context.Context, rawToken string) error {
	if err := a.sessions.DiscardByToken(ctx, rawToken); err != nil {
		return fmt.Errorf("logout session: %w", err)
	}
	return nil
}

// discardExpiredUserSessions は、期限切れのセッションを上限つきで日和見的に削除する。
// トランザクション確定後に呼び、失敗しても認証結果には影響させない。
func discardExpiredUserSessions(ctx context.Context, sessions *domain.UserSessions) {
	if sessions == nil {
		return
	}
	if _, err := sessions.DiscardExpired(ctx, discardExpiredSessionLimit); err != nil {
		log.Printf("auth: discard expired sessions: %v", err)
	}
}
