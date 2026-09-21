package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// discardExpiredLimit は、signup のたびに日和見的に削除する、期限切れの確認待ちの
// 最大件数である（専用のバックグラウンドジョブは作らない）。
const discardExpiredLimit = 20

// SignupConfirmation は、「signup の確認メールを送る」という意図である。宛先と、確認のリンクの
// URL と、その有効期間という、意味のある値だけを持つ。件名・本文・書式は持たない（文面は送信側が決める）。
type SignupConfirmation struct {
	// To は宛先の email である。
	To string
	// ConfirmURL は、確認のリンクの URL である（平文のトークンを含む）。
	ConfirmURL string
	// ValidFor は、リンクの有効期間である。
	ValidFor time.Duration
	// IdempotencyKey は、この要求の冪等キーである。同じキーの要求は、メールが 1 通しか出ない。
	IdempotencyKey string
}

// AlreadyRegisteredNotice は、「登録済みの email に、すでに登録済みであることを知らせる」という
// 意図である。確認のリンク・トークンは持たない。
type AlreadyRegisteredNotice struct {
	// To は宛先の email である。
	To string
	// SignInURL は、ログイン画面の URL である。
	SignInURL string
	// IdempotencyKey は、この要求の冪等キーである。同じキーの要求は、メールが 1 通しか出ない。
	IdempotencyKey string
}

// Mailer は、signup に関するメールを出す契約である。メソッドは、ドメインの意図を表す。
// メールの件名・本文・書式や、送信のプロバイダー（SMTP など）の形式・エラーは、実装が受け止め、
// この契約の外へ出さない。実装は呼び出しを待たせない（非同期）ので、送信の成否・遅延は呼び出し側に
// 見えず、応答時間から登録の有無を推測されない。送信の記録と失敗のログも実装が担う。
type Mailer interface {
	// SendSignupConfirmation は、確認リンクつきのメールを出す。
	SendSignupConfirmation(n SignupConfirmation)
	// SendAlreadyRegistered は、「すでに登録済み」の通知を出す。
	SendAlreadyRegistered(n AlreadyRegisteredNotice)
}

// SignupConfig は、signup の確認メールのリンクの設定である。
type SignupConfig struct {
	// BaseURL は、確認メールのリンクの生成元（frontend の URL。末尾に / を付けない）である。
	BaseURL string
	// Now は現在時刻を返す（冪等キーの時間の窓に使う）。nil なら time.Now である。テストが時計を進めるために注入する。
	Now func() time.Time
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
// メッセージは username、認証情報（domain.ValidateCredentials。email、password の順）、
// confirmation の順に並ぶ。登録の有無には依存しない検証だけを行う（「登録済み」を
// 示すエラーは返さない）。
func (in SignupInput) validate() []string {
	msgs := domain.ValidateUsername(in.Username)
	msgs = append(msgs, domain.ValidateCredentials(in.Email, in.Password)...)
	if in.PasswordConfirmation != nil && *in.PasswordConfirmation != in.Password {
		msgs = append(msgs, "Password confirmation doesn't match Password")
	}
	return msgs
}

// Signups は、メール確認つきの signup の use case を実装する。アカウントは、確認メールの
// リンクを開いて初めて作られる。読み取りは query、書き込みは domain の書き込みオブジェクト
// （domain.SignupVerifications）だけを通し、repository には依存しない。
type Signups struct {
	query         UserQuery
	verifications *domain.SignupVerifications
	hasher        PasswordHasher
	mailer        Mailer
	issuer        TokenIssuer
	cfg           SignupConfig
}

func NewSignups(query UserQuery, verifications *domain.SignupVerifications, hasher PasswordHasher, mailer Mailer, issuer TokenIssuer, cfg SignupConfig) *Signups {
	return &Signups{query: query, verifications: verifications, hasher: hasher, mailer: mailer, issuer: issuer, cfg: cfg}
}

// Request は signup の入力を検証し、確認メールを送る手配をする。**登録済みの email でも
// 未登録の email でも、成功したときは同じ結果（nil）を返す**ので、応答から登録の有無を
// 判別できない（アカウント列挙の防止）。検証の失敗は *domain.ValidationError で、登録の
// 有無には依存しない。
//
// どの分岐でも、bcrypt のハッシュ計算を行ってから分岐する（応答時間の差を bcrypt 1 回分の
// 揺らぎにとどめる）。メールの送信は Mailer が非同期に行う。登録済みなら、状態を変えず、
// 「すでに登録済み」の通知を送る（確認リンクは含まない）。未登録なら、確認待ちを最新の入力で
// 置き換えて確認メールを送る。前回の送信から domain.SignupResendInterval 以内なら、
// 確認待ちを変えず、メールも送らない。
func (s *Signups) Request(ctx context.Context, input SignupInput) error {
	if msgs := input.validate(); len(msgs) > 0 {
		return &domain.ValidationError{Messages: msgs}
	}
	digest, err := s.hasher.Hash(input.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	s.discardExpired(ctx)

	_, err = s.query.GetActiveUserByEmail(ctx, input.Email)
	switch {
	case err == nil:
		s.mailer.SendAlreadyRegistered(AlreadyRegisteredNotice{
			To:             input.Email,
			SignInURL:      s.cfg.BaseURL + "/signin",
			IdempotencyKey: domain.AlreadyRegisteredMailKey(input.Email, s.now()),
		})
		return nil
	case !errors.Is(err, domain.ErrUserNotFound):
		return fmt.Errorf("get user by email: %w", err)
	}

	token, err := domain.NewSignupToken()
	if err != nil {
		return err
	}
	receipt, err := s.verifications.Create(ctx, domain.CreateSignupVerificationParams{
		Email:          input.Email,
		Username:       input.Username,
		PasswordDigest: digest,
		TokenHash:      token.Hash,
	})
	if err != nil {
		return fmt.Errorf("create signup verification: %w", err)
	}
	if receipt.Accepted {
		s.mailer.SendSignupConfirmation(SignupConfirmation{
			To:             input.Email,
			ConfirmURL:     s.cfg.BaseURL + "/signup/confirm?token=" + url.QueryEscape(token.Raw),
			ValidFor:       domain.SignupTokenTTL,
			IdempotencyKey: domain.SignupConfirmationMailKey(receipt.ID, receipt.Generation),
		})
	}
	return nil
}

// Confirm は、確認メールのリンクの平文トークンでアカウントを作成し、新しい認証トークンとともに
// そのユーザーを返す（従来の signup の成功と同じ結果）。期限切れ・存在しない・改ざん・使用済みの
// トークンは、確認までの間に同じ email のユーザーが作られていた場合も含め、すべて
// domain.ErrSignupTokenInvalid になる（区別できない）。
func (s *Signups) Confirm(ctx context.Context, rawToken string) (domain.User, string, error) {
	user, err := s.verifications.Confirm(ctx, rawToken)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, "", fmt.Errorf("confirm signup: %w", domain.ErrSignupTokenInvalid)
		}
		return domain.User{}, "", fmt.Errorf("confirm signup: %w", err)
	}
	token, err := s.issuer.Issue(user.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return user, token, nil
}

// discardExpired は、期限切れの確認待ちを上限つきで削除する。失敗しても signup の結果には
// 影響させない（掃除は日和見的で、次の signup で再び試みられる）。
func (s *Signups) discardExpired(ctx context.Context) {
	if _, err := s.verifications.DiscardExpired(ctx, discardExpiredLimit); err != nil {
		log.Printf("signup: discard expired verifications: %v", err)
	}
}

// now は現在時刻を返す。
func (s *Signups) now() time.Time {
	if s.cfg.Now != nil {
		return s.cfg.Now()
	}
	return time.Now()
}
